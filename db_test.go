package emberkv

import (
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSetGetDelete(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := db.Set([]byte("alpha"), []byte("one")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := db.Set([]byte("beta"), []byte("")); err != nil {
		t.Fatalf("Set(empty value) error = %v", err)
	}

	v, ok, err := db.Get([]byte("alpha"))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || string(v) != "one" {
		t.Fatalf("Get(alpha) = (%q, %v), want (one, true)", v, ok)
	}

	v, ok, err = db.Get([]byte("beta"))
	if err != nil {
		t.Fatalf("Get(beta) error = %v", err)
	}
	if !ok || string(v) != "" {
		t.Fatalf("Get(beta) = (%q, %v), want (\"\", true)", v, ok)
	}

	if err := db.Delete([]byte("alpha")); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok, err := db.Get([]byte("alpha")); err != nil || ok {
		t.Fatalf("Get(alpha) after delete = (%v, %v), want (nil, false)", err, ok)
	}
}

func TestInvalidAndOversizedInputs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := db.Set(nil, []byte("value")); err == nil {
		t.Fatal("Set(nil key) = nil, want error")
	}
	if err := db.Set([]byte("k"), make([]byte, maxValueBytes+1)); err == nil {
		t.Fatal("Set(oversized value) = nil, want error")
	}
	if err := db.Set(make([]byte, maxKeyBytes+1), []byte("v")); err == nil {
		t.Fatal("Set(oversized key) = nil, want error")
	}
	if err := db.Delete(nil); err == nil {
		t.Fatal("Delete(nil key) = nil, want error")
	}
}

func TestRecoveryFromTruncatedLog(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db.Set([]byte("keep"), []byte("value")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	logPath := filepath.Join(dir, "emberkv.log")
	f, err := os.OpenFile(logPath, os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("Open logfile: %v", err)
	}
	if _, err := f.Write([]byte{0xFF, 0xFF, 0xFF}); err != nil {
		t.Fatalf("short write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	db, err = Open(dir)
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	defer db.Close()

	v, ok, err := db.Get([]byte("keep"))
	if err != nil {
		t.Fatalf("Get() after reopen = %v", err)
	}
	if !ok || string(v) != "value" {
		t.Fatalf("Get(keep) after reopen = (%q, %v), want (value, true)", v, ok)
	}
}

func TestCompactionRemovesDeletedKeys(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db.Set([]byte("a"), []byte("1")); err != nil {
		t.Fatalf("Set(a) = %v", err)
	}
	if err := db.Set([]byte("b"), []byte("2")); err != nil {
		t.Fatalf("Set(b) = %v", err)
	}
	if err := db.Delete([]byte("a")); err != nil {
		t.Fatalf("Delete(a) = %v", err)
	}
	if err := db.Compact(); err != nil {
		t.Fatalf("Compact() = %v", err)
	}
	if _, ok, err := db.Get([]byte("a")); err != nil || ok {
		t.Fatalf("Get(a) after compact = (%v, %v), want (nil, false)", err, ok)
	}
	if v, ok, err := db.Get([]byte("b")); err != nil || !ok || string(v) != "2" {
		t.Fatalf("Get(b) after compact = (%q, %v, %v), want (2, true, nil)", v, ok, err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() again = %v", err)
	}
}

func TestClosedStoreBehavior(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := db.Set([]byte("x"), []byte("y")); err != ErrClosed {
		t.Fatalf("Set on closed db = %v, want %v", err, ErrClosed)
	}
	if _, _, err := db.Get([]byte("x")); err != ErrClosed {
		t.Fatalf("Get on closed db = %v, want %v", err, ErrClosed)
	}
}

func TestRandomizedReferenceModel(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	expected := make(map[string][]byte)
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 200; i++ {
		key := []byte("k")
		key = append(key, byte(rng.Intn(20)))
		switch rng.Intn(4) {
		case 0:
			value := make([]byte, rng.Intn(16))
			for j := range value {
				value[j] = byte(rng.Intn(256))
			}
			expected[string(key)] = value
			if err := db.Set(key, value); err != nil {
				t.Fatalf("Set step %d: %v", i, err)
			}
		case 1:
			if err := db.Delete(key); err != nil {
				t.Fatalf("Delete step %d: %v", i, err)
			}
			delete(expected, string(key))
		case 2:
			v, ok, err := db.Get(key)
			if err != nil {
				t.Fatalf("Get step %d: %v", i, err)
			}
			want, exists := expected[string(key)]
			if ok != exists || !bytes.Equal(v, want) {
				t.Fatalf("step %d mismatch: key=%q got=(%v,%v) want=(%v,%v)", i, key, v, ok, want, exists)
			}
		default:
			if err := db.Compact(); err != nil {
				t.Fatalf("Compact step %d: %v", i, err)
			}
		}
	}
}

func TestRecoveryAfterCorruptionStopsAtBadRecord(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db.Set([]byte("good"), []byte("still good")); err != nil {
		t.Fatalf("Set(good): %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	path := filepath.Join(dir, "emberkv.log")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("Open logfile: %v", err)
	}
	if _, err := f.Write([]byte("BAD")); err != nil {
		t.Fatalf("Write corruption: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close corrupt log: %v", err)
	}

	db, err = Open(dir)
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	defer db.Close()
	if v, ok, err := db.Get([]byte("good")); err != nil || !ok || string(v) != "still good" {
		t.Fatalf("Get(good) after corruption = (%q, %v, %v), want (still good, true, nil)", v, ok, err)
	}
}

func TestReopenRebuildsIndex(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db.Set([]byte("a"), []byte("1")); err != nil {
		t.Fatalf("Set(a): %v", err)
	}
	if err := db.Set([]byte("b"), []byte("2")); err != nil {
		t.Fatalf("Set(b): %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	db, err = Open(dir)
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	defer db.Close()
	if v, ok, err := db.Get([]byte("a")); err != nil || !ok || string(v) != "1" {
		t.Fatalf("Get(a) after reopen = (%q,%v,%v), want (1,true,nil)", v, ok, err)
	}
	if v, ok, err := db.Get([]byte("b")); err != nil || !ok || string(v) != "2" {
		t.Fatalf("Get(b) after reopen = (%q,%v,%v), want (2,true,nil)", v, ok, err)
	}
}

func TestDefaultRandSourceNotNeeded(t *testing.T) {
	_ = time.Now()
}
