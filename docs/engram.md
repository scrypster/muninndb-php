# The Engram

An engram in neuroscience is the physical trace a memory leaves in the brain. Not a pointer to a memory. Not a label for a memory. The strengthened pattern of synaptic connections that *is* the memory — encoded in structure, in relationship, in weight.

MuninnDB borrows this term deliberately.

---

## 1. What Is an Engram?

When you learn something and it sticks, your brain has done something physical: specific neurons have formed stronger connections with each other. The next time you encounter related context, those connections activate. The more you revisit something, the more stable those connections become. This is what memory *is* at the biological level — a pattern of weighted associations.

An engram in MuninnDB is modeled on exactly this. It is not a record that describes a piece of information. It *is* the memory trace — carrying not just the content, but the entire cognitive state around that content:

- How confident we are in it
- How relevant it is right now
- How strongly it connects to other things
- How stable it is over time
- How recently and how often it has been activated

These aren't application-level annotations. They are part of the record format itself, indexed natively, computed continuously.

---

## 2. Why Not a Row?

A relational row is: schema, fields, values. It has no concept of importance. No concept of time-weighted relevance. No native associations to other rows. No temporal priority.

Two rows in the same table are equally "present." Postgres does not know that the row you wrote yesterday matters more than the one from three years ago. It does not know that one record reinforces another or contradicts it. It does not know which records tend to be retrieved together.

You can build all of that yourself — as application logic, on top, with extra tables, extra queries, scheduled jobs, and significant complexity. But it is not in the storage layer. It is not part of the record. It is not indexed.

An engram carries relevance, stability, confidence, access count, last access time, and a graph of weighted associations as first-class storage primitives. They are part of the record format. They are indexed. They are computed continuously by background workers that never block your reads or writes.

---

## 3. Why Not a Document?

A document store like MongoDB is flexible and semantically empty. It will store whatever you put in it. It does not know what any of it means. It does not track how related one document is to another unless you define that relationship explicitly — and even then, it is just a foreign key. There is no native temporal priority. There is no confidence model.

An engram has a typed association model with 15 built-in relationship types: `supports`, `contradicts`, `depends_on`, `supersedes`, `relates_to`, `is_part_of`, `causes`, `preceded_by`, `followed_by`, `created_by_person`, `belongs_to_project`, `references`, `implements`, `blocks`, `resolves` — plus a user-defined range (`0x8000+`) for domain-specific relationships. These associations carry weights and confidence scores that evolve automatically based on use patterns.

When two engrams are activated together repeatedly, their association strengthens. When they go dormant, it weakens. A document store cannot do this because it has no concept of activation.

---

## 4. Why Not a Node?

Graph databases come closest to the engram model. A node can have properties. Edges can have weights. The association model is native.

But graph edges are static. You define them, they stay the same until you change them. They do not strengthen when you traverse them repeatedly. They do not weaken when they go dormant. Nodes do not lose relevance over time. The graph does not push to you when something changes.

An engram's associations are Hebbian. They strengthen when engrams co-activate. They weaken when they go dormant. The graph is not a snapshot of relationships you defined — it is a living record of which ideas tend to surface together, and how strongly.

---

## 5. The Engram Data Model

```go
type Engram struct {
    ID           ULID
    Concept      string
    Content      string
    Confidence   float32
    Relevance    float32
    Stability    float32
    AccessCount  uint32
    LastAccess   time.Time
    CreatedAt    time.Time
    State        LifecycleState
    Tags         []string
    Associations []Association
    CreatedBy    string
}

type Association struct {
    TargetID      ULID
    RelType       uint16
    Weight        float32
    Confidence    float32
    LastActivated time.Time
}
```

**ID (ULID)**
Universally unique, lexicographically sortable by creation time. 16 bytes. Sortable IDs are not a cosmetic preference — they make time-range scans over the KV store efficient. A UUID is random; a ULID encodes timestamp in its high bits. Range queries over time do not scatter across the keyspace.

**Concept**
What this engram is about. Maximum 512 bytes. Think of it as the headline. The concept field carries the highest field weight in full-text search (3.0x) because it is the most intentional description of the content. When you search, matching the concept is stronger evidence of relevance than matching buried text in the content body.

**Content**
The actual information. Maximum 16KB. Automatically compressed with zstd when it exceeds 512 bytes. The content field carries a 1.0x FTS weight — it is searched, but a match here is a weaker signal than a match in concept or tags.

**Confidence (float32, 0.0–1.0)**
A Bayesian score representing how reliable this information is believed to be. Updated automatically when contradictions or reinforcements are detected. Every activation result is multiplied by this score — a 0.3-confidence engram scores 30% of what it would at full confidence. Low-confidence memories rank lower automatically, without being excluded.

**Relevance (float32, 0.0–1.0)**
A stored relevance estimate used for candidate retrieval and score reporting. The primary temporal scoring in production is ACT-R, which is computed at query time from AccessCount and LastAccess — not from this stored value. Relevance is not permanent. It must be earned through use.

**Stability (float32, in days)**
How resistant this engram is to temporal scoring loss. Used in the reported decay factor component. Stability increases with access frequency and — critically — access *pattern*. An engram accessed 50 times over six months is more stable than one accessed 50 times in a single day.

