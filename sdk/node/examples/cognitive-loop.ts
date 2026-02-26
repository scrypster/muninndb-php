import { MuninnClient } from '../src/index';

async function main() {
  const client = new MuninnClient({
    baseUrl: 'http://localhost:8476',
    token: process.env.MUNINN_TOKEN || '',
  });

  try {
    console.log('=== Cognitive Loop Example ===\n');

    const memories = await client.writeBatch('default', [
      {
        concept: 'transformer architecture',
        content: 'Self-attention mechanism allows parallel processing of sequence elements. O(n²) complexity for sequence length n.',
        tags: ['ml', 'architecture'],
      },
      {
        concept: 'attention mechanism',
        content: 'Query-Key-Value attention: softmax(QK^T/√d)V. Multi-head attention uses h parallel attention functions.',
        tags: ['ml', 'attention'],
      },
      {
        concept: 'positional encoding',
        content: 'Sinusoidal position embeddings added to input embeddings. Learned positional embeddings also effective.',
        tags: ['ml', 'encoding'],
      },
    ]);
    const ids = memories.results.map(r => r.id).filter((id): id is string => !!id);
    console.log(`Stored ${ids.length} memories`);

    if (ids.length >= 3) {
      await client.link({ source_id: ids[0], target_id: ids[1], rel_type: 'related', weight: 0.9 });
      await client.link({ source_id: ids[1], target_id: ids[2], rel_type: 'related', weight: 0.8 });
      console.log('Created association graph');
    }

    const result = await client.activate({
      context: ['how does self-attention work in transformers?'],
      limit: 10,
      include_why: true,
    });
    console.log(`\nRecall returned ${result.total_found} memories:`);
    for (const item of result.activations) {
      console.log(`  [${item.score.toFixed(3)}] ${item.concept}`);
    }

    if (ids[0]) {
      const graph = await client.traverse({
        start_id: ids[0],
        max_hops: 2,
        max_nodes: 10,
      });
      console.log(`\nGraph traversal from "${ids[0]}":`);
      console.log(`  Nodes: ${graph.nodes.length}, Edges: ${graph.edges.length}`);
      for (const node of graph.nodes) {
        console.log(`  [depth ${node.depth}] ${node.concept}`);
      }
    }

    if (ids[0]) {
      const explanation = await client.explain({
        engram_id: ids[0],
        query: ['self-attention', 'transformer'],
      });
      console.log(`\nScore explanation for "${explanation.concept}":`);
      console.log(`  Final score: ${explanation.final_score.toFixed(4)}`);
      console.log(`  Would return: ${explanation.would_return}`);
      console.log(`  Components: Semantic=${explanation.components.semantic.toFixed(3)}, Recency=${explanation.components.recency.toFixed(3)}, Confidence=${explanation.components.confidence.toFixed(3)}`);
    }

    if (ids[0]) {
      const links = await client.getLinks(ids[0]);
      console.log(`\nAssociations for ${ids[0]}: ${links.length} links`);
    }
  } finally {
    client.close();
  }
}

main().catch(console.error);
