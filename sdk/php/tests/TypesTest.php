<?php

declare(strict_types=1);

namespace MuninnDB\Tests;

use MuninnDB\Exceptions\AuthException;
use MuninnDB\Exceptions\NotFoundException;
use MuninnDB\Exceptions\ConflictException;
use MuninnDB\Exceptions\ValidationException;
use MuninnDB\Exceptions\ServerException;
use MuninnDB\MuninnClient;
use MuninnDB\Types\ActivateResponse;
use MuninnDB\Types\AssociationItem;
use MuninnDB\Types\BatchWriteResponse;
use MuninnDB\Types\CoherenceResult;
use MuninnDB\Types\ContradictionItem;
use MuninnDB\Types\ContradictionsResponse;
use MuninnDB\Types\DeletedEngram;
use MuninnDB\Types\ExplainComponents;
use MuninnDB\Types\ExplainResponse;
use MuninnDB\Types\RetryEnrichResponse;
use MuninnDB\Types\SessionEntry;
use MuninnDB\Types\StatsResponse;
use MuninnDB\Types\TraversalEdge;
use MuninnDB\Types\TraversalNode;
use MuninnDB\Types\TraverseResponse;
use MuninnDB\Types\WriteResponse;
use PHPUnit\Framework\TestCase;

/**
 * Unit tests for MuninnDB PHP SDK type deserialization and exception mapping.
 * These tests verify that fromArray() methods read the correct snake_case
 * field names that the server actually sends on the wire.
 */
class TypesTest extends TestCase
{
    // ── WriteResponse ───────────────────────────────────────────

    public function testWriteResponseDeserializes(): void
    {
        $r = WriteResponse::fromArray(['id' => '01ARZ3', 'created_at' => 1700000000]);
        self::assertSame('01ARZ3', $r->id);
        self::assertSame(1700000000, $r->createdAt);
        self::assertNull($r->hint);
    }

    // ── BatchWriteResponse ──────────────────────────────────────

    public function testBatchWriteResultIdIsOptional(): void
    {
        $resp = BatchWriteResponse::fromArray([
            'results' => [
                ['index' => 0, 'id' => 'id-1', 'status' => 'created'],
                ['index' => 1, 'status' => 'duplicate'],
            ],
        ]);
        self::assertCount(2, $resp->results);
        self::assertSame('id-1', $resp->results[0]->id);
        self::assertNull($resp->results[1]->id);
        self::assertSame('duplicate', $resp->results[1]->status);
    }

    // ── TraversalNode — hop_dist (not depth) ────────────────────

    public function testTraversalNodeUsesHopDist(): void
    {
        $node = TraversalNode::fromArray([
            'id'       => 'n1',
            'concept'  => 'test',
            'hop_dist' => 2,
            'summary'  => 'brief',
        ]);
        self::assertSame('n1', $node->id);
        self::assertSame(2, $node->hopDist);
        self::assertSame('brief', $node->summary);
    }

    public function testTraversalNodeDefaultHopDistIsZero(): void
    {
        $node = TraversalNode::fromArray(['id' => 'n1', 'concept' => 'c']);
        self::assertSame(0, $node->hopDist);
    }

    // ── TraversalEdge — from_id / to_id (not source/target) ─────

    public function testTraversalEdgeUsesFromIdToId(): void
    {
        $edge = TraversalEdge::fromArray([
            'from_id'  => 'src',
            'to_id'    => 'dst',
            'rel_type' => '5',
            'weight'   => 0.8,
        ]);
        self::assertSame('src', $edge->fromId);
        self::assertSame('dst', $edge->toId);
        self::assertEqualsWithDelta(0.8, $edge->weight, 0.001);
    }

    // ── TraverseResponse ─────────────────────────────────────────

