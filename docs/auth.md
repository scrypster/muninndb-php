# Access & Authentication

MuninnDB uses a two-layer auth model. The layers are separate because they serve different actors with different needs.

---

## Why this is different from other databases

In every traditional database, **reads are transparent**. A `SELECT` in Postgres doesn't modify row weights. A `GET` in Redis doesn't affect TTLs. You can give someone read access and be certain that their queries won't alter the data landscape for anyone else.

In MuninnDB, **reads are not transparent**. When you activate a memory:

- Its access count increases, which raises its stability score
- The Hebbian weights between co-activated engrams strengthen
- Its decay timer resets — it becomes "recent" again
- RRF fusion scores shift for the next retrieval by anyone in that vault

This is the cognitive model working correctly. A brain that remembers something is not the same brain it was before. But it means that a careless read-only consumer can silently reshape the vault's learned relevance for every other user.

The auth model is designed around this reality.

---

## Layer 1 — Admin credentials

Admin users access the system operator layer: the web UI, the shell (`muninn shell`), and vault management endpoints. They do not normally interact with vault data directly — that is what API keys are for.

**First run:** MuninnDB prints a generated root password on the first startup. Save it. Change it via the web UI afterward. The password is never printed again.

```
┌─────────────────────────────────────────┐
│         MuninnDB — First Run Auth        │
│                                          │
│  Admin username: root                    │
│  Admin password: xK9mP2nQ4rT7wY1aZb3c   │
│                                          │
│  Change this password after first login. │
└─────────────────────────────────────────┘
```

Admin credentials authenticate to:
- **Web UI** — session cookie, 24-hour TTL
- **Shell** — prompted at `muninn shell`, no session stored

---

## Layer 2 — Vault API keys

A vault is either **open** (no auth required) or **locked** (API key required, default). Unconfigured vaults default to fail-closed: they require an API key for any access. To allow unauthenticated access to a vault, explicitly configure it with `public: true` via SetVaultConfig.

A vault can have multiple API keys — one per integration point. You might have:

```
vault: default
  mk_abc...  label: "ai-agent"         mode: full
  mk_def...  label: "analytics-dash"   mode: observe
  mk_ghi...  label: "backup-exporter"  mode: observe
```

### Key modes

| Mode | Reads | Cognitive state writes | Use case |
|------|-------|------------------------|----------|
| `full` | Yes | **Yes** — decay resets, Hebbian weights update, access counts increment | AI agents, primary integrations, anything that is *part of* the brain |
| `observe` | Yes | **No** — scores are computed but nothing is persisted | Dashboards, analytics, read-only partners, exports |

The `observe` mode exists because the vault's cognitive state is the thing of value. A dashboard reading engrams 1000 times a day should not inflate access counts and distort what the AI agent sees as relevant. `observe` keys see the brain; they don't affect it.

### Key format

```
mk_xK9mP2nQ4rT7wY1aZb3cD5eF6gH7iJ8kL9m
│  └─────────────────────────────────────── 32 random bytes, base64url encoded
└── prefix identifies MuninnDB API keys
```

Keys are 46 characters. The raw bytes are generated with `crypto/rand`. The token itself is the credential — MuninnDB stores only a SHA-256 hash of the raw bytes, so a compromised database file does not expose valid tokens.

**Tokens are shown once at creation and never again.** Copy them immediately.

### Using a key

Include the key as a bearer token on every request:

```bash
curl http://localhost:8475/api/engrams?vault=default \
  -H "Authorization: Bearer mk_xK9m..."
```

The key implicitly identifies the vault. If the key belongs to `default` and the request specifies a different vault, the request is rejected.

---

## Managing keys

### Create a key (admin only)

```bash
curl -X POST http://localhost:8475/api/admin/keys \
  -H "Content-Type: application/json" \
  -d '{
    "vault": "default",
    "label": "my-agent",
    "mode": "full"
  }'
```

Response:
```json
{
  "token": "mk_xK9m...",
  "key": {
    "id": "A1B2C3D4",
    "vault": "default",
    "label": "my-agent",
    "mode": "full",
    "created_at": "2026-02-20T..."
  }
}
```

### List keys for a vault

```bash
curl "http://localhost:8475/api/admin/keys?vault=default"
```

Token values are not returned. You see the key metadata (ID, label, mode, created date) only.

### Revoke a key

```bash
curl -X DELETE "http://localhost:8475/api/admin/keys/A1B2C3D4?vault=default"
```

Revocation is immediate. The token stops working on the next request.

---

## Vault configuration

### Vault access control

Unconfigured vaults default to fail-closed and require an API key for all access. To allow unauthenticated (public) access:

