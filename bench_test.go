package emberkv

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkSet(b *testing.B) {
	for _, size := range []int{32, 256} {
		b.Run(fmt.Sprintf("value-%d", size), func(b *testing.B) {
			dir := filepath.Join(b.TempDir(), "bench")
			db, err := Open(dir)
			if err != nil {
				b.Fatal(err)
			}
			defer db.Close()
			value := make([]byte, size)
			for i := range value {
				value[i] = byte(i)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				key := []byte(fmt.Sprintf("k-%d", i))
				if err := db.Set(key, value); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkGet(b *testing.B) {
	dir := filepath.Join(b.TempDir(), "bench")
	db, err := Open(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	for i := 0; i < 1000; i++ {
		if err := db.Set([]byte(fmt.Sprintf("k-%d", i)), []byte("value-value-value")); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := db.Get([]byte(fmt.Sprintf("k-%d", i%1000))); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReopen(b *testing.B) {
	for i := 0; i < b.N; i++ {
		dir := filepath.Join(b.TempDir(), "bench")
		db, err := Open(dir)
		if err != nil {
			b.Fatal(err)
		}
		for j := 0; j < 500; j++ {
			if err := db.Set([]byte(fmt.Sprintf("k-%d", j)), []byte("value-value-value")); err != nil {
				b.Fatal(err)
			}
		}
		if err := db.Close(); err != nil {
			b.Fatal(err)
		}
		if _, err := Open(dir); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompact(b *testing.B) {
	dir := filepath.Join(b.TempDir(), "bench")
	db, err := Open(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	for i := 0; i < 2000; i++ {
		if err := db.Set([]byte(fmt.Sprintf("k-%d", i)), []byte("hello-world-hello-world")); err != nil {
			b.Fatal(err)
		}
		if i%10 == 0 {
			if err := db.Delete([]byte(fmt.Sprintf("k-%d", i/2))); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := db.Compact(); err != nil {
			b.Fatal(err)
		}
	}
	_ = os.Remove(dir)
}