    public function testTraverseResponseDeserializes(): void
    {
        $resp = TraverseResponse::fromArray([
            'nodes' => [
                ['id' => 'n1', 'concept' => 'a', 'hop_dist' => 0],
                ['id' => 'n2', 'concept' => 'b', 'hop_dist' => 1],
            ],
            'edges' => [
                ['from_id' => 'n1', 'to_id' => 'n2', 'rel_type' => '1', 'weight' => 0.9],
            ],
            'total_reachable' => 10,
            'query_ms'        => 3.5,
        ]);
        self::assertCount(2, $resp->nodes);
        self::assertCount(1, $resp->edges);
        self::assertSame(10, $resp->totalReachable);
        self::assertEqualsWithDelta(3.5, $resp->queryMs, 0.001);
        self::assertSame(1, $resp->nodes[1]->hopDist);
        self::assertSame('n1', $resp->edges[0]->fromId);
    }

    // ── ExplainComponents — server field names ───────────────────

    public function testExplainComponentsFieldNames(): void
    {
        $c = ExplainComponents::fromArray([
            'full_text_relevance' => 0.9,
            'semantic_similarity' => 0.8,
            'decay_factor'        => 0.7,
            'hebbian_boost'       => 0.6,
            'access_frequency'    => 0.5,
            'confidence'          => 0.4,
        ]);
        self::assertEqualsWithDelta(0.9, $c->fullTextRelevance, 0.001);
        self::assertEqualsWithDelta(0.8, $c->semanticSimilarity, 0.001);
        self::assertEqualsWithDelta(0.7, $c->decayFactor, 0.001);
        self::assertEqualsWithDelta(0.6, $c->hebbianBoost, 0.001);
        self::assertEqualsWithDelta(0.5, $c->accessFrequency, 0.001);
        self::assertEqualsWithDelta(0.4, $c->confidence, 0.001);
    }

    public function testExplainResponseDeserializes(): void
    {
        $resp = ExplainResponse::fromArray([
            'engram_id'   => 'e1',
            'concept'     => 'test',
            'final_score' => 0.85,
            'components'  => [
                'full_text_relevance' => 0.9,
                'semantic_similarity' => 0.0,
                'decay_factor'        => 1.0,
                'hebbian_boost'       => 0.1,
                'access_frequency'    => 0.2,
                'confidence'          => 0.8,
            ],
            'fts_matches'  => ['token1'],
            'assoc_path'   => ['e2'],
            'would_return' => true,
            'threshold'    => 0.5,
        ]);
        self::assertSame('e1', $resp->engramId);
        self::assertSame('test', $resp->concept);
        self::assertEqualsWithDelta(0.85, $resp->finalScore, 0.001);
        self::assertTrue($resp->wouldReturn);
        self::assertEqualsWithDelta(0.5, $resp->threshold, 0.001);
        self::assertSame(['token1'], $resp->ftsMatches);
        self::assertEqualsWithDelta(0.9, $resp->components->fullTextRelevance, 0.001);
    }

    // ── ContradictionItem — id_a, concept_a, etc. ───────────────

    public function testContradictionItemFieldNames(): void
    {
        $item = ContradictionItem::fromArray([
            'id_a'        => 'aaa',
            'concept_a'   => 'fire',
            'id_b'        => 'bbb',
            'concept_b'   => 'ice',
            'detected_at' => 1700001000,
        ]);
        self::assertSame('aaa', $item->idA);
        self::assertSame('fire', $item->conceptA);
        self::assertSame('bbb', $item->idB);
        self::assertSame('ice', $item->conceptB);
        self::assertSame(1700001000, $item->detectedAt);
    }

    public function testContradictionsResponseDeserializes(): void
    {
        $resp = ContradictionsResponse::fromArray([
            'contradictions' => [
                ['id_a' => 'a1', 'concept_a' => 'hot', 'id_b' => 'b1', 'concept_b' => 'cold', 'detected_at' => 0],
            ],
        ]);
        self::assertCount(1, $resp->contradictions);
        self::assertSame('a1', $resp->contradictions[0]->idA);
    }

    // ── CoherenceResult — all server fields ──────────────────────

