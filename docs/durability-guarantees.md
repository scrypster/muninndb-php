# Durability Guarantees

MuninnDB uses Pebble (an LSM-tree engine) with a two-tier durability model. Core data — engrams, scoring weights, provenance, episodic frames, and auth — is fsynced to disk on every write. Everything else rides a WAL group-commit pattern that syncs every 10ms, trading a tiny data-loss window for significantly higher throughput on secondary indexes and metadata updates.

After a crash, operators should expect all engrams and audit records to be intact. Secondary indexes (associations, FTS postings, entity graph edges) may lose up to 10ms of writes and will self-heal on the next relevant operation.

---

## The Two Durability Tiers

| Operation | Tier | Data-Loss Window | Notes |
|---|---|---|---|
| `WriteEngram` / `WriteBatch` | Sync | Zero | Default behavior. Configurable via `NoSyncEngrams`. |
| `scoring/Store.Save` | Sync | Zero | Vault-level Hebbian weights. |
| `provenance/Store.Append` | Sync | Zero | Append-only audit trail. |
| `episodic/AppendFrame` | Sync | Zero | Atomic frame + FrameCount batch. |
| `DeleteEngram` vault count key (0x15) | Sync | Zero | Quota enforcement — must be crash-safe. |
| Auth writes | Sync | Zero | API keys, tokens. |
| Migration writes | Sync | Zero | Schema version bumps. |
| `WriteAssociation` | NoSync + WAL syncer | ≤10ms | Association edges (forward, reverse, weight). |
| `UpdateMetadata` | NoSync + WAL syncer | ≤10ms | Access counts, last-access timestamps, lifecycle state. |
| `UpdateRelevance` | NoSync + WAL syncer | ≤10ms | Relevance and stability scores. |
| `SoftDelete` / `DeleteEngram` key deletes | NoSync + WAL syncer | ≤10ms | Tombstone writes. |
| Entity graph writes | NoSync + WAL syncer | ≤10ms | UpsertEntityRecord, WriteEntityEngramLink, UpsertRelationshipRecord, IncrementEntityCoOccurrence. |
| `episodic/CreateEpisode`, `CloseEpisode` | NoSync + WAL syncer | ≤10ms | Episode lifecycle (not frame data). |
| FTS index updates | NoSync + WAL syncer | ≤10ms | Posting lists, trigrams, term stats. |
| Idempotency receipts | NoSync + WAL syncer | ≤10ms | Duplicate-request guard. |
| All secondary indexes | NoSync + WAL syncer | ≤10ms | Tag, state, relevance bucket, creator, last-access. |

---

## The WAL Group-Commit Pattern

Pebble's WAL (write-ahead log) captures every mutation before it reaches the memtable. When MuninnDB issues a `pebble.NoSync` write, Pebble still appends the mutation to the WAL — it just skips the fsync syscall that forces the kernel to flush its page cache to stable storage.

A background goroutine (the **WAL syncer**) calls `db.LogData(nil, pebble.Sync)` every 10ms. This single fsync flushes every WAL entry that accumulated in the preceding interval, amortizing one expensive syscall across hundreds or thousands of mutations.

The analogy is straightforward: this is the same strategy as MySQL's `innodb_flush_log_at_trx_commit=2` or PostgreSQL's `synchronous_commit=off`. Individual writes return instantly; a background thread periodically guarantees they reach disk. The maximum data-loss window equals the sync interval — 10ms.

On clean shutdown, the WAL syncer performs a final sync before exiting. No data is left in an ambiguous state.

---

## What Survives a Crash

All of these are intact after an unclean shutdown:

- Every engram body and metadata slice written with `pebble.Sync` (the default).
- Vault-level Hebbian scoring weights.
- The full provenance (audit) trail.
- Episodic frames and their frame counts (atomic batch, synced).
- Vault engram counts (quota enforcement).
- Auth credentials and migration markers.
- Any NoSync write that was captured by a WAL syncer cycle that completed before the crash.

---

## What May Roll Back

These categories may lose the most recent ≤10ms of writes after a crash:

- **Association edges** — the next Hebbian learning pass will re-derive them from engram content.
- **Metadata counters** (access count, last-access timestamp) — statistical, not correctness-critical. The next read updates them.
- **Relevance and stability scores** — recomputed by the confidence worker on its next pass.
- **Entity graph records** — entity extraction re-runs on the next relevant write or enrichment pass.
- **FTS postings and trigram indexes** — a vault-level `ReindexFTSVault` rebuilds them from source engrams. Under normal operation, the next write to the same terms also corrects stale entries.
- **Secondary indexes** (tag, state, relevance bucket, creator) — derived from engram metadata; self-healing on the next metadata update.
- **Idempotency receipts** — a lost receipt means a duplicate request might be processed twice. The window is ≤10ms; in practice, clients retry on the order of seconds.
- **Episode lifecycle records** (create/close, not frames) — the episode can be re-closed on the next session boundary.

This is acceptable because every item in the NoSync tier is either derived from Sync-tier source data, statistical, or has a bounded blast radius measured in single-digit milliseconds.

---

## Configuration

The `PebbleStoreConfig.NoSyncEngrams` flag controls engram write durability:

| `NoSyncEngrams` | `WriteEngram` / `WriteBatch` Behavior | Use Case |
|---|---|---|
| `false` (default) | `pebble.Sync` — zero data loss | Production default. Engrams are the source of truth. |
| `true` | `pebble.NoSync` — ≤10ms window | High-ingest pipelines where throughput matters more than per-engram durability. The WAL syncer still guarantees data reaches disk within 10ms. |

No other durability behavior is configurable. The WAL sync interval (10ms) is a compile-time constant (`walSyncInterval`).

---

## Operational Guidance

**WAL syncer health.** The WAL syncer logs failures at `slog.Error` level with `"component", "wal_syncer"`. If these appear, the 10ms durability guarantee for the NoSync tier is degraded. Investigate disk I/O latency and Pebble compaction pressure.

**Post-crash verification.** After an unclean shutdown, check the application log for WAL syncer errors in the seconds before the crash. If the syncer was healthy, the effective data-loss window is ≤10ms. If the syncer was failing, the window extends to the last successful sync.

**Shutdown sequence.** A clean shutdown performs a final WAL sync. If the process is terminated with `SIGKILL`, the final sync does not run — treat this as a crash scenario with the ≤10ms window.

**Quota accuracy.** The vault engram count (0x15) is Sync-tier specifically because quota enforcement must survive crashes. If a crash occurs mid-delete, the count may be slightly higher than the actual number of engrams (conservative direction). It self-corrects on the next write or explicit recount.

---

**See also:** [Key-Space Schema](key-space-schema.md) · [Cluster Operations](cluster-operations.md)
