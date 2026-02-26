import { MuninnClient } from '../src/index';

async function main() {
  const client = new MuninnClient({
    baseUrl: 'http://localhost:8476',
    token: process.env.MUNINN_TOKEN || '',
  });

  try {
    console.log('=== Memory Lifecycle Example ===\n');

    const original = await client.write({
      concept: 'database choice',
      content: 'We chose PostgreSQL for the main application database.',
      tags: ['architecture', 'database'],
    });
    console.log(`Created: ${original.id}`);

    const evolved = await client.evolve(
      original.id,
      'We migrated from PostgreSQL to CockroachDB for multi-region support.',
      'Multi-region expansion required distributed SQL',
    );
    console.log(`Evolved to: ${evolved.id}`);

    const stateResult = await client.setState(evolved.id, 'active');
    console.log(`State set: ${stateResult.state} (previous: ${stateResult.previous_state})`);

    const decision = await client.decide({
      decision: 'Use CockroachDB for all new services',
      rationale: 'Multi-region support, PostgreSQL wire compatibility, and automatic sharding.',
      alternatives: ['Keep PostgreSQL with Citus', 'Switch to Spanner', 'Use Vitess'],
      evidence_ids: [evolved.id],
    });
    console.log(`Decision recorded: ${decision.id}`);

    const m1 = await client.write({
      concept: 'DB connection pooling',
      content: 'Using PgBouncer for connection pooling with max 100 connections.',
    });
    const m2 = await client.write({
      concept: 'DB pooling config',
      content: 'Connection pool size set to 100, timeout 30s, idle timeout 10min.',
    });

    const consolidated = await client.consolidate({
      ids: [m1.id, m2.id],
      merged_content: 'Using PgBouncer for connection pooling: max 100 connections, 30s timeout, 10min idle timeout.',
    });
    console.log(`Consolidated ${consolidated.archived.length} memories into: ${consolidated.id}`);

    await client.forget(consolidated.id);
    console.log(`Soft-deleted: ${consolidated.id}`);

    const deleted = await client.listDeleted();
    console.log(`Recoverable memories: ${deleted.count}`);

    const restored = await client.restore(consolidated.id);
    console.log(`Restored: ${restored.id} (state: ${restored.state})`);

    const contradictions = await client.contradictions();
    console.log(`\nContradictions: ${contradictions.contradictions.length}`);

    const guide = await client.guide();
    console.log(`\nGuide preview: ${guide.slice(0, 100)}...`);

    const enrichResult = await client.retryEnrich(evolved.id);
    console.log(`\nRe-enriched: ${enrichResult.plugins_queued.join(', ')}`);
  } finally {
    client.close();
  }
}

main().catch(console.error);
