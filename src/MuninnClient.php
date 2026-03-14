<?php

declare(strict_types=1);

namespace MuninnDB;

use MuninnDB\Exceptions\AuthException;
use MuninnDB\Exceptions\ConflictException;
use MuninnDB\Exceptions\ConnectionException;
use MuninnDB\Exceptions\MuninnException;
use MuninnDB\Exceptions\NotFoundException;
use MuninnDB\Exceptions\ServerException;
use MuninnDB\Exceptions\TimeoutException;
use MuninnDB\Exceptions\ValidationException;
use MuninnDB\Types\ActivateResponse;
use MuninnDB\Types\AssociationItem;
use MuninnDB\Types\BatchWriteResponse;
use MuninnDB\Types\ConsolidateResponse;
use MuninnDB\Types\ContradictionsResponse;
use MuninnDB\Types\DecideResponse;
use MuninnDB\Types\Engram;
use MuninnDB\Types\EvolveResponse;
use MuninnDB\Types\ExplainResponse;
use MuninnDB\Types\HealthResponse;
use MuninnDB\Types\ListDeletedResponse;
use MuninnDB\Types\ListEngramsResponse;
use MuninnDB\Types\RestoreResponse;
use MuninnDB\Types\RetryEnrichResponse;
use MuninnDB\Types\SessionResponse;
use MuninnDB\Types\SetStateResponse;
use MuninnDB\Types\StatsResponse;
use MuninnDB\Types\TraverseResponse;
use MuninnDB\Types\WriteResponse;

class MuninnClient
{
    public function __construct(
        private readonly string $baseUrl = 'http://localhost:8476',
        private readonly string $token = '',
        private readonly float $timeout = 5.0,
        private readonly int $maxRetries = 3,
        private readonly float $retryBackoff = 0.5,
    ) {}

    // ── Core CRUD ────────────────────────────────────────────

    /**
     * Write a single engram.
     *
     * @param string[] $tags
     * @param string[] $entities
     * @param array<array{source?:string,target?:string,rel_type?:string}>|null $relationships
     */
    public function write(
        string $content,
        string $concept = '',
        string $vault = 'default',
        array $tags = [],
        float $confidence = 0.5,
        float $stability = 0.5,
        string $memoryType = '',
        string $typeLabel = '',
        string $summary = '',
        array $entities = [],
        ?array $relationships = null,
    ): WriteResponse {
        $body = array_filter([
            'vault'         => $vault,
            'concept'       => $concept,
            'content'       => $content,
            'tags'          => $tags,
            'confidence'    => $confidence,
            'stability'     => $stability,
            'memory_type'   => $memoryType,
            'type_label'    => $typeLabel,
            'summary'       => $summary,
            'entities'      => $entities,
            'relationships' => $relationships,
        ], fn(mixed $v) => $v !== '' && $v !== [] && $v !== null);

        return WriteResponse::fromArray(
            $this->request('POST', '/api/engrams?vault=' . urlencode($vault), $body),
        );
    }

    /**
     * Batch-write up to 50 engrams in one request.
     *
     * @param array<array<string,mixed>> $engrams Raw engram payloads
     */
    public function writeBatch(array $engrams, string $vault = 'default'): BatchWriteResponse
    {
        $prepared = array_map(function (array $e) use ($vault): array {
            $e['vault'] = $e['vault'] ?? $vault;
            return $e;
        }, $engrams);

        return BatchWriteResponse::fromArray(
            $this->request('POST', '/api/engrams/batch?vault=' . urlencode($vault), ['engrams' => $prepared]),
        );
    }

    /** Read a single engram by ID. */
    public function read(string $id, string $vault = 'default'): Engram
    {
        return Engram::fromArray(
            $this->request('GET', "/api/engrams/$id?vault=" . urlencode($vault)),
        );
    }

    /** Soft-delete (or hard-delete) an engram. */
    public function forget(string $id, string $vault = 'default', bool $hard = false): void
    {
        $qs = 'vault=' . urlencode($vault);
        if ($hard) {
            $qs .= '&hard=true';
        }
        $this->request('DELETE', "/api/engrams/$id?$qs");
    }

