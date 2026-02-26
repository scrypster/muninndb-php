<?php

declare(strict_types=1);

namespace MuninnDB\Types;

class WriteResponse
{
    public function __construct(
        public readonly string $id,
        public readonly int $createdAt,
        public readonly ?string $hint = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'],
            createdAt: $data['created_at'] ?? 0,
            hint: $data['hint'] ?? null,
        );
    }
}

class BatchWriteResult
{
    public function __construct(
        public readonly int $index,
        public readonly ?string $id,
        public readonly string $status,
        public readonly ?string $error = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            index: $data['index'],
            id: $data['id'] ?? null,
            status: $data['status'] ?? 'unknown',
            error: $data['error'] ?? null,
        );
    }
}

class BatchWriteResponse
{
    /** @param BatchWriteResult[] $results */
    public function __construct(
        public readonly array $results,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            results: array_map(
                fn(array $r) => BatchWriteResult::fromArray($r),
                $data['results'] ?? [],
            ),
        );
    }
}

class Engram
{
    /** @param string[] $tags */
    public function __construct(
        public readonly string $id,
        public readonly string $vault,
        public readonly string $concept,
        public readonly string $content,
        public readonly array $tags,
        public readonly float $confidence,
        public readonly float $stability,
        public readonly string $memoryType,
        public readonly string $typeLabel,
        public readonly ?string $summary,
        public readonly ?array $entities,
        public readonly ?array $relationships,
        public readonly ?string $state,
        public readonly ?int $createdAt,
        public readonly ?int $updatedAt,
        public readonly ?bool $deleted,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'] ?? '',
            vault: $data['vault'] ?? 'default',
            concept: $data['concept'] ?? '',
            content: $data['content'] ?? '',
            tags: $data['tags'] ?? [],
            confidence: (float) ($data['confidence'] ?? 0.5),
            stability: (float) ($data['stability'] ?? 0.5),
            memoryType: $data['memory_type'] ?? '',
            typeLabel: $data['type_label'] ?? '',
            summary: $data['summary'] ?? null,
            entities: $data['entities'] ?? null,
            relationships: $data['relationships'] ?? null,
            state: $data['state'] ?? null,
            createdAt: $data['created_at'] ?? null,
            updatedAt: $data['updated_at'] ?? null,
            deleted: $data['deleted'] ?? null,
        );
    }
}

class ActivationItem
{
    public function __construct(
        public readonly string $id,
        public readonly string $concept,
        public readonly string $content,
        public readonly float $score,
        public readonly ?string $summary = null,
        public readonly ?array $tags = null,
        public readonly ?string $memoryType = null,
        public readonly ?string $typeLabel = null,
        public readonly ?string $why = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'] ?? '',
            concept: $data['concept'] ?? '',
            content: $data['content'] ?? '',
            score: (float) ($data['score'] ?? 0.0),
            summary: $data['summary'] ?? null,
            tags: $data['tags'] ?? null,
            memoryType: $data['memory_type'] ?? null,
            typeLabel: $data['type_label'] ?? null,
            why: $data['why'] ?? null,
        );
    }
}

class BriefSentence
{
    public function __construct(
        public readonly string $text,
        public readonly ?string $engramId = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            text: $data['text'] ?? '',
            engramId: $data['engram_id'] ?? null,
        );
    }
}

class ActivateResponse
{
    /**
     * @param ActivationItem[] $activations
     * @param BriefSentence[] $brief
     */
    public function __construct(
        public readonly string $queryId,
        public readonly int $totalFound,
        public readonly array $activations,
        public readonly float $latencyMs,
        public readonly array $brief = [],
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            queryId: $data['query_id'] ?? '',
            totalFound: (int) ($data['total_found'] ?? 0),
            activations: array_map(
                fn(array $a) => ActivationItem::fromArray($a),
                $data['activations'] ?? [],
            ),
            latencyMs: (float) ($data['latency_ms'] ?? 0.0),
            brief: array_map(
                fn(array $b) => BriefSentence::fromArray($b),
                $data['brief'] ?? [],
            ),
        );
    }
}

class EvolveResponse
{
    public function __construct(
        public readonly string $id,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(id: $data['id'] ?? '');
    }
}

class ConsolidateResponse
{
    /**
     * @param string[] $archived
     * @param string[] $warnings
     */
    public function __construct(
        public readonly string $id,
        public readonly array $archived,
        public readonly array $warnings = [],
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'] ?? '',
            archived: $data['archived'] ?? [],
            warnings: $data['warnings'] ?? [],
        );
    }
}

