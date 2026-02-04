package emberkv

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
)

const (
	magicBytes    = "EMBK"
	versionByte   = 1
	opSet         = 1
	opDelete      = 2
	headerSize    = 20
	maxKeyBytes   = 1 << 20
	maxValueBytes = 1 << 20
	maxEntryBytes = 1 << 20
)

var (
	ErrClosed        = errors.New("emberkv: closed")
	ErrLocked        = errors.New("emberkv: database is locked")
	ErrInvalidKey    = errors.New("emberkv: invalid key")
	ErrInvalidValue  = errors.New("emberkv: invalid value")
	ErrInvalidRecord = errors.New("emberkv: invalid record")
	ErrKeyTooLarge   = errors.New("emberkv: key too large")
	ErrValueTooLarge = errors.New("emberkv: value too large")
	ErrEntryTooLarge = errors.New("emberkv: entry too large")
)

// DB is a single-process persistent key-value store backed by an append-only log.
type DB struct {
	mu       sync.Mutex
	dir      string
	logPath  string
	log      *os.File
	lockFile *os.File
	closed   bool
	index    map[string][]byte
}

// Open creates or reopens a database rooted at directory.
func Open(directory string) (*DB, error) {
	if directory == "" {
		return nil, errors.New("emberkv: directory is required")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("create dir: %w", err)
	}

	lockPath := filepath.Join(directory, "emberkv.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lockFile.Close()
		return nil, ErrLocked
	}

	logPath := filepath.Join(directory, "emberkv.log")
	if _, statErr := os.Stat(logPath); os.IsNotExist(statErr) {
		backupPath := logPath + ".bak"
		if _, backupErr := os.Stat(backupPath); backupErr == nil {
			if err := os.Rename(backupPath, logPath); err != nil {
				lockFile.Close()
				return nil, fmt.Errorf("restore backup: %w", err)
			}
		}
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		lockFile.Close()
		return nil, fmt.Errorf("open log: %w", err)
	}

	db := &DB{
		dir:      directory,
		logPath:  logPath,
		log:      logFile,
		lockFile: lockFile,
		index:    make(map[string][]byte),
	}
	if err := db.rebuildIndex(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// Set stores key/value, overwriting any previous value.
func (db *DB) Set(key, value []byte) error {
	if db == nil {
		return ErrClosed
	}
	if err := validateKeyValue(key, value); err != nil {
		return err
	}
	return db.writeRecord(opSet, key, value)
}

// Get returns the stored value and true when the key exists.
func (db *DB) Get(key []byte) ([]byte, bool, error) {
	if db == nil {
		return nil, false, ErrClosed
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return nil, false, ErrClosed
	}
	if err := validateKey(key); err != nil {
		return nil, false, err
	}
	v, ok := db.index[string(key)]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), v...), true, nil
}

// Delete removes the key if present.
func (db *DB) Delete(key []byte) error {
	if db == nil {
		return ErrClosed
	}
	if err := validateKey(key); err != nil {
		return err
	}
	return db.writeRecord(opDelete, key, nil)
}

// Compact rewrites the log using a safe replacement protocol.
func (db *DB) Compact() error {
	if db == nil {
		return ErrClosed
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return ErrClosed
	}

	entries := make([]string, 0, len(db.index))
	for key := range db.index {
		entries = append(entries, key)
	}
	sort.Strings(entries)

	tempPath := db.logPath + ".tmp"
	tempFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create compaction temp: %w", err)
	}
	for _, key := range entries {
		value := db.index[key]
		record, err := encodeRecord(opSet, []byte(key), value)
		if err != nil {
			_ = tempFile.Close()
			_ = os.Remove(tempPath)
			return err
		}
		if n, err := tempFile.Write(record); err != nil {
			_ = tempFile.Close()
			_ = os.Remove(tempPath)
			return err
		} else if n != len(record) {
			_ = tempFile.Close()
			_ = os.Remove(tempPath)
			return io.ErrShortWrite
		}
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("sync compaction temp: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close compaction temp: %w", err)
	}

	backupPath := db.logPath + ".bak"
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove old backup: %w", err)
	}
	if err := os.Rename(db.logPath, backupPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("move active log to backup: %w", err)
	}
	if err := os.Rename(tempPath, db.logPath); err != nil {
		if _, restoreErr := os.Stat(backupPath); restoreErr == nil {
			_ = os.Rename(backupPath, db.logPath)
		}
		return fmt.Errorf("replace active log: %w", err)
	}
	if err := syncDir(db.dir); err != nil {
		return fmt.Errorf("sync directory after compaction: %w", err)
	}

	if err := db.log.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		return fmt.Errorf("close active log: %w", err)
	}
	logFile, err := os.OpenFile(db.logPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("reopen log after compaction: %w", err)
	}
	db.log = logFile

	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove backup after compaction: %w", err)
	}
	if err := syncDir(db.dir); err != nil {
		return fmt.Errorf("sync directory after backup cleanup: %w", err)
	}
	return nil
}

// Close releases the database handles.
func (db *DB) Close() error {
	if db == nil {
		return nil
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return nil
	}
	db.closed = true
	if db.log != nil {
		if err := db.log.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			return fmt.Errorf("close log: %w", err)
		}
		db.log = nil
	}
	if db.lockFile != nil {
		if err := syscall.Flock(int(db.lockFile.Fd()), syscall.LOCK_UN); err != nil && !errors.Is(err, syscall.EBADF) {
			return fmt.Errorf("release lock: %w", err)
		}
		if err := db.lockFile.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			return fmt.Errorf("close lock file: %w", err)
		}
		db.lockFile = nil
	}
	return nil
}