    /**
     * Recall / activate engrams by context.
     *
     * @param string[] $context
     */
    public function activate(
        array $context,
        string $vault = 'default',
        float $threshold = 0.0,
        int $maxResults = 10,
        int $maxHops = 0,
        string $profile = '',
        string $mode = '',
        ?string $since = null,
        ?string $before = null,
        bool $includeWhy = false,
        string $briefMode = '',
    ): ActivateResponse {
        // Always-present fields (threshold=0.0 is a valid value, must not be dropped).
        $body = [
            'vault'       => $vault,
            'context'     => $context,
            'threshold'   => $threshold,
            'max_results' => $maxResults,
        ];
        if ($maxHops > 0) {
            $body['max_hops'] = $maxHops;
        }
        if ($profile !== '') {
            $body['profile'] = $profile;
        }
        if ($mode !== '') {
            $body['mode'] = $mode;
        }
        if ($since !== null) {
            $body['since'] = $since;
        }
        if ($before !== null) {
            $body['before'] = $before;
        }
        if ($includeWhy) {
            $body['include_why'] = true;
        }
        if ($briefMode !== '') {
            $body['brief_mode'] = $briefMode;
        }

        return ActivateResponse::fromArray(
            $this->request('POST', '/api/activate?vault=' . urlencode($vault), $body),
        );
    }

    /** Create an association between two engrams. */
    public function link(
        string $sourceId,
        string $targetId,
        int $relType = 1,
        float $weight = 1.0,
        string $vault = 'default',
    ): void {
        $this->request('POST', '/api/link?vault=' . urlencode($vault), [
            'vault'     => $vault,
            'source_id' => $sourceId,
            'target_id' => $targetId,
            'rel_type'  => $relType,
            'weight'    => $weight,
        ]);
    }

    // ── Extended Operations ──────────────────────────────────

    /** Evolve (update) an engram's content. */
    public function evolve(string $id, string $newContent, string $reason, string $vault = 'default'): EvolveResponse
    {
        return EvolveResponse::fromArray(
            $this->request('POST', "/api/engrams/$id/evolve?vault=" . urlencode($vault), [
                'new_content' => $newContent,
                'reason'      => $reason,
                'vault'       => $vault,
            ]),
        );
    }

    /**
     * Consolidate (merge) multiple engrams into one.
     *
     * @param string[] $ids
     */
    public function consolidate(array $ids, string $mergedContent, string $vault = 'default'): ConsolidateResponse
    {
        return ConsolidateResponse::fromArray(
            $this->request('POST', '/api/consolidate?vault=' . urlencode($vault), [
                'vault'          => $vault,
                'ids'            => $ids,
                'merged_content' => $mergedContent,
            ]),
        );
    }

    /**
     * Record a decision with rationale.
     *
     * @param string[] $alternatives
     * @param string[] $evidenceIds
     */
    public function decide(
        string $decision,
        string $rationale,
        array $alternatives = [],
        array $evidenceIds = [],
        string $vault = 'default',
    ): DecideResponse {
        return DecideResponse::fromArray(
            $this->request('POST', '/api/decide?vault=' . urlencode($vault), array_filter([
                'vault'        => $vault,
                'decision'     => $decision,
                'rationale'    => $rationale,
                'alternatives' => $alternatives,
                'evidence_ids' => $evidenceIds,
            ], fn(mixed $v) => $v !== [])),
        );
    }

    /** Restore a soft-deleted engram. */
    public function restore(string $id, string $vault = 'default'): RestoreResponse
    {
        return RestoreResponse::fromArray(
            $this->request('POST', "/api/engrams/$id/restore?vault=" . urlencode($vault), ['vault' => $vault]),
        );
    }

    /**
     * Traverse the engram graph starting from a given node.
     *
     * @param string[] $relTypes Filter by relationship types
     */
    public function traverse(
        string $startId,
        int $maxHops = 2,
        int $maxNodes = 20,
        array $relTypes = [],
        bool $followEntities = false,
        string $vault = 'default',
    ): TraverseResponse {
        $body = [
            'vault'     => $vault,
            'start_id'  => $startId,
            'max_hops'  => $maxHops,
            'max_nodes' => $maxNodes,
        ];
        if ($relTypes !== []) {
            $body['rel_types'] = $relTypes;
        }
        if ($followEntities) {
            $body['follow_entities'] = true;
        }

        return TraverseResponse::fromArray(
            $this->request('POST', '/api/traverse?vault=' . urlencode($vault), $body),
        );
    }

