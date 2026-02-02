# emberkv design

emberskv is a deliberately small single-process key-value store built around an append-only log. The design aims for clear crash recovery, straightforward validation, and an auditable compaction protocol.

## Record layout and validation

Each record in the log begins with a fixed binary header:

- magic: 4 bytes, ASCII `EMBK`
- version: 1 byte, currently `1`
- op: 1 byte (`1 = set`, `2 = delete`)
- reserved: 2 bytes, zero-filled for future compatibility
- keyLen: 4 bytes, unsigned little-endian
- valueLen: 4 bytes, unsigned little-endian
- checksum: 4 bytes, CRC32C/IEEE of the payload bytes that follow the checksum
- key/value payload: raw key bytes followed by raw value bytes

The database rejects keys and values larger than a conservative limit (`maxEntryBytes = 1 << 20`, 1 MiB) before allocation. Every record must pass length checks, checksum validation, and a magic/version/op validation gate before it is accepted as a valid mutation. A final partial record is treated as a truncated tail and ignored during recovery.

## Supported concurrency and process-access assumptions

This engine assumes:

- exactly one process opens a directory for write access at a time
- callers coordinate concurrent access at the application layer
- the same database directory is not shared by multiple writers for correctness
- the filesystem is a local POSIX-like filesystem with close-to-open consistency

The implementation is not a networked or multi-process coordination system; it intentionally avoids distributed consistency claims.

## Successful-write durability guarantees

A successful `Set` or `Delete` is considered durable only after the record bytes have been appended to the log and the file has been `fsync`'d. The implementation exposes the contract clearly: callers should treat the return from `Set`/`Delete` as the point at which the mutation is stable on a local filesystem with the usual crash semantics of `fsync`. This is a process-crash and power-loss durability claim for a single local writer, not a stronger guarantee across OS crash windows or hardware failures.

## Synchronization policy

The store uses a single mutex for all public write operations and index updates. Reads are guarded by the same lock to keep the in-memory index and file position consistent. File writes are performed in a single append operation, and the log is synced after each durable operation. This keeps the data model simple and auditable without requiring a more complex WAL or sharded architecture.

## Recovery behavior for a truncated final record

During `Open`, the database scans the log sequentially. If the final record cannot be parsed because the file ends before the full header or payload is present, the read stops and the trailing fragment is discarded. The in-memory index is rebuilt from the valid records before the truncated tail. This allows the database to recover cleanly from a crash or partial write without losing the last complete state.

## Behavior for checksum failures and interior corruption

A checksum mismatch or invalid record body is treated as corruption. The recovery logic stops scanning at the first invalid record and refuses to resurrect data beyond that point. This is intentionally conservative: it avoids silently accepting partial or mutated log content. We do not salvage data after corruption because the database design is simpler and safer when the log is treated as append-only and authoritative until a known invalid boundary.

## Compaction interruption scenarios

Compaction works by writing a fresh log to a temporary file, replaying only the live entries from the current index, `fsync`'ing the temporary file, and then swapping the files atomically with a backup step:

1. rename current log to `.bak`
2. rename temp log to active log
3. `fsync` the directory
4. remove `.bak`

If the process crashes before the swap, the old log remains intact. If it crashes after renaming the active log but before removing the backup, the database will prefer the new log and can recover the previous log from `.bak` if needed. This avoids losing the last good snapshot during compaction.

## Filesystem and operating-system assumptions

The design assumes a local Linux/macOS filesystem that supports `O_APPEND`, atomic rename, and directory fsync. The implementation uses the standard library and keeps platform-specific behavior explicit in the code comments and API docs. The store is intended for local single-host durability; it is not portable to unusual network filesystems or Windows semantics without extra validation.

## Unsupported behavior and limitations

The following are intentionally out of scope:

- replication or consensus
- SQL query language or transactions
- multi-process writer coordination
- true bounded-latency writes under heavy concurrent load
- crash-safe guarantees stronger than local single-writer durability
- automatic recovery from arbitrary interior corruption beyond the first invalid record

The design favors mechanical correctness and operational simplicity over speculative performance features.
