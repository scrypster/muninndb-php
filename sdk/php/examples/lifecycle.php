<?php

declare(strict_types=1);

// If using Composer autoload:
// require_once __DIR__ . '/../vendor/autoload.php';

// Manual autoload for running without Composer:
spl_autoload_register(function (string $class) {
    $prefix = 'MuninnDB\\';
    if (strncmp($class, $prefix, strlen($prefix)) !== 0) return;
    $relative = substr($class, strlen($prefix));
    $file = __DIR__ . '/../src/' . str_replace('\\', '/', $relative) . '.php';
    if (file_exists($file)) require_once $file;
});

use MuninnDB\MuninnClient;

$client = new MuninnClient(
    baseUrl: 'http://localhost:8476',
    token: getenv('MUNINN_TOKEN') ?: '',
);

echo "=== Memory Lifecycle Example ===\n\n";

$original = $client->write(
    content: 'We chose PostgreSQL for the main application database.',
    concept: 'database choice',
    tags: ['architecture', 'database'],
);
echo "Created: {$original->id}\n";

$evolved = $client->evolve(
    id: $original->id,
    newContent: 'We migrated from PostgreSQL to CockroachDB for multi-region support.',
    reason: 'Multi-region expansion required distributed SQL',
);
echo "Evolved to: {$evolved->id}\n";

$stateResult = $client->setState(
    id: $evolved->id,
    state: 'active',
);
echo "State set: {$stateResult->state} (previous: {$stateResult->previousState})\n";

$decision = $client->decide(
    decision: 'Use CockroachDB for all new services',
    rationale: 'Multi-region support, PostgreSQL wire compatibility, and automatic sharding.',
    alternatives: ['Keep PostgreSQL with Citus', 'Switch to Spanner', 'Use Vitess'],
    evidenceIds: [$evolved->id],
);
echo "Decision recorded: {$decision->id}\n";

$m1 = $client->write(
    content: 'Using PgBouncer for connection pooling with max 100 connections.',
    concept: 'DB connection pooling',
);
$m2 = $client->write(
    content: 'Connection pool size set to 100, timeout 30s, idle timeout 10min.',
    concept: 'DB pooling config',
);

$consolidated = $client->consolidate(
    ids: [$m1->id, $m2->id],
    mergedContent: 'Using PgBouncer for connection pooling: max 100 connections, 30s timeout, 10min idle timeout.',
);
echo 'Consolidated ' . count($consolidated->archived) . " memories into: {$consolidated->id}\n";

$client->forget($consolidated->id);
echo "Soft-deleted: {$consolidated->id}\n";

$deleted = $client->listDeleted();
echo 'Recoverable memories: ' . count($deleted->engrams) . "\n";

$restored = $client->restore($consolidated->id);
echo "Restored: {$restored->id} (state: {$restored->state})\n";

$contradictions = $client->contradictions();
echo "\nContradictions: " . count($contradictions->contradictions) . "\n";

$guide = $client->guide();
echo "\nGuide preview: " . substr($guide, 0, 100) . "...\n";

$enrichResult = $client->retryEnrich($evolved->id);
echo "\nRe-enriched: {$enrichResult->id} (status: {$enrichResult->status})\n";
