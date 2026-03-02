# How Memory Works

*The neuroscience behind MuninnDB's architecture — why this model is correct, not just clever.*

---

## The question databases never asked

Every database ever built answers the same question: "What data do I have?"

MuninnDB answers a different question: "What do I *remember*?"

These sound similar. They are not.

A library and a brain are both ways of storing information. A library is static. Every book is equally present, equally accessible, equally relevant. The library doesn't know which shelf you've visited most. It doesn't know that two books are about the same idea. It won't pull a book off the shelf and set it on your desk because you might need it. You go to the library, ask for something specific, and the library returns it exactly.

A brain is dynamic. It knows that what you thought about yesterday is more present than what you thought about a year ago. It discovers connections between ideas you never consciously linked. It surfaces memories you didn't know you needed. It forgets what stopped mattering.

Every database before MuninnDB was a library. Perfectly reliable. Perfectly static.

MuninnDB is a brain. It does something no database has ever done.

---

## ACT-R and base-level activation

How does a brain decide what to remember right now?

Cognitive scientists have studied this for over a century. In 1885, Hermann Ebbinghaus discovered that forgetting follows a precise mathematical curve — you lose most of a memory quickly, then the rate slows. In 1998, John Anderson's ACT-R (Adaptive Control of Thought — Rational) theory refined this into something more powerful: a unified equation that captures both *how often* and *how recently* you've used a memory, and turns those into a single activation score.

The intuition is simple. A memory you've accessed 13 times in the last 10 days is far more "present" than a memory you looked up once four years ago. ACT-R quantifies exactly how much more present:

```
B(M) = ln(n + 1) - 0.5 × ln(ageDays / (n + 1))
```

Where:
- **B** is the base-level activation — how cognitively available this memory is right now
- **n** is the number of times the memory has been accessed
- **ageDays** is the number of days since the last access

Reading it: frequent access (high n) raises activation logarithmically. Time since last access (high ageDays) lowers it. The formula balances both — a memory used heavily but not recently still fades; a memory used once but very recently still has weight. The logarithmic scaling means the first few accesses matter most, and the benefit of additional accesses diminishes naturally.

**A concrete example.** A note accessed 13 times, last accessed 10 days ago: softplus(B) ≈ 2.6. A note accessed once, last accessed 1,400 days ago: softplus(B) ≈ 0.07. That's a **37x temporal advantage** for the actively-used note. Same content, same semantic similarity — completely different cognitive weight.

MuninnDB implements this directly. Every engram stores its access count and last-access timestamp. At query time, the activation engine computes B(M) from these values and the current wall clock — no background worker mutates stored scores. An engram you haven't activated in six months scores lower than it did last week, not because something degraded it, but because the formula produces a lower value when more time has passed. This is a total-recall design: nothing is ever lost in storage, and the same engram queried at the same moment always produces the same score.

One design choice deserves attention: activation never makes a memory completely invisible. Nothing is ever truly deleted — engrams become dormant. They can be reactivated. This matches biological reality: you don't permanently lose consolidated memories, you lose access pathways to them.

ACT-R is one of the most validated cognitive architectures in psychology, used in hundreds of published studies modeling human memory retrieval. MuninnDB uses it because it captures something true about how memory works: relevance is not permanent, it is earned through use. For a full treatment of the retrieval pipeline, see [Retrieval Design](retrieval-design.md).

---

## Donald Hebb and associative learning (1949)

In 1949, Canadian neuropsychologist Donald Hebb published *The Organization of Behavior*, where he proposed what became the most cited principle in neuroscience:

**Neurons that fire together, wire together.**

The idea is precise: when two neurons activate at the same time, the synaptic connection between them strengthens. Do it enough times, and the connection becomes so strong that activating one neuron automatically activates the other. This is the mechanism behind habits, expertise, and associative memory. It's why the smell of something can pull a memory out of nowhere — sensory neurons and memory neurons fired together enough times that the smell became a retrieval cue.

This is also how expertise consolidates. A chess grandmaster doesn't analyze every move from first principles; patterns activate related patterns automatically. Years of co-activation built a network where activating one node floods the relevant neighborhood instantly.

MuninnDB implements Hebb's Rule in the association graph. Every engram carries a set of association weights — edges to other engrams. When two engrams are retrieved in the same activation event, their association weight increases. The update is multiplicative:

```
w_new = min(1.0, w_old × (1 + η)^n)
```

Where:
- **η** is the learning rate (0.01)
- **n** is co-activation count in this event

Associations strengthen with every co-activation. The `min(1.0, ...)` cap ensures weights stay bounded. The learning rate is intentionally small — associations build gradually through repeated co-activation, not from a single event.

You didn't define this relationship. You never created an edge in a schema. You never wrote a migration saying "payment service connects to idempotency keys." The connection emerged from how you used the system — exactly the way expertise emerges from practice.

This is fundamentally different from a graph database, where edges are static and hand-defined. In Neo4j, you create a relationship and it exists until you delete it, unchanged, unweighted, indifferent to whether you ever traversed it again. Hebb's Rule produces dynamic relationships that strengthen when they're useful and fade when they're not.

