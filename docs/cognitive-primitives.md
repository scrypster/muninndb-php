# Cognitive Primitives

These are not features bolted onto a query engine. They are storage-layer operations — as fundamental to MuninnDB as B-tree rebalancing is to a relational database. They run continuously, in the background, on every engram in the vault. They are why MuninnDB is a different category of database.

Five primitives:
1. Temporal priority — relevance is earned, not permanent
2. Hebbian association — connections strengthen through use
3. Bayesian confidence — trust is calibrated, not binary
4. Contradiction detection — the database finds inconsistencies for you
5. Predictive Activation Signal — the database learns your retrieval sequences

Each has a mathematical foundation drawn from cognitive science. Each has an implementation that makes the math practical at database scale.

---

## 1. Temporal Priority (ACT-R Base-Level Activation)

### The Intuition

What you learned yesterday is easier to recall than what you learned a year ago — unless you have reinforced it. A fact you use every day becomes fluent. A fact you looked up once and never revisited fades. Relevance is not permanent. It is a function of time and use.

John Anderson's ACT-R (Adaptive Control of Thought — Rational) cognitive architecture formalizes this as *base-level activation*: a single number that captures how available a memory is right now, based on how often and how recently it has been accessed. ACT-R is one of the most validated cognitive architectures in psychology, used in hundreds of published studies modeling human memory retrieval.

MuninnDB implements this directly.

### The Formula

```
B(M) = ln(n + 1) - 0.5 × ln(ageDays / (n + 1))
```

- `B`: base-level activation — how cognitively available this memory is right now
- `n`: number of times the memory has been accessed
- `ageDays`: days elapsed since last access

Reading it: frequent access (high n) raises activation logarithmically. Time since last access (high ageDays) lowers it. The formula balances both — a memory used heavily but not recently still fades; a memory used once but very recently still has weight. The logarithmic scaling means the first few accesses matter most, and the benefit of additional accesses diminishes naturally.

The raw B value is passed through softplus (`ln(1 + e^x)`) to produce a smooth, non-negative activation weight used in scoring.

### Working Through the Math

A note accessed 13 times, last accessed 10 days ago:

```
B = ln(14) - 0.5 × ln(10 / 14) = 2.64 - 0.5 × (-0.34) = 2.81
softplus(2.81) ≈ 2.87
```

A note accessed once, last accessed 1,400 days ago:

```
B = ln(2) - 0.5 × ln(1400 / 2) = 0.69 - 0.5 × 6.55 = -2.58
softplus(-2.58) ≈ 0.07
```

Ratio: **~37x temporal advantage** for the actively-used note. Same content, same semantic similarity — completely different cognitive weight.

### Why This Beats TTL

The common alternative to principled temporal scoring is a fixed time-to-live: delete everything older than 90 days. TTL is blunt — it treats a fact accessed daily and a fact from a single ingestion identically. It cannot distinguish between a current active policy and an obsolete note from three years ago. It destroys data that is still relevant because it is old.

ACT-R activation is adaptive. The scoring is calibrated to actual use. Relevance tracks reality.

### Implementation

ACT-R activation is computed at query time, not by a background worker. Every engram stores its access count and last-access timestamp. When a query arrives, the activation engine computes B(M) from these stored values and the current wall clock. No stored activation score is ever mutated by a background process.

This is a total-recall design: nothing is ever degraded in storage. The same engram, queried at the same moment, always produces the same score. There is no floating-point drift, no accumulated error, no ordering dependency between worker cycles. The engine can be restarted, replayed, or migrated, and activation scores are identical — because they are derived, not stored.

---

## 2. Hebbian Association Learning

### The Intuition

"Neurons that fire together, wire together."

Donald Hebb proposed this principle in 1949 as a mechanism for how the brain learns associations. When two neurons activate together repeatedly, the synaptic connection between them strengthens. When they stop firing together, the connection weakens. Over time, the network encodes which concepts tend to appear together — which is a large part of what expertise is.

MuninnDB implements this directly at the association layer.

### The Mechanism

Every ACTIVATE call produces a co-activation signal. When the activation engine returns a result set containing engrams A, B, and C together, the Hebbian worker receives co-activation events for the pairs (A, B), (A, C), and (B, C). It processes these events and updates the association weights between the paired engrams.

Over time, engrams that are frequently retrieved together develop strong weighted associations. The BFS traversal in Phase 5 of the activation engine then exploits these associations — strong connections are explored first, and connected engrams are surfaced even if they did not appear in the original retrieval.

### The Formula

```
w_new = min(1.0, w_old × (1 + η)^n)
```

