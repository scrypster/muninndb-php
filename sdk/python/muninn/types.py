"""MuninnDB type definitions."""

from dataclasses import dataclass, field
from typing import Any


@dataclass
class WriteRequest:
    """Request to write an engram."""

    vault: str
    concept: str
    content: str
    tags: list[str] | None = None
    confidence: float = 0.9
    stability: float = 0.5
    embedding: list[float] | None = None
    associations: dict[str, Any] | None = None
    memory_type: int | None = None
    type_label: str | None = None
    summary: str | None = None
    entities: list[dict] | None = None  # each: {"name": str, "type": str}
    relationships: list[dict] | None = None  # each: {"target_id": str, "relation": str, "weight": float}


@dataclass
class BatchWriteResult:
    """Result for a single engram in a batch write."""

    index: int
    id: str
    status: str
    error: str | None = None


@dataclass
class BatchWriteResponse:
    """Response from a batch write operation."""

    results: list[BatchWriteResult]


@dataclass
class WriteResponse:
    """Response from writing an engram."""

    id: str
    created_at: int
    hint: str | None = None


@dataclass
class ActivateRequest:
    """Request to activate memory."""

    vault: str
    context: list[str]
    max_results: int = 10
    threshold: float = 0.1
    max_hops: int = 0
    include_why: bool = False
    brief_mode: str = "auto"


@dataclass
class ActivationItem:
    """A single activated memory item."""

    id: str
    concept: str
    content: str
    score: float
    confidence: float
    why: str | None = None
    hop_path: list[str] | None = None
    dormant: bool = False
    memory_type: int = 0
    type_label: str = ""


@dataclass
class BriefSentence:
    """A sentence extracted by brief mode."""

    engram_id: str
    text: str
    score: float


@dataclass
class ActivateResponse:
    """Response from activating memory."""

    query_id: str
    total_found: int
    activations: list[ActivationItem]
    latency_ms: float = 0.0
    brief: list[BriefSentence] | None = None


@dataclass
class ReadResponse:
    """Response from reading an engram."""

    id: str
    concept: str
    content: str
    confidence: float
    relevance: float
    stability: float
    access_count: int
    tags: list[str]
    state: str
    created_at: int
    updated_at: int
    last_access: int | None = None
    coherence: "dict[str, CoherenceResult] | None" = None


@dataclass
class CoherenceResult:
    """Coherence metrics for a vault."""

    score: float
    orphan_ratio: float
    contradiction_density: float
    duplication_pressure: float
    temporal_variance: float
    total_engrams: int


@dataclass
class StatResponse:
    """Response from stats endpoint."""

    engram_count: int
    vault_count: int
    storage_bytes: int
    coherence: dict[str, CoherenceResult] | None = None


@dataclass
class Push:
    """SSE push event from subscription."""

    subscription_id: str
    trigger: str
    push_number: int
    engram_id: str | None = None
    at: int | None = None