```bash
curl -X PUT http://localhost:8475/api/admin/vaults/config \
  -H "Content-Type: application/json" \
  -d '{"name":"default","public":true}'
```

To require authentication (lock a vault):

```bash
curl -X PUT http://localhost:8475/api/admin/vaults/config \
  -H "Content-Type: application/json" \
  -d '{"name":"default","public":false}'
```

**Every unconfigured vault requires an API key.** You must explicitly opt in to public (unauthenticated) access via SetVaultConfig.

### Per-vault plasticity configuration

Plasticity controls the cognitive pipeline for a vault — how it learns, forgets, and traverses connections between engrams. Each vault can have its own plasticity settings independent of others.

**Four preset profiles** are available: `default` (balanced Hebbian + decay), `reference` (preserves with strong Hebbian bonds), `scratchpad` (rapid decay, minimal history), and `knowledge-graph` (rich traversal, strong associative learning).

Get the current plasticity configuration for a vault:

```bash
curl "http://localhost:8475/api/admin/vault/default/plasticity" \
  -H "Authorization: Bearer <admin-session>"
```

Response includes both the saved configuration and the fully resolved values (preset merged with overrides):

```json
{
  "config": {
    "preset": "default",
    "hebbian_enabled": true,
    "decay_stability": 30
  },
  "resolved": {
    "hebbian_enabled": true,
    "decay_enabled": true,
    "hop_depth": 2,
    "semantic_weight": 0.6,
    "fts_weight": 0.3,
    "decay_floor": 0.05,
    "decay_stability": 30,
    "hebbian_weight": 0.5,
    "decay_weight": 0.4,
    "recency_weight": 0.3,
    "traversal_profile": ""
  }
}
```

Update plasticity for a vault:

```bash
curl -X PUT "http://localhost:8475/api/admin/vault/default/plasticity" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <admin-session>" \
  -d '{
    "preset": "knowledge-graph",
    "decay_stability": 60,
    "traversal_profile": "causal"
  }'
```

**Configuration fields:**

| Field | Type | Range | Purpose |
|-------|------|-------|---------|
| `preset` | string | `default` \| `reference` \| `scratchpad` \| `knowledge-graph` | Base cognitive profile; overrides applied on top |
| `hebbian_enabled` | bool | — | Enable/disable Hebbian weight updates (coactivation learning) |
| `decay_enabled` | bool | — | Enable/disable time-based decay of access scores |
| `hop_depth` | int | 0–8 | BFS hops for associative retrieval; higher = broader context |
| `semantic_weight` | float | 0–1 | Multiplier for semantic similarity in fusion scoring |
| `fts_weight` | float | 0–1 | Multiplier for full-text keyword match scoring |
| `decay_floor` | float | 0–1 | Minimum stability score; prevents total decay |
| `decay_stability` | float | >0 | Days before an engram reaches half-life |
| `traversal_profile` | string | `default` \| `causal` \| `confirmatory` \| `adversarial` \| `structural` | Link traversal strategy; empty = auto-infer |

---

## The one brain principle

A vault is a single cognitive entity. All connections with `full` keys participate in that entity's learned state equally — there is no per-user relevance. The vault's access patterns, Hebbian weights, and decay scores reflect the collective behavior of every `full` connection.

This is a deliberate design decision:

**Why not per-user cognitive state?**

If each user had their own relevance weights, the vault would have N brains instead of one. You'd lose the emergent collective intelligence — the thing that makes a shared knowledge base useful is that everyone's usage teaches the system what matters to the group. Per-user weights would also multiply storage requirements for every engram by the number of users.

**If you need isolation, use separate vaults.** A vault per service, per project, or per person gives you isolated cognitive states. Multiple keys into one vault gives you a shared brain with controlled access.

---

## Security properties

| Property | Detail |
|----------|--------|
| Token storage | SHA-256 hash only — plaintext never persisted |
| Admin passwords | bcrypt with default cost |
| Session tokens | HMAC-SHA256 signed, 24h TTL, HttpOnly cookie |
| Transport | HTTP by default; run behind TLS-terminating proxy in production |
| Key revocation | Immediate, no grace period |
| Observe isolation | Enforced in engine activation layer — not just an honor system |

---

## Migration from unauthenticated installations

Existing vaults default to `public: true`. Nothing breaks. You add auth incrementally:

1. Admin user is created on first run with the new binary
2. All existing vaults continue to work without keys
3. Lock specific vaults by setting `public: false` via the admin API
4. Generate keys for your integrations
5. Update your integrations to include `Authorization: Bearer mk_...`

You can lock vaults one at a time while rolling out keys to your services.
