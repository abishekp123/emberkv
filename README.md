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

## Design

See [DESIGN.md](DESIGN.md) for the record format, crash-recovery model, synchronization policy, and compaction protocol.

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
