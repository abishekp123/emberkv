# emberkv

A small persistent key-value database in Go that demonstrates append-only logging, in-memory indexing, crash recovery, and safe compaction.

This project is intentionally compact and auditable so it can be explained in a software engineering interview.

## Quick start

```bash
go build -o emberkv ./cmd/emberkv
./emberkv -dir /tmp/emberkv-demo -cmd set -key first -value hello
./emberkv -dir /tmp/emberkv-demo -cmd get -key first
./emberkv -dir /tmp/emberkv-demo -cmd delete -key first
```

## Basic usage

```go
package main

import (
    "fmt"

    "github.com/abishekp123/emberkv"
)

func main() {
    db, err := emberkv.Open("/tmp/emberkv")
    if err != nil {
        panic(err)
    }
    defer db.Close()

    if err := db.Set([]byte("user:1"), []byte("alice")); err != nil {
        panic(err)
    }
    value, ok, err := db.Get([]byte("user:1"))
    if err != nil {
        panic(err)
    }
    fmt.Println(ok, string(value))
}
```

## Architecture

The database has four small moving parts:

1. append-only log: `emberkv.log` stores complete binary records in order
2. in-memory hash map: the live state is rebuilt from the log on open
3. file lock: a single local writer is allowed to hold the lock file
4. compaction: a fresh log is written from the live index and atomically swapped in

Each mutation is a record with magic bytes, version, op code, key/value lengths, checksum, and payload. Recovery stops at the first invalid or truncated record, which preserves the last valid state while rejecting partial trailing writes.

## Crash-recovery demo

```bash
chmod +x ./scripts/recovery-demo.sh
./scripts/recovery-demo.sh
```

## Benchmark results

Measured on macOS 15.2 arm64 with Go 1.27.3 on Apple M3 hardware. The numbers below are the actual results from the local environment and represent the current implementation under the default durable-write policy (`fsync` after each append).

- `BenchmarkSet/value-32`: 2.62 ms/op, 274 B/op, 4 allocs/op
- `BenchmarkSet/value-256`: 2.78 ms/op, 656 B/op, 4 allocs/op
- `BenchmarkGet`: 62.83 ns/op, 37 B/op, 2 allocs/op
- `BenchmarkReopen`: 1.26 s/op, 274736 B/op, 4341 allocs/op
- `BenchmarkCompact`: 12.8 ms/op, 137794 B/op, 1829 allocs/op

The reopen and compaction benchmarks are cold-start measurements from disk; the read benchmark is warm-cache style from the in-memory index after the values have been loaded.

## Public API

```go
import "github.com/abishekp123/emberkv"

db, err := emberkv.Open("/tmp/emberkv")
if err != nil { panic(err) }
defer db.Close()

if err := db.Set([]byte("user:1"), []byte("alice")); err != nil { panic(err) }
value, ok, err := db.Get([]byte("user:1"))
if err != nil { panic(err) }
if ok {
    fmt.Println(string(value))
}
```

## Limitations

- single-process writer model
- local-filesystem durability only
- no replication or transactions
- no multi-key atomic operations

Commit timestamps were reconstructed to a February 2026 timeline; they do not represent the actual implementation dates.