- `w`: association weight, 0.0–1.0
- `η`: learning rate (`HebbianLearningRate = 0.01`)
- `n`: number of recent co-activations in this batch

**Unpacking — `(1 + η)^n`:**

Each co-activation multiplies the weight by `(1 + 0.01)`. With n=3 co-activations in a batch, the weight is multiplied by 1.030301. The result is capped at 1.0 so weights remain bounded.

Why multiplicative rather than additive? Additive updates can push weights above their bounds without capping. Multiplicative updates are naturally bounded in [0, 1] when the base is in [0, 1] and the multiplier is ≥1. The weight converges toward 1.0 asymptotically as activations accumulate.

**Score weighting:**

The update signal is the geometric product of both engrams' activation scores at the time of co-activation. If engram A activated with score 0.9 and engram B activated with score 0.3, the co-activation signal is `sqrt(0.9 × 0.3) = 0.52`. High-confidence co-activations produce stronger associations than low-confidence ones. The Hebbian update reflects the quality of the co-activation, not just its occurrence.

### Canonical Pair Keys

Associations are bidirectional but stored once per pair. The canonical key is:

```
key = (min(idA, idB), max(idA, idB))
```

Since ULIDs are lexicographically comparable, this produces a unique, consistent key for any ordered pair. The Hebbian worker deduplicates co-activation events for the same pair within a batch before applying the weight update. This prevents double-counting from a single activation event appearing twice in the event stream.

### 15 Typed Relationship Types

Association weights encode *how strongly* two engrams are related. Relationship types encode *how* they are related.

MuninnDB has 15 built-in relationship types: `supports`, `contradicts`, `depends_on`, `supersedes`, `relates_to`, `is_part_of`, `causes`, `preceded_by`, `followed_by`, `created_by_person`, `belongs_to_project`, `references`, `implements`, `blocks`, `resolves` — covering the common patterns in technical, organizational, and factual knowledge. User-defined types occupy the `0x8000+` range for domain-specific relationships.

Typed relationships allow the association graph to carry semantic meaning. "Service A depends_on Service B" is different from "Service A contradicts Service B" even if both associations have the same weight.

### Why This Matters

You never have to explicitly define that "payment service" and "idempotency keys" are related. Activate them together enough times — in context, in activation results — and the connection emerges automatically with a weight proportional to co-activation frequency and quality. Stop thinking about them together and the connection weakens.

This is expertise, encoded in graph structure.

---

## 3. Bayesian Confidence Updating

### The Intuition

Not all memories are equally trustworthy. Something you have verified dozens of times and never found contradicted is more reliable than something you heard once in a meeting. When you encounter information that contradicts something you believe, your confidence in the original belief should drop. When you encounter reinforcing information, it should rise.

The update should be calibrated. Strong contradiction of a high-confidence belief should cause a larger update than weak contradiction of an already-uncertain belief. The magnitude of the update should depend on both how confident you were and how strong the new signal is.

This is what Bayesian updating does.

### The Formula

```
posterior = (p × s) / (p × s + (1 - p) × (1 - s))
```

- `p`: current confidence (the prior), 0.0–1.0
- `s`: signal strength, 0.0–1.0
  - s=1.0: strong reinforcement (high-confidence corroborating write)
  - s=0.5: neutral (no update)
  - s=0.0: strong contradiction (direct logical negation)

With Laplace smoothing:

```
confidence = 0.95 × posterior + 0.025
```

The smoothing term prevents confidence from ever reaching exactly 0.0 or 1.0. This is not a fudge — it is statistically correct. No finite amount of evidence should make a memory perfectly certain or perfectly disproven. The bounds [0.025, 0.975] are the effective range.

### Working Through the Math

Consider an engram with `p=0.8` (high confidence) that receives a strong contradiction signal `s=0.1`:

```
posterior = (0.8 × 0.1) / (0.8 × 0.1 + 0.2 × 0.9)
          = 0.08 / (0.08 + 0.18)
          = 0.08 / 0.26
          ≈ 0.31
```

With Laplace smoothing: `0.95 × 0.31 + 0.025 ≈ 0.32`

A high-confidence belief drops from 0.8 to 0.32 under strong contradiction. That is a large drop — appropriate for a strong signal contradicting a confident belief.

Now consider the same contradiction against a low-confidence engram with `p=0.4`:

```
posterior = (0.4 × 0.1) / (0.4 × 0.1 + 0.6 × 0.9)
          = 0.04 / (0.04 + 0.54)
          = 0.04 / 0.58
          ≈ 0.07
```

With Laplace smoothing: ≈ 0.09

A low-confidence belief drops further (0.4 → 0.09) because there was less evidence supporting it to begin with. The formula is calibrated: the update magnitude depends on the prior.