    public function testCoherenceResultFieldNames(): void
    {
        $r = CoherenceResult::fromArray([
            'score'                  => 0.95,
            'orphan_ratio'           => 0.01,
            'contradiction_density'  => 0.02,
            'duplication_pressure'   => 0.03,
            'temporal_variance'      => 0.1,
            'total_engrams'          => 42,
        ]);
        self::assertEqualsWithDelta(0.95, $r->score, 0.001);
        self::assertEqualsWithDelta(0.01, $r->orphanRatio, 0.001);
        self::assertEqualsWithDelta(0.02, $r->contradictionDensity, 0.001);
        self::assertEqualsWithDelta(0.03, $r->duplicationPressure, 0.001);
        self::assertEqualsWithDelta(0.1, $r->temporalVariance, 0.001);
        self::assertSame(42, $r->totalEngrams);
    }

    // ── StatsResponse — server field names ──────────────────────

    public function testStatsResponseFieldNames(): void
    {
        $resp = StatsResponse::fromArray([
            'engram_count'  => 100,
            'vault_count'   => 3,
            'storage_bytes' => 204800,
        ]);
        self::assertSame(100, $resp->engramCount);
        self::assertSame(3, $resp->vaultCount);
        self::assertSame(204800, $resp->storageBytes);
        self::assertEmpty($resp->coherence);
    }

    public function testStatsResponseWithCoherence(): void
    {
        $resp = StatsResponse::fromArray([
            'engram_count'  => 50,
            'vault_count'   => 1,
            'storage_bytes' => 1024,
            'coherence'     => [
                'default' => ['score' => 0.9, 'orphan_ratio' => 0.05, 'contradiction_density' => 0.0,
                              'duplication_pressure' => 0.0, 'temporal_variance' => 0.0, 'total_engrams' => 50],
            ],
        ]);
        self::assertArrayHasKey('default', $resp->coherence);
        self::assertEqualsWithDelta(0.9, $resp->coherence['default']->score, 0.001);
    }

    // ── AssociationItem — target_id, rel_type, co_activation_count

    public function testAssociationItemFieldNames(): void
    {
        $item = AssociationItem::fromArray([
            'target_id'           => 'tgt',
            'rel_type'            => 5,
            'weight'              => 0.75,
            'co_activation_count' => 12,
            'restored_at'         => 1700002000,
        ]);
        self::assertSame('tgt', $item->targetId);
        self::assertSame(5, $item->relType);
        self::assertEqualsWithDelta(0.75, $item->weight, 0.001);
        self::assertSame(12, $item->coActivationCount);
        self::assertSame(1700002000, $item->restoredAt);
    }

    public function testAssociationItemRestoredAtNullWhenAbsent(): void
    {
        $item = AssociationItem::fromArray(['target_id' => 't', 'rel_type' => 1, 'weight' => 1.0]);
        self::assertNull($item->restoredAt);
        self::assertSame(0, $item->coActivationCount);
    }

    // ── SessionEntry — content + created_at ──────────────────────

    public function testSessionEntryFieldNames(): void
    {
        $entry = SessionEntry::fromArray([
            'id'         => 'e1',
            'concept'    => 'test',
            'content'    => 'body text',
            'created_at' => 1700000000,
        ]);
        self::assertSame('body text', $entry->content);
        self::assertSame(1700000000, $entry->createdAt);
    }

    // ── DeletedEngram — recoverable_until + tags ──────────────────

    public function testDeletedEngramFields(): void
    {
        $e = DeletedEngram::fromArray([
            'id'                => 'del1',
            'concept'           => 'gone',
            'deleted_at'        => 1700000000,
            'recoverable_until' => 1700086400,
            'tags'              => ['t1', 't2'],
        ]);
        self::assertSame('del1', $e->id);
        self::assertSame(1700000000, $e->deletedAt);
        self::assertSame(1700086400, $e->recoverableUntil);
        self::assertSame(['t1', 't2'], $e->tags);
    }

