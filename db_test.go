package emberkv

import (
	"os"
	"path/filepath"
	"testing"
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
		t.Fatalf("Set(empty) error = %v", err)
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
