# Vault Coherence Score

## What it does

MuninnDB continuously tracks the health of each vault's memory graph using four incremental counters. These combine into a single 0.0–1.0 coherence score that tells you how "well-organized" the vault's memory is.

**Score of 1.0** = perfectly organized: all engrams are connected, no contradictions, no duplicates, uniform confidence.
**Score of 0.0** = maximum disorder: all engrams are isolated orphans with contradictions and duplicates everywhere.

## Why it matters

Most databases give you storage metrics (bytes used, index size, row count). MuninnDB gives you a **cognitive health metric** — a measure of whether your memory is actually useful, not just filled. A low coherence score is an actionable signal:

- **High orphan ratio** → write more engrams with tags, or manually link existing ones.
- **High contradiction density** → run contradiction resolution.
- **High duplication pressure** → call `CONSOLIDATE` on near-duplicate clusters.
- **High temporal variance** → some memories are fading while others are active; consider archiving stale ones.

## How it works

### Incremental Counters (no full scans)

Coherence is computed in O(1) from atomic counters updated at every write/link/contradiction event:

| Counter | Updated when |
|---------|-------------|
| `TotalEngrams` | Every write |
| `OrphanCount` | Write (↑), first link created (↓), last link deleted (↑) |
| `Contradictions` | Contradiction detected (↑), resolved (↓) |
| `RefinesCount` | REFINES link created (↑), deleted (↓) |
| `ConfidenceSum/SumSq` | Every write + confidence update (Welford variance) |

### Formula

```
coherence = 1.0 - (orphanRatio × 0.3
                 + contradictionDensity × 0.3
                 + temporalVariance × 0.2
                 + duplicationPressure × 0.2)
```

All components are clamped to [0, 1] before combining.

## What is new / never done before (to our knowledge)

A **cognitive health score native to the memory model** has not been implemented in any database we are aware of. Traditional database health metrics measure infrastructure (disk I/O, query time, cache hit rate). MuninnDB's coherence score measures the **semantic quality of the stored knowledge** — a fundamentally different class of metric that only makes sense in a cognitive data model.

## API

Coherence scores appear in the `GET /api/stats` response:

```json
{
  "engram_count": 1842,
  "vault_count": 3,
  "storage_bytes": 4194304,
  "coherence": {
    "default": {
      "score": 0.73,
      "orphan_ratio": 0.12,
      "contradiction_density": 0.03,
      "duplication_pressure": 0.08,
      "temporal_variance": 0.41,
      "total_engrams": 1200
    },
    "research": {
      "score": 0.91,
      ...
    }
  }
}
```

## Python SDK

```python
stats = await client.stats()
for vault_name, coh in (stats.coherence or {}).items():
    print(f"Vault '{vault_name}': coherence={coh.score:.2f} orphans={coh.orphan_ratio:.1%}")
```
