"""Async MuninnDB client."""

import asyncio
import json
import random

import httpx

from .errors import (
    MuninnAuthError,
    MuninnConnectionError,
    MuninnConflict,
    MuninnError,
    MuninnNotFound,
    MuninnServerError,
    MuninnTimeoutError,
)
from .sse import SSEStream
from .types import (
    ActivateResponse,
    ActivationItem,
    BatchWriteResponse,
    BatchWriteResult,
    BriefSentence,
    CoherenceResult,
    ReadResponse,
    StatResponse,
    WriteResponse,
)


class MuninnClient:
    """Async client for MuninnDB REST API.

    The client uses httpx for async HTTP and supports automatic retry with
    exponential backoff for transient failures.

    Usage:
        async with MuninnClient("http://localhost:8476") as client:
            eng_id = await client.write(
                vault="default",
                concept="memory concept",
                content="memory content"
            )
            results = await client.activate(
                vault="default",
                context=["search query"]
            )
            async for push in client.subscribe(vault="default"):
                print(f"New engram: {push.engram_id}")
                break

    Args:
        base_url: Base URL of MuninnDB server (default: http://localhost:8476)
        token: Optional Bearer token for authentication
        timeout: Request timeout in seconds (default: 5.0)
        max_retries: Maximum retry attempts for transient errors (default: 3)
        retry_backoff: Initial backoff multiplier for retries (default: 0.5)
        max_connections: Max concurrent connections (default: 20)
        keepalive_connections: Max keepalive connections (default: 10)
    """

    def __init__(
        self,
        base_url: str = "http://localhost:8476",
        token: str | None = None,
        timeout: float = 5.0,
        max_retries: int = 3,
        retry_backoff: float = 0.5,
        max_connections: int = 20,
        keepalive_connections: int = 10,
    ):
        self._base_url = base_url.rstrip("/")
        self._token = token
        self._timeout = timeout
        self._max_retries = max_retries
        self._retry_backoff = retry_backoff
        self._max_connections = max_connections
        self._keepalive_connections = keepalive_connections
        self._http: httpx.AsyncClient | None = None

    async def __aenter__(self):
        """Enter async context."""
        self._http = httpx.AsyncClient(
            base_url=self._base_url,
            timeout=self._timeout,
            limits=httpx.Limits(
                max_connections=self._max_connections,
                max_keepalive_connections=self._keepalive_connections,
            ),
            headers=self._default_headers(),
        )
        return self

    async def __aexit__(self, *args):
        """Exit async context."""
        if self._http:
            await self._http.aclose()

    async def write(
        self,
        vault: str = "default",
        concept: str = "",
        content: str = "",
        tags: list[str] | None = None,
        confidence: float = 0.9,
        stability: float = 0.5,
        memory_type: int | None = None,
        type_label: str | None = None,
        summary: str | None = None,
        entities: list[dict] | None = None,
        relationships: list[dict] | None = None,
    ) -> WriteResponse:
        """Write an engram to the database.

        Args:
            vault: Vault name (default: "default")
            concept: Concept/title for this engram
            content: Main content/body
            tags: Optional list of tags for categorization
            confidence: Confidence score 0-1 (default: 0.9)
            stability: Stability score 0-1 (default: 0.5)
            memory_type: Memory type enum (0=unknown, 1=fact, 2=decision, etc.)
            type_label: Free-form type label (e.g. "architecture_decision")
            summary: Caller-provided summary for inline enrichment
            entities: Caller-provided entities [{"name": "...", "type": "..."}]
            relationships: Caller-provided relationships [{"target_id": "...", "relation": "...", "weight": 1.0}]

        Returns:
            WriteResponse with id, created_at, and optional hint

        Raises:
            MuninnError: If write fails
        """
        body: dict = {
            "vault": vault,
            "concept": concept,
            "content": content,
            "confidence": confidence,
            "stability": stability,
        }
        if tags:
            body["tags"] = tags
        if memory_type is not None:
            body["memory_type"] = memory_type
        if type_label is not None:
            body["type_label"] = type_label
        if summary is not None:
            body["summary"] = summary
        if entities is not None:
            body["entities"] = entities
        if relationships is not None:
            body["relationships"] = relationships

        response = await self._request("POST", "/api/engrams", json=body)
        return WriteResponse(
            id=response.get("id", ""),
            created_at=response.get("created_at", 0),
            hint=response.get("hint"),
        )

    async def write_batch(
        self,
        vault: str = "default",
        engrams: list[dict] | None = None,
    ) -> BatchWriteResponse:
        """Write multiple engrams in a single batch call.

        More efficient than calling write() repeatedly. Maximum 50 per batch.
        Each engram dict can contain: concept, content, tags, confidence,
        stability, memory_type, type_label, summary, entities, relationships.

        Args:
            vault: Default vault for engrams that don't specify one
            engrams: List of engram dicts to write

        Returns:
            BatchWriteResponse with per-item results

        Raises:
            MuninnError: If batch write fails
        """
        if not engrams:
            raise MuninnError("engrams list is required and must not be empty")
        if len(engrams) > 50:
            raise MuninnError("batch size exceeds maximum of 50")

        items = []
        for eng in engrams:
            item = dict(eng)
            if "vault" not in item:
                item["vault"] = vault
            items.append(item)

        response = await self._request(
            "POST", "/api/engrams/batch", json={"engrams": items}
        )

        results = [
            BatchWriteResult(
                index=r.get("index", i),
                id=r.get("id", ""),
                status=r.get("status", "error"),
                error=r.get("error"),
            )
            for i, r in enumerate(response.get("results", []))
        ]
        return BatchWriteResponse(results=results)

    async def activate(
        self,
        vault: str = "default",
        context: list[str] | None = None,
        max_results: int = 10,
        threshold: float = 0.1,
        max_hops: int = 0,
        include_why: bool = False,
        brief_mode: str = "auto",
    ) -> ActivateResponse:
        """Activate memory using semantic search and graph traversal.

        Args:
            vault: Vault name (default: "default")
            context: List of query terms/context
            max_results: Max results to return (default: 10)
            threshold: Min activation score threshold (default: 0.1)
            max_hops: Max graph hops to traverse (default: 0)
            include_why: Include reasoning/why field (default: False)
            brief_mode: Brief extraction mode - "auto", "extractive", "abstractive" (default: "auto")

        Returns:
            ActivateResponse with activations and optional brief

        Raises:
            MuninnError: If activation fails
        """
        if context is None:
            context = []

        body = {
            "vault": vault,
            "context": context,
            "max_results": max_results,
            "threshold": threshold,
            "max_hops": max_hops,
            "include_why": include_why,
            "brief_mode": brief_mode,
        }

        response = await self._request("POST", "/api/activate", json=body)

        activations = [
            ActivationItem(
                id=item.get("id", ""),
                concept=item.get("concept", ""),
                content=item.get("content", ""),
                score=item.get("score", 0.0),
                confidence=item.get("confidence", 0.0),
                why=item.get("why"),
                hop_path=item.get("hop_path"),
                dormant=item.get("dormant", False),
                memory_type=item.get("memory_type", 0),
                type_label=item.get("type_label", ""),
            )
            for item in response.get("activations", [])
        ]

        brief = None
        if response.get("brief"):
            brief = [
                BriefSentence(
                    engram_id=sent.get("engram_id", ""),
                    text=sent.get("text", ""),
                    score=sent.get("score", 0.0),
                )
                for sent in response["brief"]
            ]

        return ActivateResponse(
            query_id=response.get("query_id", ""),
            total_found=response.get("total_found", 0),
            activations=activations,
            latency_ms=response.get("latency_ms", 0.0),
            brief=brief,
        )

    async def read(self, id: str, vault: str = "default") -> ReadResponse:
        """Read a specific engram by ID.

        Args:
            id: Engram ULID
            vault: Vault name (default: "default")

        Returns:
            ReadResponse with engram details

        Raises:
            MuninnNotFound: If engram doesn't exist
            MuninnError: If read fails
        """
        response = await self._request("GET", f"/api/engrams/{id}", params={"vault": vault})

        coherence = response.get("coherence")
        return ReadResponse(
            id=response.get("id", ""),
            concept=response.get("concept", ""),
            content=response.get("content", ""),
            confidence=response.get("confidence", 0.0),
            relevance=response.get("relevance", 0.0),
            stability=response.get("stability", 0.0),
            access_count=response.get("access_count", 0),
            tags=response.get("tags", []),
            state=response.get("state", ""),
            created_at=response.get("created_at", 0),
            updated_at=response.get("updated_at", 0),
            last_access=response.get("last_access"),
        )

    async def forget(self, id: str, vault: str = "default", hard: bool = False) -> bool:
        """Delete an engram (soft or hard delete).

        Args:
            id: Engram ULID
            vault: Vault name (default: "default")
            hard: If True, hard delete (cannot recover). If False, soft delete (default: False)

        Returns:
            True if deletion successful

        Raises:
            MuninnNotFound: If engram doesn't exist
            MuninnError: If deletion fails
        """
        if hard:
            await self._request(
                "POST",
                f"/api/engrams/{id}/forget",
                params={"vault": vault, "hard": "true"},
            )
        else:
            await self._request(
                "DELETE",
                f"/api/engrams/{id}",
                params={"vault": vault},
            )
        return True

    async def link(
        self,
        source_id: str,
        target_id: str,
        vault: str = "default",
        rel_type: int = 5,
        weight: float = 1.0,
    ) -> bool:
        """Create an association/link between two engrams.

        Args:
            source_id: Source engram ULID
            target_id: Target engram ULID
            vault: Vault name (default: "default")
            rel_type: Relationship type code (default: 5)
            weight: Link weight/strength (default: 1.0)

        Returns:
            True if link created successfully

        Raises:
            MuninnError: If link creation fails
        """
        body = {
            "vault": vault,
            "source_id": source_id,
            "target_id": target_id,
            "rel_type": rel_type,
            "weight": weight,
        }
        await self._request("POST", "/api/link", json=body)
        return True

    async def stats(self) -> StatResponse:
        """Get database statistics including coherence scores.

        Returns:
            StatResponse with engram count, vault count, storage bytes, and coherence

        Raises:
            MuninnError: If stats request fails
        """
        response = await self._request("GET", "/api/stats")

        coherence = None
        if response.get("coherence"):
            coherence = {
                vault_name: CoherenceResult(
                    score=data.get("score", 0.0),
                    orphan_ratio=data.get("orphan_ratio", 0.0),
                    contradiction_density=data.get("contradiction_density", 0.0),
                    duplication_pressure=data.get("duplication_pressure", 0.0),
                    temporal_variance=data.get("temporal_variance", 0.0),
                    total_engrams=data.get("total_engrams", 0),
                )
                for vault_name, data in response["coherence"].items()
            }

        return StatResponse(
            engram_count=response.get("engram_count", 0),
            vault_count=response.get("vault_count", 0),
            storage_bytes=response.get("storage_bytes", 0),
            coherence=coherence,
        )

    def subscribe(
        self,
        vault: str = "default",
        push_on_write: bool = True,
        threshold: float = 0.0,
    ) -> SSEStream:
        """Subscribe to vault events via Server-Sent Events (SSE).

        This returns an async iterable that yields Push events when engrams are
        written to the vault. The stream automatically reconnects on network errors.

        Usage:
            stream = client.subscribe(vault="default")
            async for push in stream:
                print(f"New engram: {push.engram_id}")
                if condition:
                    await stream.close()

        Args:
            vault: Vault to subscribe to (default: "default")
            push_on_write: Emit push events on new writes (default: True)
            threshold: Min activation threshold for push events (default: 0.0)

        Returns:
            SSEStream async iterable

        Raises:
            MuninnError: If subscription fails
        """
        params = {
            "vault": vault,
            "push_on_write": str(push_on_write).lower(),
        }
        if threshold:
            params["threshold"] = str(threshold)

        return SSEStream(self, "/api/subscribe", params)

    async def health(self) -> bool:
        """Check if MuninnDB server is healthy.

        Returns:
            True if server responds with 200 OK

        Raises:
            MuninnError: If health check fails
        """
        try:
            response = await self._request("GET", "/health")
            return response.get("status") == "ok"
        except MuninnError:
            return False

    async def _request(self, method: str, path: str, **kwargs) -> dict:
        """Make an HTTP request with automatic retry logic.

        Retries on transient errors (502, 503, 504, connection/read errors).
        Does not retry on 4xx errors. Uses exponential backoff with jitter.

        Args:
            method: HTTP method (GET, POST, DELETE, etc)
            path: URL path relative to base_url
            **kwargs: Additional arguments to pass to httpx

        Returns:
            Parsed JSON response as dict

        Raises:
            MuninnAuthError: 401 Unauthorized
            MuninnNotFound: 404 Not Found
            MuninnConflict: 409 Conflict
            MuninnServerError: 5xx errors
            MuninnTimeoutError: Request timeout
            MuninnConnectionError: Connection error
            MuninnError: Other HTTP errors
        """
        if not self._http:
            raise MuninnError("Client not initialized. Use 'async with' context manager.")

        attempt = 0
        while attempt <= self._max_retries:
            try:
                response = await self._http.request(method, path, **kwargs)
                self._raise_for_status(response)
                return response.json()

            except (httpx.ConnectError, httpx.ReadError, httpx.RemoteProtocolError) as e:
                if attempt >= self._max_retries:
                    raise MuninnConnectionError(f"Connection failed: {str(e)}")
                await self._backoff(attempt)
                attempt += 1

            except httpx.ReadTimeout as e:
                if attempt >= self._max_retries:
                    raise MuninnTimeoutError(f"Request timeout: {str(e)}")
                await self._backoff(attempt)
                attempt += 1

            except httpx.HTTPStatusError as e:
                # Don't retry on 4xx (except certain ones), do retry on 5xx
                if 500 <= e.response.status_code < 600:
                    if attempt >= self._max_retries:
                        self._raise_for_status(e.response)
                    await self._backoff(attempt)
                    attempt += 1
                else:
                    self._raise_for_status(e.response)

            except MuninnError:
                raise

        raise MuninnError("Max retries exceeded")

    async def _backoff(self, attempt: int):
        """Wait with exponential backoff + jitter.

        Args:
            attempt: Attempt number (0-indexed)
        """
        delay = self._retry_backoff * (2 ** attempt) + random.uniform(0, 0.1)
        await asyncio.sleep(delay)

    def _default_headers(self) -> dict:
        """Build default request headers."""
        headers = {"Content-Type": "application/json"}
        if self._token:
            headers["Authorization"] = f"Bearer {self._token}"
        return headers

    def _raise_for_status(self, response: httpx.Response):
        """Convert httpx response to appropriate MuninnError.

        Args:
            response: httpx Response object

        Raises:
            Appropriate MuninnError subclass
        """
        if response.status_code == 401:
            raise MuninnAuthError(
                "Authentication required. Provide token= parameter to MuninnClient.",
                401,
            )
        elif response.status_code == 404:
            raise MuninnNotFound(f"Not found: {response.text}", 404)
        elif response.status_code == 409:
            raise MuninnConflict(f"Conflict: {response.text}", 409)
        elif 500 <= response.status_code < 600:
            raise MuninnServerError(
                f"Server error {response.status_code}: {response.text}",
                response.status_code,
            )
        elif response.status_code >= 400:
            raise MuninnError(
                f"Client error {response.status_code}: {response.text}",
                response.status_code,
            )