    /**
     * Get a scoring explanation for an engram against a query.
     *
     * @param string[] $query
     */
    public function explain(string $engramId, array $query, string $vault = 'default'): ExplainResponse
    {
        return ExplainResponse::fromArray(
            $this->request('POST', '/api/explain?vault=' . urlencode($vault), [
                'vault'     => $vault,
                'engram_id' => $engramId,
                'query'     => $query,
            ]),
        );
    }

    /** Set an engram's workflow state. */
    public function setState(string $id, string $state, string $reason = '', string $vault = 'default'): SetStateResponse
    {
        $body = ['state' => $state, 'vault' => $vault];
        if ($reason !== '') {
            $body['reason'] = $reason;
        }

        return SetStateResponse::fromArray(
            $this->request('PUT', "/api/engrams/$id/state?vault=" . urlencode($vault), $body),
        );
    }

    /** List soft-deleted engrams. */
    public function listDeleted(string $vault = 'default', int $limit = 20): ListDeletedResponse
    {
        return ListDeletedResponse::fromArray(
            $this->request('GET', '/api/deleted?vault=' . urlencode($vault) . "&limit=$limit"),
        );
    }

    /** Retry enrichment for an engram. */
    public function retryEnrich(string $id, string $vault = 'default'): RetryEnrichResponse
    {
        return RetryEnrichResponse::fromArray(
            $this->request('POST', "/api/engrams/$id/retry-enrich?vault=" . urlencode($vault), ['vault' => $vault]),
        );
    }

    /** Get detected contradictions in a vault. */
    public function contradictions(string $vault = 'default'): ContradictionsResponse
    {
        return ContradictionsResponse::fromArray(
            $this->request('GET', '/api/contradictions?vault=' . urlencode($vault)),
        );
    }

    /** Get the human-readable guide for a vault. */
    public function guide(string $vault = 'default'): string
    {
        $data = $this->request('GET', '/api/guide?vault=' . urlencode($vault));
        return $data['guide'] ?? $data['text'] ?? json_encode($data);
    }

    // ── Query & List ─────────────────────────────────────────

    /** Get vault statistics. */
    public function stats(string $vault = 'default'): StatsResponse
    {
        return StatsResponse::fromArray(
            $this->request('GET', '/api/stats?vault=' . urlencode($vault)),
        );
    }

    /** List engrams with pagination. */
    public function listEngrams(string $vault = 'default', int $limit = 20, int $offset = 0): ListEngramsResponse
    {
        return ListEngramsResponse::fromArray(
            $this->request('GET', '/api/engrams?vault=' . urlencode($vault) . "&limit=$limit&offset=$offset"),
        );
    }

    /**
     * Get all links (associations) for an engram.
     *
     * @return AssociationItem[]
     */
    public function getLinks(string $id, string $vault = 'default'): array
    {
        $data = $this->request('GET', "/api/engrams/$id/links?vault=" . urlencode($vault));
        $links = $data['links'] ?? $data['associations'] ?? [];

        return array_map(
            fn(array $l) => AssociationItem::fromArray($l),
            $links,
        );
    }

    /**
     * List all vaults.
     *
     * @return string[]
     */
    public function listVaults(): array
    {
        $data = $this->request('GET', '/api/vaults');
        return $data['vaults'] ?? [];
    }

    /** Get session activity log. */
    public function session(
        string $vault = 'default',
        ?string $since = null,
        int $limit = 50,
        int $offset = 0,
    ): SessionResponse {
        $qs = 'vault=' . urlencode($vault) . "&limit=$limit&offset=$offset";
        if ($since !== null) {
            $qs .= '&since=' . urlencode($since);
        }

        return SessionResponse::fromArray(
            $this->request('GET', "/api/session?$qs"),
        );
    }

    // ── Streaming & Health ───────────────────────────────────

    /**
     * Open an SSE subscription for real-time engram events.
     * Returns an iterable SseStream — use in a foreach loop.
     */
    public function subscribe(string $vault = 'default', bool $pushOnWrite = true): SseStream
    {
        $qs = 'vault=' . urlencode($vault);
        if ($pushOnWrite) {
            $qs .= '&push_on_write=true';
        }

        return new SseStream(
            url: rtrim($this->baseUrl, '/') . "/api/subscribe?$qs",
            token: $this->token,
        );
    }

