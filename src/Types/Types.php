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
        public readonly int $hopDist = 0,
        public readonly ?string $summary = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'] ?? '',
            concept: $data['concept'] ?? '',
            hopDist: (int) ($data['hop_dist'] ?? 0),
            summary: $data['summary'] ?? null,
        );
    }
}

class TraversalEdge
{
    public function __construct(
        public readonly string $fromId,
        public readonly string $toId,
        public readonly string $relType,
        public readonly float $weight,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            fromId: $data['from_id'] ?? '',
            toId: $data['to_id'] ?? '',
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
        public readonly float $fullTextRelevance = 0.0,
        public readonly float $semanticSimilarity = 0.0,
        public readonly float $decayFactor = 0.0,
        public readonly float $hebbianBoost = 0.0,
        public readonly float $accessFrequency = 0.0,
        public readonly float $confidence = 0.0,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            fullTextRelevance: (float) ($data['full_text_relevance'] ?? 0.0),
            semanticSimilarity: (float) ($data['semantic_similarity'] ?? 0.0),
            decayFactor: (float) ($data['decay_factor'] ?? 0.0),
            hebbianBoost: (float) ($data['hebbian_boost'] ?? 0.0),
            accessFrequency: (float) ($data['access_frequency'] ?? 0.0),
            confidence: (float) ($data['confidence'] ?? 0.0),
        );
    }
}

class ExplainResponse
{
    /** @param string[] $ftsMatches @param string[] $assocPath */
    public function __construct(
        public readonly string $engramId,
        public readonly string $concept,
        public readonly float $finalScore,
        public readonly ExplainComponents $components,
        public readonly array $ftsMatches = [],
        public readonly array $assocPath = [],
        public readonly bool $wouldReturn = false,
        public readonly float $threshold = 0.0,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            engramId: $data['engram_id'] ?? '',
            concept: $data['concept'] ?? '',
            finalScore: (float) ($data['final_score'] ?? 0.0),
            components: ExplainComponents::fromArray($data['components'] ?? []),
            ftsMatches: $data['fts_matches'] ?? [],
            assocPath: $data['assoc_path'] ?? [],
            wouldReturn: (bool) ($data['would_return'] ?? false),
            threshold: (float) ($data['threshold'] ?? 0.0),
        );
    }
}

class SetStateResponse
{
    public function __construct(
        public readonly string $id,
        public readonly string $state,
        public readonly bool $updated = false,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'] ?? '',
            state: $data['state'] ?? '',
            updated: (bool) ($data['updated'] ?? false),
        );
    }
}

class DeletedEngram
{
    /** @param string[] $tags */
    public function __construct(
        public readonly string $id,
        public readonly string $concept,
        public readonly int $deletedAt = 0,
        public readonly int $recoverableUntil = 0,
        public readonly array $tags = [],
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'] ?? '',
            concept: $data['concept'] ?? '',
            deletedAt: (int) ($data['deleted_at'] ?? 0),
            recoverableUntil: (int) ($data['recoverable_until'] ?? 0),
            tags: $data['tags'] ?? [],
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
    /** @param string[] $pluginsQueued @param string[] $alreadyComplete */
    public function __construct(
        public readonly string $engramId,
        public readonly array $pluginsQueued,
        public readonly array $alreadyComplete,
        public readonly ?string $note = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            engramId: $data['engram_id'] ?? '',
            pluginsQueued: $data['plugins_queued'] ?? [],
            alreadyComplete: $data['already_complete'] ?? [],
            note: $data['note'] ?? null,
        );
    }
}

class ContradictionItem
{
    public function __construct(
        public readonly string $idA,
        public readonly string $conceptA,
        public readonly string $idB,
        public readonly string $conceptB,
        public readonly int $detectedAt = 0,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            idA: $data['id_a'] ?? '',
            conceptA: $data['concept_a'] ?? '',
            idB: $data['id_b'] ?? '',
            conceptB: $data['concept_b'] ?? '',
            detectedAt: (int) ($data['detected_at'] ?? 0),
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
        public readonly float $score = 0.0,
        public readonly float $orphanRatio = 0.0,
        public readonly float $contradictionDensity = 0.0,
        public readonly float $duplicationPressure = 0.0,
        public readonly float $temporalVariance = 0.0,
        public readonly int $totalEngrams = 0,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            score: (float) ($data['score'] ?? 0.0),
            orphanRatio: (float) ($data['orphan_ratio'] ?? 0.0),
            contradictionDensity: (float) ($data['contradiction_density'] ?? 0.0),
            duplicationPressure: (float) ($data['duplication_pressure'] ?? 0.0),
            temporalVariance: (float) ($data['temporal_variance'] ?? 0.0),
            totalEngrams: (int) ($data['total_engrams'] ?? 0),
        );
    }
}

class StatsResponse
{
    /** @param array<string, CoherenceResult> $coherence */
    public function __construct(
        public readonly int $engramCount,
        public readonly int $vaultCount,
        public readonly int $storageBytes,
        public readonly array $coherence = [],
    ) {}

    public static function fromArray(array $data): self
    {
        $coherence = [];
        foreach ($data['coherence'] ?? [] as $vault => $c) {
            $coherence[$vault] = CoherenceResult::fromArray($c);
        }

        return new self(
            engramCount: (int) ($data['engram_count'] ?? 0),
            vaultCount: (int) ($data['vault_count'] ?? 0),
            storageBytes: (int) ($data['storage_bytes'] ?? 0),
            coherence: $coherence,
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
        public readonly string $targetId,
        public readonly int $relType,
        public readonly float $weight,
        public readonly int $coActivationCount = 0,
        public readonly ?int $restoredAt = null,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            targetId: $data['target_id'] ?? '',
            relType: (int) ($data['rel_type'] ?? 0),
            weight: (float) ($data['weight'] ?? 1.0),
            coActivationCount: (int) ($data['co_activation_count'] ?? 0),
            restoredAt: isset($data['restored_at']) ? (int) $data['restored_at'] : null,
        );
    }
}

class SessionEntry
{
    public function __construct(
        public readonly string $id,
        public readonly string $concept,
        public readonly string $content = '',
        public readonly int $createdAt = 0,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'] ?? '',
            concept: $data['concept'] ?? '',
            content: $data['content'] ?? '',
            createdAt: (int) ($data['created_at'] ?? 0),
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
        public readonly ?int $uptimeSeconds = null,
        public readonly bool $dbWritable = true,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            status: $data['status'] ?? 'unknown',
            version: $data['version'] ?? null,
            uptimeSeconds: isset($data['uptime_seconds']) ? (int) $data['uptime_seconds'] : null,
            dbWritable: (bool) ($data['db_writable'] ?? true),
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
