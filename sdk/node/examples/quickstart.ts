import { MuninnClient } from '../src/index';

async function main() {
  const client = new MuninnClient({
    baseUrl: 'http://localhost:8476',
    token: process.env.MUNINN_TOKEN || '',
  });

  try {
    const health = await client.health();
    console.log(`Server status: ${health.status}, version: ${health.version}`);

    const auth = await client.write({
      concept: 'auth architecture',
      content: 'We use short-lived JWTs (15min) with refresh tokens stored in HttpOnly cookies.',
      tags: ['auth', 'security', 'jwt'],
    });
    console.log(`Stored auth memory: ${auth.id}`);

    const deploy = await client.write({
      concept: 'deployment process',
      content: 'Blue-green deployments with Kubernetes rolling updates. Canary releases for critical services.',
      tags: ['devops', 'deployment'],
    });
    console.log(`Stored deploy memory: ${deploy.id}`);

    const result = await client.activate({
      context: ['reviewing the login flow for security audit'],
      limit: 5,
    });
    console.log(`\nRecall found ${result.total_found} memories:`);
    for (const item of result.activations) {
      console.log(`  [${item.score.toFixed(3)}] ${item.concept}: ${item.content.slice(0, 80)}...`);
    }

    const memory = await client.read(auth.id);
    console.log(`\nRead memory: ${memory.concept} (confidence: ${memory.confidence})`);

    await client.link({
      source_id: auth.id,
      target_id: deploy.id,
      rel_type: 'related',
      weight: 0.8,
    });
    console.log(`Linked ${auth.id} → ${deploy.id}`);

    const vaults = await client.listVaults();
    console.log(`\nVaults: ${vaults.join(', ')}`);

    const stats = await client.stats();
    console.log(`Total engrams: ${stats.total_engrams}`);
  } finally {
    client.close();
  }
}

main().catch(console.error);
