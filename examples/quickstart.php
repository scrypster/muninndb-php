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

$health = $client->health();
echo "Server status: {$health->status}, version: {$health->version}\n";

$auth = $client->write(
    content: 'We use short-lived JWTs (15min) with refresh tokens stored in HttpOnly cookies.',
    concept: 'auth architecture',
    tags: ['auth', 'security', 'jwt'],
);
echo "Stored auth memory: {$auth->id}\n";

$deploy = $client->write(
    content: 'Blue-green deployments with Kubernetes rolling updates. Canary releases for critical services.',
    concept: 'deployment process',
    tags: ['devops', 'deployment'],
);
echo "Stored deploy memory: {$deploy->id}\n";

$result = $client->activate(
    context: ['reviewing the login flow for security audit'],
    limit: 5,
);
echo "\nRecall found {$result->totalFound} memories:\n";
foreach ($result->activations as $item) {
    $score = number_format($item->score, 3);
    $preview = substr($item->content, 0, 80);
    echo "  [{$score}] {$item->concept}: {$preview}...\n";
}

$memory = $client->read($auth->id);
echo "\nRead memory: {$memory->concept} (confidence: {$memory->confidence})\n";

$client->link(
    sourceId: $auth->id,
    targetId: $deploy->id,
    relType: 'related',
    weight: 0.8,
);
echo "Linked {$auth->id} → {$deploy->id}\n";

$vaults = $client->listVaults();
echo "\nVaults: " . implode(', ', $vaults) . "\n";

$stats = $client->stats();
echo "Total engrams: {$stats->totalEngrams}, links: {$stats->totalLinks}\n";
