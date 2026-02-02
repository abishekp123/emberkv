package emberkv

import (
	"errors"
)

var ErrClosed = errors.New("emberkv: closed")

// DB is the public storage interface. The implementation is supplied in later stages.
type DB struct{}

// Open creates a database rooted at directory.
func Open(directory string) (*DB, error) {
	return nil, errors.New("emberkv: not implemented yet")
}

// Set writes a key/value pair.
func (db *DB) Set(key, value []byte) error {
	return ErrClosed
}

// Get returns the value and whether it exists.
func (db *DB) Get(key []byte) ([]byte, bool, error) {
	return nil, false, ErrClosed
}

// Delete removes a key.
func (db *DB) Delete(key []byte) error {
	return ErrClosed
}

// Compact rewrites the log without stale entries.
func (db *DB) Compact() error {
	return ErrClosed
}

// Close releases resources.
func (db *DB) Close() error {
	return ErrClosed
}