func (db *DB) writeRecord(op byte, key, value []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return ErrClosed
	}
	if db.log == nil {
		return ErrClosed
	}

	record, err := encodeRecord(op, key, value)
	if err != nil {
		return err
	}
	if n, err := db.log.Write(record); err != nil {
		return fmt.Errorf("append record: %w", err)
	} else if n != len(record) {
		return io.ErrShortWrite
	}
	if err := db.log.Sync(); err != nil {
		return fmt.Errorf("sync record: %w", err)
	}

	switch op {
	case opSet:
		db.index[string(key)] = append([]byte(nil), value...)
	case opDelete:
		delete(db.index, string(key))
	}
	return nil
}

func encodeRecord(op byte, key, value []byte) ([]byte, error) {
	if err := validateKeyValue(key, value); err != nil {
		if op == opDelete {
			return nil, validateKey(key)
		}
		return nil, err
	}
	if op != opSet && op != opDelete {
		return nil, fmt.Errorf("%w: unsupported operation %d", ErrInvalidRecord, op)
	}
	payloadLen := len(key) + len(value)
	if op == opDelete {
		payloadLen = len(key)
	}
	if payloadLen > maxEntryBytes {
		return nil, ErrEntryTooLarge
	}

	record := make([]byte, headerSize+payloadLen)
	copy(record[:4], magicBytes)
	record[4] = versionByte
	record[5] = op
	binary.LittleEndian.PutUint32(record[8:12], uint32(len(key)))
	binary.LittleEndian.PutUint32(record[12:16], uint32(len(value)))
	payload := record[headerSize:]
	copy(payload, key)
	if op == opSet {
		copy(payload[len(key):], value)
	}
	checksum := crc32.ChecksumIEEE(payload)
	binary.LittleEndian.PutUint32(record[16:20], checksum)
	return record, nil
}

func parseHeader(header []byte) (byte, uint32, uint32, uint32, error) {
	if len(header) != headerSize {
		return 0, 0, 0, 0, fmt.Errorf("%w: short header", ErrInvalidRecord)
	}
	if string(header[:4]) != magicBytes {
		return 0, 0, 0, 0, fmt.Errorf("%w: bad magic", ErrInvalidRecord)
	}
	if header[4] != versionByte {
		return 0, 0, 0, 0, fmt.Errorf("%w: unsupported version %d", ErrInvalidRecord, header[4])
	}
	op := header[5]
	if op != opSet && op != opDelete {
		return 0, 0, 0, 0, fmt.Errorf("%w: unsupported operation %d", ErrInvalidRecord, op)
	}
	keyLen := binary.LittleEndian.Uint32(header[8:12])
	valueLen := binary.LittleEndian.Uint32(header[12:16])
	if keyLen > maxKeyBytes {
		return 0, 0, 0, 0, ErrKeyTooLarge
	}
	if valueLen > maxValueBytes {
		return 0, 0, 0, 0, ErrValueTooLarge
	}
	if op == opDelete && valueLen != 0 {
		return 0, 0, 0, 0, fmt.Errorf("%w: delete record with value", ErrInvalidRecord)
	}
	checksum := binary.LittleEndian.Uint32(header[16:20])
	return op, keyLen, valueLen, checksum, nil
}

func (db *DB) rebuildIndex() error {
	file, err := os.Open(db.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open log for recovery: %w", err)
	}
	defer file.Close()

	db.index = make(map[string][]byte)
	for {
		var header [headerSize]byte
		if _, err := io.ReadFull(file, header[:]); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return fmt.Errorf("read record header: %w", err)
		}

		op, keyLen, valueLen, checksum, err := parseHeader(header[:])
		if err != nil {
			return nil
		}
		payloadLen := int(keyLen) + int(valueLen)
		if payloadLen > maxEntryBytes {
			return nil
		}
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(file, payload); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return fmt.Errorf("read record payload: %w", err)
		}
		if crc32.ChecksumIEEE(payload) != checksum {
			return nil
		}

		key := payload[:keyLen]
		value := payload[keyLen:]
		switch op {
		case opSet:
			db.index[string(key)] = append([]byte(nil), value...)
		case opDelete:
			delete(db.index, string(key))
		default:
			return nil
		}
	}
}

func validateKey(key []byte) error {
	if len(key) == 0 {
		return ErrInvalidKey
	}
	if len(key) > maxKeyBytes {
		return ErrKeyTooLarge
	}
	return nil
}

func validateValue(value []byte) error {
	if len(value) > maxValueBytes {
		return ErrValueTooLarge
	}
	return nil
}

func validateKeyValue(key, value []byte) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if err := validateValue(value); err != nil {
		return err
	}
	if len(key)+len(value) > maxEntryBytes {
		return ErrEntryTooLarge
	}
	return nil
}

func syncDir(dir string) error {
	dirFile, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open directory: %w", err)
	}
	defer dirFile.Close()
	if err := dirFile.Sync(); err != nil {
		return fmt.Errorf("fsync directory: %w", err)
	}
	return nil
}