    public function testDeletedEngramDefaultsForOptionalFields(): void
    {
        $e = DeletedEngram::fromArray(['id' => 'x', 'concept' => 'y']);
        self::assertSame(0, $e->deletedAt);
        self::assertSame(0, $e->recoverableUntil);
        self::assertSame([], $e->tags);
    }

    // ── RetryEnrichResponse ───────────────────────────────────────

    public function testRetryEnrichResponseFieldNames(): void
    {
        $resp = RetryEnrichResponse::fromArray([
            'engram_id'       => 'e1',
            'plugins_queued'  => ['embed', 'ner'],
            'already_complete' => ['fts'],
            'note'            => 'ok',
        ]);
        self::assertSame('e1', $resp->engramId);
        self::assertSame(['embed', 'ner'], $resp->pluginsQueued);
        self::assertSame(['fts'], $resp->alreadyComplete);
        self::assertSame('ok', $resp->note);
    }

    // ── ActivateResponse ──────────────────────────────────────────

    public function testActivateResponseDeserializes(): void
    {
        $resp = ActivateResponse::fromArray([
            'query_id'    => 'q1',
            'total_found' => 2,
            'activations' => [
                ['id' => 'a1', 'concept' => 'hit', 'content' => 'body', 'score' => 0.9],
                ['id' => 'a2', 'concept' => 'miss', 'content' => 'body', 'score' => 0.3],
            ],
            'latency_ms' => 12.5,
        ]);
        self::assertSame(2, $resp->totalFound);
        self::assertSame('a1', $resp->activations[0]->id);
        self::assertEqualsWithDelta(12.5, $resp->latencyMs, 0.001);
    }

    // ── Exception mapping (via reflection on handleResponse) ──────

    public function testHandleResponseThrows401AsAuthException(): void
    {
        $client = new MuninnClient('http://localhost:8476', 'tok');
        $method = new \ReflectionMethod($client, 'handleResponse');
        $method->setAccessible(true);

        $this->expectException(AuthException::class);
        $method->invoke($client, 401, '{"error":"unauthorized"}');
    }

    public function testHandleResponseThrows404AsNotFoundException(): void
    {
        $client = new MuninnClient('http://localhost:8476', 'tok');
        $method = new \ReflectionMethod($client, 'handleResponse');
        $method->setAccessible(true);

        $this->expectException(NotFoundException::class);
        $method->invoke($client, 404, '{"error":"not found"}');
    }

    public function testHandleResponseThrows409AsConflictException(): void
    {
        $client = new MuninnClient('http://localhost:8476', 'tok');
        $method = new \ReflectionMethod($client, 'handleResponse');
        $method->setAccessible(true);

        $this->expectException(ConflictException::class);
        $method->invoke($client, 409, '{"error":"conflict"}');
    }

    public function testHandleResponseThrows422AsValidationException(): void
    {
        $client = new MuninnClient('http://localhost:8476', 'tok');
        $method = new \ReflectionMethod($client, 'handleResponse');
        $method->setAccessible(true);

        $this->expectException(ValidationException::class);
        $method->invoke($client, 422, '{"error":"bad input"}');
    }

    public function testHandleResponseThrows500AsServerException(): void
    {
        $client = new MuninnClient('http://localhost:8476', 'tok');
        $method = new \ReflectionMethod($client, 'handleResponse');
        $method->setAccessible(true);

        $this->expectException(ServerException::class);
        $method->invoke($client, 500, '{"error":"internal error"}');
    }

    public function testHandleResponseReturnsDecodedArrayOn200(): void
    {
        $client = new MuninnClient('http://localhost:8476', 'tok');
        $method = new \ReflectionMethod($client, 'handleResponse');
        $method->setAccessible(true);

        $result = $method->invoke($client, 200, '{"id":"abc","created_at":1700000000}');
        self::assertSame('abc', $result['id']);
        self::assertSame(1700000000, $result['created_at']);
    }
}