class DecideResponse
{
    public function __construct(
        public readonly string $id,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(id: $data['id'] ?? '');
    }
}

class RestoreResponse
{
    public function __construct(
        public readonly string $id,
        public readonly string $concept,
        public readonly bool $restored,
        public readonly string $state,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'] ?? '',
            concept: $data['concept'] ?? '',
            restored: (bool) ($data['restored'] ?? false),
            state: $data['state'] ?? '',
        );
    }
}

class TraversalNode
{
    public function __construct(
        public readonly string $id,
        public readonly string $concept,
        public readonly ?string $content = null,
        public readonly ?int $depth = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'] ?? '',
            concept: $data['concept'] ?? '',
            content: $data['content'] ?? null,
            depth: $data['depth'] ?? null,
        );
    }
}

class TraversalEdge
{
    public function __construct(
        public readonly string $source,
        public readonly string $target,
        public readonly string $relType,
        public readonly float $weight,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            source: $data['source'] ?? '',
            target: $data['target'] ?? '',
            relType: $data['rel_type'] ?? '',
            weight: (float) ($data['weight'] ?? 1.0),
        );
    }
}

class TraverseResponse
{
    /**
     * @param TraversalNode[] $nodes
     * @param TraversalEdge[] $edges
     */
    public function __construct(
        public readonly array $nodes,
        public readonly array $edges,
        public readonly int $totalReachable,
        public readonly float $queryMs,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            nodes: array_map(
                fn(array $n) => TraversalNode::fromArray($n),
                $data['nodes'] ?? [],
            ),
            edges: array_map(
                fn(array $e) => TraversalEdge::fromArray($e),
                $data['edges'] ?? [],
            ),
            totalReachable: (int) ($data['total_reachable'] ?? 0),
            queryMs: (float) ($data['query_ms'] ?? 0.0),
        );
    }
}

class ExplainComponents
{
    public function __construct(
        public readonly ?float $semantic = null,
        public readonly ?float $recency = null,
        public readonly ?float $confidence = null,
        public readonly ?float $stability = null,
        public readonly ?float $tagBoost = null,
        public readonly ?float $entityBoost = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            semantic: isset($data['semantic']) ? (float) $data['semantic'] : null,
            recency: isset($data['recency']) ? (float) $data['recency'] : null,
            confidence: isset($data['confidence']) ? (float) $data['confidence'] : null,
            stability: isset($data['stability']) ? (float) $data['stability'] : null,
            tagBoost: isset($data['tag_boost']) ? (float) $data['tag_boost'] : null,
            entityBoost: isset($data['entity_boost']) ? (float) $data['entity_boost'] : null,
        );
    }
}

class ExplainResponse
{
    public function __construct(
        public readonly string $engramId,
        public readonly float $finalScore,
        public readonly ExplainComponents $components,
        public readonly ?string $profile = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            engramId: $data['engram_id'] ?? '',
            finalScore: (float) ($data['final_score'] ?? 0.0),
            components: ExplainComponents::fromArray($data['components'] ?? []),
            profile: $data['profile'] ?? null,
        );
    }
}

class SetStateResponse
{
    public function __construct(
        public readonly string $id,
        public readonly string $state,
        public readonly ?string $previousState = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'] ?? '',
            state: $data['state'] ?? '',
            previousState: $data['previous_state'] ?? null,
        );
    }
}

class DeletedEngram
{
    public function __construct(
        public readonly string $id,
        public readonly string $concept,
        public readonly ?int $deletedAt = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'] ?? '',
            concept: $data['concept'] ?? '',
            deletedAt: $data['deleted_at'] ?? null,
        );
    }
}

class ListDeletedResponse
{
    /** @param DeletedEngram[] $engrams */
    public function __construct(
        public readonly array $engrams,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            engrams: array_map(
                fn(array $e) => DeletedEngram::fromArray($e),
                $data['engrams'] ?? $data['deleted'] ?? [],
            ),
        );
    }
}

class RetryEnrichResponse
{
    public function __construct(
        public readonly string $id,
        public readonly string $status,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'] ?? '',
            status: $data['status'] ?? '',
        );
    }
}

class ContradictionItem
{
    public function __construct(
        public readonly string $engramA,
        public readonly string $engramB,
        public readonly string $description,
        public readonly ?float $severity = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            engramA: $data['engram_a'] ?? '',
            engramB: $data['engram_b'] ?? '',
            description: $data['description'] ?? '',
            severity: isset($data['severity']) ? (float) $data['severity'] : null,
        );
    }
}