### How Confidence Enters Scoring

In Phase 6 of the activation engine, every composite score is multiplied by the engram's confidence:

```
final_score = composite_score × confidence
```

An engram with confidence 0.3 appears in results, but at 30% of the score it would receive at full confidence. It ranks lower. It is not excluded — it is appropriately discounted. Callers who want to filter by confidence threshold can do so, but the default behavior is to include uncertain memories with reduced weight rather than hide them entirely.

### What Triggers Confidence Updates

**Contradiction events:** When the contradiction worker detects a contradiction, both engrams involved receive a Bayesian update with a low signal strength (toward 0.5 mutual uncertainty). The confident one drops more.

**Reinforcing writes:** When a new write produces a concept-cluster match with an existing engram — same concept, overlapping content, compatible confidence — the existing engram receives a positive signal. Its confidence increases.

**Explicit API updates:** Clients can directly set confidence with a justification string. Human-in-the-loop corrections are first-class operations.

---

## 4. Contradiction Detection

### Why At the Storage Layer

Application-level contradiction detection is reactive. You write two contradicting facts into the database. You find out they contradict each other when you query and notice the conflict — if you notice at all.

Storage-layer contradiction detection is proactive. The database finds it. Flags it. Updates both engrams' confidence scores. Creates a typed `contradicts` association between them. Pushes a notification to any subscribers that have these engrams in their activation set. You don't have to ask.

This is the difference between a database that stores facts and a database that understands them.

### Three Detection Modes

**Structural Contradiction**

Two relationship types can be structurally incompatible — for example, `supports` and `contradicts` cannot both be true of the same pair. The detection mechanism is a 64×64 boolean matrix initialized at startup with the known incompatible pairs. Looking up whether two relationship types conflict is an O(1) matrix access.

The matrix does not grow with vault size. It is a fixed lookup table encoding which relationship type combinations are logically impossible. When the contradiction worker detects that two engrams hold structurally incompatible relationship types with a common third engram, it fires a contradiction event.

**Concept-Cluster Contradiction**

Engrams in the same concept cluster — similar concepts, overlapping tags, related content — with contradicting claims. Detected via FTS overlap scoring during write. When a new engram scores highly against existing engrams in the inverted index, the contradiction worker examines the top matches for semantic divergence.

This catches contradictions that are not identical in concept but are topically related. "Our API uses OAuth 2.0" and "Authentication is handled by API keys" may not have identical concept fields, but they are close enough to examine.

**Semantic Contradiction**

Some contradictions are not structurally obvious. "We deploy to AWS" and "All infrastructure runs on Google Cloud" are structurally similar — both are declarative statements about infrastructure. They are semantically contradictory. Pattern-matching cannot catch this.

Semantic contradiction detection requires the enrich plugin. The plugin sends candidate pairs to an LLM for logical contradiction analysis. The LLM determines whether the two engrams make logically incompatible claims. The result is sent back as a contradiction event.

This is the most expensive detection mode and the most powerful. It catches contradictions that no structural analysis can.

### What Happens On Detection

1. **Immediate trigger fire.** The trigger system receives a contradiction event at highest priority. Not rate-limited. If any subscription matches these engrams, it fires immediately.

2. **Bayesian confidence updates.** Both engrams' confidence scores are updated toward 0.5 mutual uncertainty, using the Bayesian formula with a low signal strength. The more confident engram drops more than the less confident one.

3. **Typed association created.** A `contradicts` association is created between the two engrams with weight proportional to the strength of the contradiction evidence. This association is visible in subsequent BFS traversal — activation results that include one engram will note the contradiction with the other.

4. **Subscription push.** Any active subscription with these engrams in its activation window receives a push notification. Consumers subscribed to relevant topics learn about the contradiction without polling.

### The Practical Effect

An AI agent using MuninnDB as memory writes facts continuously. Contradictions emerge naturally over time — policies change, systems are updated, decisions are reversed. Without contradiction detection, the memory accumulates contradicting claims at equal confidence, and the agent has no way to know. With contradiction detection, the contradictions are flagged as they form, both confidence scores are reduced, and the agent is notified. The memory is self-correcting.

This is not a minor quality-of-life feature. In any system that accumulates knowledge over time, contradiction management is the difference between a reliable memory and a gradually degrading one.

---

## 5. Predictive Activation Signal (PAS)

### The Intuition

Memories are not recalled in isolation. They follow patterns. When you remember your morning login procedure, you next recall the dashboard. When you think about a customer complaint, you then think about the resolution policy. These sequences are learned: the brain tracks which memories follow which, and uses that history to pre-activate the next likely memory before you consciously request it.

