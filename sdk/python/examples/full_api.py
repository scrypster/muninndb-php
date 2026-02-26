"""MuninnDB Python SDK — Full API Demo

Demonstrates all cognitive operations including the extended API:
evolve, consolidate, decide, traverse, explain, lifecycle states,
and recovery operations.

Usage:
    pip install muninn-python
    python full_api.py
"""

import asyncio
import os

from muninn import MuninnClient


async def main():
    async with MuninnClient(
        base_url="http://localhost:8476",
        token=os.getenv("MUNINN_TOKEN", ""),
    ) as client:
        print("=== MuninnDB Full API Demo ===\n")

        # Write some memories
        auth = await client.write(
            concept="auth design",
            content="JWT tokens with 15-minute expiry and HttpOnly refresh cookies.",
            tags=["auth", "security"],
        )
        print(f"Stored: {auth.id}")

        deploy = await client.write(
            concept="deploy strategy",
            content="Blue-green deployments via Kubernetes with canary releases.",
            tags=["devops"],
        )
        print(f"Stored: {deploy.id}")

        # Evolve a memory
        evolved = await client.evolve(
            auth.id,
            new_content="Migrated to OAuth2 with PKCE flow for public clients.",
            reason="Security audit recommended PKCE for mobile apps",
        )
        print(f"\nEvolved to: {evolved.id}")

        # Set lifecycle state
        state_result = await client.set_state(evolved.id, "active")
        print(f"State: {state_result.state} (updated: {state_result.updated})")

        # Record a decision
        decision = await client.decide(
            decision="Adopt OAuth2 with PKCE for all public clients",
            rationale="PKCE prevents authorization code interception attacks.",
            alternatives=["Keep JWT-only flow", "Use device code flow"],
            evidence_ids=[evolved.id],
        )
        print(f"Decision: {decision.id}")

        # Link memories
        await client.link(
            source_id=evolved.id,
            target_id=deploy.id,
            rel_type=5,
            weight=0.8,
        )
        print(f"Linked {evolved.id} → {deploy.id}")

        # Traverse the graph
        graph = await client.traverse(start_id=evolved.id, max_hops=2)
        print(f"\nGraph: {len(graph.nodes)} nodes, {len(graph.edges)} edges")
        for node in graph.nodes:
            print(f"  [hop {node.hop_dist}] {node.concept}")

        # Explain a score
        explanation = await client.explain(
            engram_id=evolved.id,
            query=["OAuth2", "authentication"],
        )
        print(
            f"\nExplain: score={explanation.final_score:.4f}, "
            f"would_return={explanation.would_return}"
        )

        # Get links
        links = await client.get_links(evolved.id)
        print(f"Links: {len(links)} associations")

        # Consolidate related memories
        m1 = await client.write(
            concept="DB pooling",
            content="PgBouncer with 100 max connections.",
        )
        m2 = await client.write(
            concept="DB config",
            content="Pool size 100, timeout 30s.",
        )
        consolidated = await client.consolidate(
            ids=[m1.id, m2.id],
            merged_content="PgBouncer: 100 max connections, 30s timeout.",
        )
        print(f"\nConsolidated {len(consolidated.archived)} into: {consolidated.id}")

        # Delete and restore
        await client.forget(consolidated.id)
        deleted = await client.list_deleted()
        print(f"Deleted memories: {deleted.count}")

        restored = await client.restore(consolidated.id)
        print(f"Restored: {restored.id} (state: {restored.state})")

        # Contradictions
        contradictions = await client.contradictions()
        print(f"\nContradictions: {len(contradictions.contradictions)}")

        # Guide
        guide_text = await client.guide()
        if len(guide_text) > 80:
            print(f"Guide: {guide_text[:80]}...")
        else:
            print(f"Guide: {guide_text}")

        # List engrams
        engrams = await client.list_engrams(limit=5)
        print(f"\nEngrams: {engrams.total} total, showing {len(engrams.engrams)}")

        # List vaults
        vaults = await client.list_vaults()
        print(f"Vaults: {vaults}")

        # Session activity
        session = await client.session()
        print(f"Session entries: {session.total}")

        # Re-enrich
        try:
            enrich_result = await client.retry_enrich(evolved.id)
            print(f"\nRe-enriched: {enrich_result.plugins_queued}")
        except Exception as e:
            print(f"\nRe-enrich (expected if no plugin): {e}")

        print("\n=== Done ===")


if __name__ == "__main__":
    asyncio.run(main())
