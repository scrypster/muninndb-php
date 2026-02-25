# Write-Time Novelty Detection

## What it does

Every time you write an engram, MuninnDB computes a Jaccard fingerprint of the new engram's text and compares it against a per-vault in-memory cache of recently written engrams. If the similarity exceeds 0.70, MuninnDB:

1. Creates a `REFINES` association from the new engram to the similar existing one.
2. The new engram is **always written** — novelty detection never suppresses writes.

## Why it matters

Duplicate and near-duplicate memories are a real problem in cognitive systems. Without detection, you end up with memory fragmentation: the same fact stored slightly differently across dozens of engrams, each accumulating separate Hebbian boost, each independently fading. Near-duplicate detection creates explicit links between related memories so activation can traverse the cluster, and duplication pressure shows up in the vault coherence score as a signal to consolidate.

## How it works

### Fingerprinting

The top-30 most frequent terms from the engram's concept and content (after stop-word filtering) form the fingerprint:

```
"The brain's ability to learn. Neurons that fire together wire together."
→ ["brain", "ability", "learn", "neurons", "fire", "together", "wire"]
```

### Jaccard Similarity

```
similarity = |A ∩ B| / |A ∪ B|
```

Two fingerprints with 21 terms in common out of 39 unique = 0.538 (below threshold).
Two fingerprints with 27 terms in common out of 33 unique = 0.818 (match).

### LRU Cache

- 16 shards (reduces lock contention under concurrent writes)
- 1000 entries per vault per shard
- LRU eviction on overflow
- In-memory only (does not persist across restarts — graceful degradation)
- Background warm-up on first vault access populates recent engrams

### Known Limitation

Two near-identical engrams written concurrently within microseconds of each other may both miss in the cache (TOCTOU race). This is a documented best-effort guarantee; the cognitive worker's contradiction detection provides a secondary safety net.

## What is new / never done before (to our knowledge)

Novelty detection at the **database layer** (not the application layer) using a sharded LRU fingerprint cache is, to our knowledge, not implemented in any existing database system. Most deduplication happens either at the application layer (comparing before writing) or via full-table fuzzy matching after the fact. MuninnDB does it in-band at write time, in sub-millisecond overhead.

## Configuration

Tuning constants in `internal/engine/novelty/novelty.go`:

| Constant | Default | Description |
|----------|---------|-------------|
| `TopTerms` | 30 | Terms per fingerprint |
| `Threshold` | 0.70 | Jaccard threshold for match |
| `CacheSize` | 1000 | LRU capacity per vault |
| `NumShards` | 16 | Shards for concurrency |