class ContradictionsResponse
{
    /** @param ContradictionItem[] $contradictions */
    public function __construct(
        public readonly array $contradictions,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            contradictions: array_map(
                fn(array $c) => ContradictionItem::fromArray($c),
                $data['contradictions'] ?? [],
            ),
        );
    }
}

class CoherenceResult
{
    public function __construct(
        public readonly ?float $score = null,
        public readonly ?int $contradictions = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            score: isset($data['score']) ? (float) $data['score'] : null,
            contradictions: $data['contradictions'] ?? null,
        );
    }
}

class StatsResponse
{
    public function __construct(
        public readonly int $totalEngrams,
        public readonly int $totalLinks,
        public readonly ?int $totalVaults = null,
        public readonly ?int $activeEngrams = null,
        public readonly ?int $deletedEngrams = null,
        public readonly ?CoherenceResult $coherence = null,
        public readonly ?array $raw = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            totalEngrams: (int) ($data['total_engrams'] ?? 0),
            totalLinks: (int) ($data['total_links'] ?? 0),
            totalVaults: $data['total_vaults'] ?? null,
            activeEngrams: $data['active_engrams'] ?? null,
            deletedEngrams: $data['deleted_engrams'] ?? null,
            coherence: isset($data['coherence'])
                ? CoherenceResult::fromArray($data['coherence'])
                : null,
            raw: $data,
        );
    }
}

class EngramItem
{
    public function __construct(
        public readonly string $id,
        public readonly string $concept,
        public readonly ?string $content = null,
        public readonly ?string $summary = null,
        public readonly ?string $memoryType = null,
        public readonly ?string $state = null,
        public readonly ?int $createdAt = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'] ?? '',
            concept: $data['concept'] ?? '',
            content: $data['content'] ?? null,
            summary: $data['summary'] ?? null,
            memoryType: $data['memory_type'] ?? null,
            state: $data['state'] ?? null,
            createdAt: $data['created_at'] ?? null,
        );
    }
}

class ListEngramsResponse
{
    /** @param EngramItem[] $engrams */
    public function __construct(
        public readonly array $engrams,
        public readonly int $total,
        public readonly int $limit,
        public readonly int $offset,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            engrams: array_map(
                fn(array $e) => EngramItem::fromArray($e),
                $data['engrams'] ?? [],
            ),
            total: (int) ($data['total'] ?? 0),
            limit: (int) ($data['limit'] ?? 20),
            offset: (int) ($data['offset'] ?? 0),
        );
    }
}

class AssociationItem
{
    public function __construct(
        public readonly string $id,
        public readonly string $sourceId,
        public readonly string $targetId,
        public readonly string $relType,
        public readonly float $weight,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'] ?? '',
            sourceId: $data['source_id'] ?? '',
            targetId: $data['target_id'] ?? '',
            relType: $data['rel_type'] ?? '',
            weight: (float) ($data['weight'] ?? 1.0),
        );
    }
}

class SessionEntry
{
    public function __construct(
        public readonly string $id,
        public readonly string $concept,
        public readonly string $action,
        public readonly ?int $timestamp = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'] ?? '',
            concept: $data['concept'] ?? '',
            action: $data['action'] ?? '',
            timestamp: $data['timestamp'] ?? null,
        );
    }
}

class SessionResponse
{
    /** @param SessionEntry[] $entries */
    public function __construct(
        public readonly array $entries,
        public readonly int $total,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            entries: array_map(
                fn(array $e) => SessionEntry::fromArray($e),
                $data['entries'] ?? [],
            ),
            total: (int) ($data['total'] ?? 0),
        );
    }
}

class HealthResponse
{
    public function __construct(
        public readonly string $status,
        public readonly ?string $version = null,
        public readonly ?float $uptime = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            status: $data['status'] ?? 'unknown',
            version: $data['version'] ?? null,
            uptime: isset($data['uptime']) ? (float) $data['uptime'] : null,
        );
    }
}

class SseEvent
{
    public function __construct(
        public readonly string $event,
        public readonly ?string $engramId = null,
        public readonly ?string $vault = null,
        public readonly ?array $data = null,
    ) {}

    public static function fromArray(array $data, string $event = 'message'): self
    {
        return new self(
            event: $event,
            engramId: $data['engram_id'] ?? $data['id'] ?? null,
            vault: $data['vault'] ?? null,
            data: $data,
        );
    }
}