**AccessCount (uint32)**
How many times this engram has been returned in an activation. Used in scoring, in stability calculation, and as a direct signal for access-frequency weighting in the composite activation score.

**State (LifecycleState)**
Eight states: `PLANNING`, `ACTIVE`, `PAUSED`, `BLOCKED`, `COMPLETED`, `CANCELLED`, `ARCHIVED`, `SOFT_DELETED`. New writes default to `ACTIVE`. Archived and soft-deleted engrams still exist and still contribute to associations and temporal scoring — they just do not appear in default activation results. Hard deletion is not the default because you cannot know what will matter later. The floor on relevance (default 0.05) ensures that no engram ever becomes completely invisible.

**Tags ([]string)**
Free-form labels. Indexed with 2.0x FTS weight. Tags are curated signals — a human deliberately applying a tag is a stronger signal than the same word appearing in the content body.

**Associations ([]Association)**
Weighted, typed edges to other engrams. Up to 256 per engram. Each association carries:
- `TargetID`: the ULID of the connected engram
- `RelType`: one of 15 built-in types, or a user-defined type in the extended range (`0x8000+`)
- `Weight`: Hebbian strength (0.0–1.0), updated by the Hebbian worker
- `Confidence`: how confident we are in this relationship
- `LastActivated`: when this edge was last traversed

---

## 6. ERF: The Engram Record Format

ERF is MuninnDB's binary storage format. The question worth asking is: why build a custom binary format at all? JSON works. Protobuf works. MessagePack works.

The answer is in the access pattern.

The cognitive workers — temporal, Hebbian, contradiction, confidence — need to read and update cognitive scores (relevance, confidence, stability, access count, timestamps) continuously. These updates are small and frequent. If they required deserializing the entire record, including the content field which can be up to 16KB and compressed, the overhead would be substantial and constant.

ERF is designed so that the metadata section — all of the cognitive scores and counters — lives at a fixed, predictable byte offset. The cognitive workers can seek directly to it, read it, update it, and write it back, without ever touching the variable-length content. This is the same principle as CPU cache lines: locality and predictability matter enormously at scale.

```
┌─────────────────────────────────────────┐
│ Header (8 bytes)                        │
│ Magic=0x4D554E4E ("MUNN"), Version=0x01 │
│ Flags, Reserved                         │
├─────────────────────────────────────────┤
│ Metadata (100 bytes, fixed)             │
│ ID(16) | Timestamps(24) | Scores(12)    │
│ Counts(5) | State | Reserved(32)        │
├─────────────────────────────────────────┤
│ Offset Table (40 bytes)                 │
│ 6 offset pairs for variable fields      │
├─────────────────────────────────────────┤
│ Variable Data                           │
│ concept | created_by | content          │
│ (zstd compressed if >512B) | tags       │
│ associations | embedding                │
├─────────────────────────────────────────┤
│ Trailer (4 bytes)                       │
│ CRC-32 (Castagnoli) checksum            │
└─────────────────────────────────────────┘
```

**Header (8 bytes)**
Every ERF record starts with the magic bytes `0x4D554E4E` — the ASCII encoding of `MUNN`. If a record doesn't start with this magic, it is corrupt or not an engram. Version byte `0x01` follows. This makes format migration possible without ambiguity.

**Metadata (100 bytes, fixed)**
The ID (16 bytes), all timestamps (24 bytes), all cognitive scores — confidence, relevance, stability — as `float32` values (12 bytes), access and association counts (5 bytes), lifecycle state (1 byte), and 32 bytes reserved for future use. This entire block is at a known, stable offset. A cognitive worker updating relevance after a temporal recomputation touches exactly these bytes and nothing else.

**Offset Table (40 bytes)**
Six offset pairs pointing into the variable-length data section. Concept, created-by, content, tags, associations, and embedding each have an offset and a length. Any field can be read independently without scanning the entire record.

**Variable Data**
The actual content. Concept and created-by strings. Content — compressed with zstd if larger than 512 bytes, which covers most non-trivial payloads. Tags serialized as a length-prefixed string list. Association sub-records, each exactly 40 bytes (TargetID + RelType + Weight + Confidence + LastActivated), allowing up to 256 per engram. Optional vector embedding appended last.

**Trailer (4 bytes)**
CRC-32 (Castagnoli) checksum over the entire record. Computed on write, verified on read. Corruption is detected at load time, not discovered downstream when a query returns nonsense.

**Typical sizes:**
- Simple engram without embedding: ~400 bytes
- Engram with a full 16KB content field and embedding: ~1.7KB (zstd is aggressive on text)
- Engram with 256 associations and embedding: ~11KB

The format is designed for the actual workload. Cognitive scores are small, frequently accessed, and fixed in position. Content is large, infrequently accessed by the workers, and compressed. The format matches the access pattern.

---

**See also:** [Cognitive Primitives](cognitive-primitives.md) · [Retrieval Design](retrieval-design.md) · [Feature Reference](feature-reference.md) · [Hierarchical Memory](hierarchical-memory.md)