---

## Bayesian confidence: not all memory is equally trustworthy

Here's a problem that no database has ever seriously addressed: you can be wrong.

You remember something confidently. Then you encounter contradicting information. How confident should you be now? If someone tells you the opposite thing three times, but you've heard the original ten times, what's the right confidence level?

The answer is Bayesian updating. MuninnDB maintains a confidence score between 0 and 1 for every engram. That score is not a label you set manually — it updates automatically when new information arrives.

The update formula:

```
posterior = (p × s) / (p × s + (1 - p) × (1 - s))
```

Where:
- **p** is the current confidence (prior)
- **s** is the signal strength of the new evidence

When contradicting information is detected, s is low — confidence updates downward. When reinforcing information arrives, s is high — confidence updates upward. With Laplace smoothing applied, the system is stable at the extremes: confidence doesn't collapse to zero or lock at one.

The practical implication: if you tell MuninnDB something and later tell it the opposite, MuninnDB doesn't blindly trust the latest message. It tracks the tension. It flags the contradiction explicitly. It updates confidence based on accumulated evidence, not recency alone.

This is how expert knowledge management actually works. A good analyst doesn't replace their model every time they see new data — they update it. MuninnDB is doing the same thing with every engram.

Confidence is also visible as a query parameter. You can ask MuninnDB to return only engrams above a confidence threshold. You can filter out low-confidence memories when you need reliable information. You can surface low-confidence memories specifically when you're doing a review or audit. The confidence score is a first-class property of every memory, not an afterthought.

---

## The spacing effect: access patterns change stability

Ebbinghaus discovered something beyond the forgetting curve: the spacing effect.

Two people memorize the same list. One reviews it 50 times in a single afternoon. The other reviews it 50 times over six months, with increasing gaps between sessions. Six months later, the second person remembers far more. Same number of repetitions. Completely different outcome.

Spaced repetition builds stability in a way that massed repetition does not. Each time you return to a memory after a gap, the retrieval itself — pulling the memory back into consciousness — strengthens it. The effort of retrieval is the consolidation mechanism. Cramming doesn't produce stable memories because you're not retrieving; you're just re-reading.

MuninnDB tracks this. Stability increases with access count, but the pattern of access matters. An engram activated 50 times over six months has higher stability than one activated 50 times in a single day. The database weights the history, not just the count.

The result: memories that are used consistently, sustainably, over time become increasingly resistant to fading. They become long-term fixtures. Memories that were activated in a burst — during an intensive project, say — lose relevance faster once the project ends. This is correct behavior. The system reflects the real epistemic weight of the memory.

---

## Why this matters for AI agents

AI agents today have a memory problem.

The most common approaches are:

**Context windows.** The agent holds recent messages in its prompt. This works for a single session. It resets when the session ends. There is no persistence, no temporal priority, no learning, no connection between sessions. The agent wakes up with amnesia every time.

**Vector databases.** Embed everything, retrieve by similarity. This is the current state of the art for AI memory, and it's still fundamentally wrong. Similarity is not relevance. "Find things similar to my current query" is not the same as "tell me what I should be thinking about right now, given what I've learned, how recently I learned it, and what connects to what." A vector database returns the 10 most similar chunks. It has no concept of whether those chunks were important last month and forgotten for good reason. It has no concept of confidence in their accuracy. It never initiates contact.

We've measured this directly. In a controlled eval, two semantically similar notes competed for the same query: one accessed recently, one dormant for years. A pure vector search ranks both nearly equally — the embeddings are similar, so the scores are similar. MuninnDB's ACT-R temporal scoring produces a **37x weighting advantage** for the recently-accessed note. In practice: the fresh note ranks above the stale one **80-100% of the time**, by an average of **+11 rank positions**. Same query. Same semantic content. Completely different retrieval outcome. The difference is that MuninnDB models the cognitive weight of memory — not just its semantic content.

**Key-value stores with retrieval logic.** Hand-written systems where someone decided which keys to write, how to look them up, and when to expire them. These work until the domain gets complex, at which point they require constant maintenance and still miss connections that weren't explicitly programmed.

None of these are memory. They're storage with retrieval. The distinction matters.

MuninnDB gives AI agents the same memory model that evolution spent hundreds of millions of years perfecting in biological brains. Relevant things surface. Irrelevant things fade. Connections form automatically. Contradictions get flagged. Sequential patterns are learned — if you always look up the dashboard after logging in, MuninnDB learns that transition and pre-surfaces the dashboard memory before you ask for it. And when something becomes urgent — because related concepts were activated, because time shifted the relevance landscape, because a contradiction appeared — the database tells the agent before the agent asks.

That last part is new. Every database before MuninnDB was passive. You query it and it responds. MuninnDB has a native push mechanism: subscribe to a context, and the database will deliver relevant engrams to you when relevance changes. Not when you ask. When it matters.

This is the architecture that Anderson (ACT-R), Hebb, and Bayes were pointing toward. They described how biological memory actually works. MuninnDB is the implementation.

---

**See also:** [Retrieval Design](retrieval-design.md) · [Cognitive Primitives](cognitive-primitives.md)