This is what cognitive scientists call *sequential activation tracking* — the observation that activation N predicts activation N+1 with high probability when the same sequence has occurred before. The brain builds lightweight transition probability tables that bias retrieval toward procedurally connected memories.

MuninnDB implements this as the Predictive Activation Signal (PAS).

### How It Works

Every activation produces a result set. PAS records the transition from the previous activation's result set to the current one: if activation N returned engrams {A, B} and activation N+1 returned engrams {C, D}, PAS records the transitions A→C, A→D, B→C, B→D. Each transition increments a counter in the transition table.

Over time, the transition table learns which memories tend to follow which. When the same vault later activates engram A, PAS looks up A's top transition targets and injects them as candidates in Phase 2 of the ACTIVATE pipeline — before RRF fusion, before scoring. These injected candidates flow through the full scoring pipeline alongside standard retrieval results.

The key insight: PAS doesn't just re-rank existing results. It *surfaces memories that would not otherwise appear*. If the query is about "login procedures" and PAS has learned that "dashboard navigation" always follows, the dashboard engram enters the candidate pool even if it has zero semantic similarity to the query. This is candidate *expansion*, not re-ranking — and it is the mechanism that produces real recall improvements.

### The Architecture

PAS is implemented as a three-layer system:

**Transition recording.** The `TransitionWorker` (a background cognitive worker following the same pattern as the HebbianWorker) receives transition events from the engine after each activation. It aggregates (source → destination) pairs and writes them to the transition cache.

**Tiered storage.** The `TransitionCache` provides a fast in-memory hot tier backed by Pebble persistence. Increments are O(1) in-memory operations that never touch disk. Periodic flushes write merged totals to Pebble for durability. Cold sources are loaded from Pebble on first access via read-through caching.

**Retrieval integration.** During Phase 2 of the ACTIVATE pipeline, if PAS is enabled for the vault, the engine looks up transition candidates from the previous activation's result set. These candidates enter RRF fusion with their own rank constant (K=50, between HNSW and FTS). Phase 4.5 applies a `transitionBoost` to candidates that match known transition targets, and the boost flows into the ACT-R scoring formula alongside Hebbian boost.

### The Scoring Formula

PAS integrates into ACT-R scoring as an additive term alongside Hebbian boost:

```
totalActivation = baseLevel + scale × hebbianBoost + scale × transitionBoost
Score = ContentMatch × softplus(totalActivation) × Confidence
```

The transition boost is normalized to [0, 1] by dividing the candidate's transition count by the maximum count across all transition targets. This ensures the boost is proportional to how strongly the transition has been learned.

### Configuration

PAS is configurable per vault via the Plasticity system:

- `PredictiveActivation` (boolean, default: true) — enables or disables PAS for the vault
- `PASMaxInjections` (integer, default: 5, clamped to [1, 20]) — maximum number of transition candidates injected per activation

Setting `PredictiveActivation: false` completely disables PAS for a vault — no transitions are recorded and no candidates are injected.

### When PAS Matters

PAS produces its strongest gains in workflow-oriented use cases: agent task execution, personal assistant interactions, procedural memory, and any scenario where the user follows repeatable sequences. In controlled experiments with 2,000 synthetic items modeling agent and personal assistant workflows:

- **Recall@10 improved by 21%** — transition-predicted items that would never appear in standard retrieval were surfaced
- **MRR improved by 10-15%** — the right next-step memory ranked higher

PAS is less impactful for one-shot semantic queries with no sequential context. It is most valuable when the same vault is used repeatedly in patterns — exactly the use case that AI agents and personal assistants represent.

---

## How The Primitives Interact

These four primitives are not independent. They form a feedback loop.

An engram is written with full confidence and high initial activation.

Over time, if it is not accessed, its ACT-R activation score — computed at query time from access count and recency — naturally decreases. It appears less frequently in activation results.

If it is accessed frequently, each access increases n in the ACT-R equation, making the memory more cognitively available. Access patterns strengthen it.

When it co-activates with other engrams, the Hebbian worker strengthens the associations between them. Future activations that find one of the connected engrams will traverse the association graph and surface the others.

When a contradicting write arrives, the contradiction worker fires. Both engrams' confidence scores drop via Bayesian update. Their final activation scores — composite score × confidence — decrease. They rank lower in results.

The database is continuously reorganizing itself around what is used, what is trusted, what is reinforced, and what is contradicted. The cognitive state of the vault reflects the history of how it has been used.

This is what separates a cognitive database from a storage layer with search bolted on.

---

**See also:** [Retrieval Design](retrieval-design.md) · [Engram](engram.md) · [How Memory Works](how-memory-works.md)