    /** Health-check the server. */
    public function health(): HealthResponse
    {
        return HealthResponse::fromArray(
            $this->request('GET', '/api/health'),
        );
    }

    // ── HTTP transport ───────────────────────────────────────

    /**
     * @return array<string,mixed> Decoded JSON response
     * @throws MuninnException
     */
    private function request(string $method, string $path, ?array $body = null): array
    {
        $url = rtrim($this->baseUrl, '/') . $path;
        $attempt = 0;

        while (true) {
            $attempt++;
            $ch = curl_init();

            $headers = ['Accept: application/json'];
            if ($this->token !== '') {
                $headers[] = 'Authorization: Bearer ' . $this->token;
            }

            $opts = [
                CURLOPT_URL            => $url,
                CURLOPT_RETURNTRANSFER => true,
                CURLOPT_TIMEOUT_MS     => (int) ($this->timeout * 1000),
                CURLOPT_CONNECTTIMEOUT => max(1, (int) $this->timeout),
                CURLOPT_HTTPHEADER     => $headers,
                CURLOPT_FOLLOWLOCATION => true,
            ];

            switch (strtoupper($method)) {
                case 'GET':
                    $opts[CURLOPT_HTTPGET] = true;
                    break;
                case 'POST':
                    $opts[CURLOPT_POST] = true;
                    if ($body !== null) {
                        $json = json_encode($body, JSON_THROW_ON_ERROR);
                        $opts[CURLOPT_POSTFIELDS] = $json;
                        $headers[] = 'Content-Type: application/json';
                        $opts[CURLOPT_HTTPHEADER] = $headers;
                    }
                    break;
                case 'PUT':
                    $opts[CURLOPT_CUSTOMREQUEST] = 'PUT';
                    if ($body !== null) {
                        $json = json_encode($body, JSON_THROW_ON_ERROR);
                        $opts[CURLOPT_POSTFIELDS] = $json;
                        $headers[] = 'Content-Type: application/json';
                        $opts[CURLOPT_HTTPHEADER] = $headers;
                    }
                    break;
                case 'DELETE':
                    $opts[CURLOPT_CUSTOMREQUEST] = 'DELETE';
                    break;
                default:
                    $opts[CURLOPT_CUSTOMREQUEST] = strtoupper($method);
                    break;
            }

            curl_setopt_array($ch, $opts);

            $responseBody = curl_exec($ch);
            $errno         = curl_errno($ch);
            $error         = curl_error($ch);
            $httpCode      = (int) curl_getinfo($ch, CURLINFO_HTTP_CODE);

            curl_close($ch);

            // Connection / timeout errors — retry
            if ($errno !== 0) {
                if ($attempt < $this->maxRetries) {
                    $this->backoff($attempt);
                    continue;
                }
                if ($errno === CURLE_OPERATION_TIMEDOUT) {
                    throw new TimeoutException("Request timed out: $error");
                }
                throw new ConnectionException("Connection failed: $error", $errno);
            }

            // Server errors — retry
            if ($httpCode >= 500 && $attempt < $this->maxRetries) {
                $this->backoff($attempt);
                continue;
            }

            return $this->handleResponse($httpCode, is_string($responseBody) ? $responseBody : '');
        }
    }

    /**
     * @return array<string,mixed>
     * @throws MuninnException
     */
    private function handleResponse(int $httpCode, string $body): array
    {
        $decoded = json_decode($body, true) ?? [];

        if ($httpCode >= 200 && $httpCode < 300) {
            return $decoded;
        }

        $message = $decoded['error'] ?? $decoded['message'] ?? "HTTP $httpCode";

        throw match (true) {
            $httpCode === 401 => new AuthException($message),
            $httpCode === 404 => new NotFoundException($message),
            $httpCode === 409 => new ConflictException($message, $body),
            $httpCode === 400, $httpCode === 422 => new ValidationException($message, $body),
            $httpCode >= 500 => new ServerException($message, $httpCode, $body),
            default => new MuninnException($message, $httpCode, responseBody: $body),
        };
    }

    private function backoff(int $attempt): void
    {
        $delay = $this->retryBackoff * (2 ** ($attempt - 1));
        usleep((int) ($delay * 1_000_000));
    }
}
