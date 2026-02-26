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

echo "=== Cognitive Loop Example ===\n\n";

$memories = $client->writeBatch([
    [
        'concept' => 'transformer architecture',
        'content' => 'Self-attention mechanism allows parallel processing of sequence elements. O(n²) complexity for sequence length n.',
        'tags'    => ['ml', 'architecture'],
    ],
    [
        'concept' => 'attention mechanism',
        'content' => 'Query-Key-Value attention: softmax(QK^T/√d)V. Multi-head attention uses h parallel attention functions.',
        'tags'    => ['ml', 'attention'],
    ],
    [
        'concept' => 'positional encoding',
        'content' => 'Sinusoidal position embeddings added to input embeddings. Learned positional embeddings also effective.',
        'tags'    => ['ml', 'encoding'],
    ],
]);
$ids = array_filter(array_map(fn($r) => $r->id, $memories->results));
echo 'Stored ' . count($ids) . " memories\n";

$ids = array_values($ids);
if (count($ids) >= 3) {
    $client->link(sourceId: $ids[0], targetId: $ids[1], relType: 'related', weight: 0.9);
    $client->link(sourceId: $ids[1], targetId: $ids[2], relType: 'related', weight: 0.8);
    echo "Created association graph\n";
}

$result = $client->activate(
    context: ['how does self-attention work in transformers?'],
    limit: 10,
    includeWhy: true,
);
echo "\nRecall returned {$result->totalFound} memories:\n";
foreach ($result->activations as $item) {
    $score = number_format($item->score, 3);
    echo "  [{$score}] {$item->concept}\n";
}

if (isset($ids[0])) {
    $graph = $client->traverse(
        startId: $ids[0],
        maxHops: 2,
        maxNodes: 10,
    );
    echo "\nGraph traversal from \"{$ids[0]}\":\n";
    echo '  Nodes: ' . count($graph->nodes) . ', Edges: ' . count($graph->edges) . "\n";
    foreach ($graph->nodes as $node) {
        $depth = $node->depth ?? 0;
        echo "  [depth {$depth}] {$node->concept}\n";
    }
}

if (isset($ids[0])) {
    $explanation = $client->explain(
        engramId: $ids[0],
        query: ['self-attention', 'transformer'],
    );
    $finalScore = number_format($explanation->finalScore, 4);
    echo "\nScore explanation for \"{$explanation->engramId}\":\n";
    echo "  Final score: {$finalScore}\n";
    $semantic = $explanation->components->semantic !== null
        ? number_format($explanation->components->semantic, 3)
        : 'n/a';
    $recency = $explanation->components->recency !== null
        ? number_format($explanation->components->recency, 3)
        : 'n/a';
    $confidence = $explanation->components->confidence !== null
        ? number_format($explanation->components->confidence, 3)
        : 'n/a';
    echo "  Components: Semantic={$semantic}, Recency={$recency}, Confidence={$confidence}\n";
}

if (isset($ids[0])) {
    $links = $client->getLinks($ids[0]);
    echo "\nAssociations for {$ids[0]}: " . count($links) . " links\n";
}
