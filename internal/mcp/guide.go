package mcp

import (
	"fmt"
	"strings"

	"github.com/scrypster/muninndb/internal/auth"
)

type engineStats struct {
	EngramCount int64
	VaultCount  int
}

func generateGuide(vaultName string, resolved auth.ResolvedPlasticity, stats engineStats) string {
	var b strings.Builder

	// Header
	fmt.Fprintf(&b, "# MuninnDB Memory Guide for vault: %s\n\n", vaultName)

	// Memory Strategy
	b.WriteString("## Memory Strategy\n\n")
	switch resolved.BehaviorMode {
	case "prompted":
		b.WriteString("Only store memories when the user explicitly asks you to remember something. ")
		b.WriteString("Use recall when the user asks you to search their memory.\n")
	case "selective":
		b.WriteString("Automatically remember decisions, errors, and their resolutions. ")
		b.WriteString("For other information, only remember when the user asks. ")
		b.WriteString("Always recall before starting tasks that relate to previous work.\n")
	case "custom":
		if resolved.BehaviorInstructions != "" {
			b.WriteString(resolved.BehaviorInstructions)
			b.WriteString("\n")
		} else {
			b.WriteString("Custom behavior mode is configured but no instructions were provided. ")
			b.WriteString("Falling back to autonomous behavior.\n")
		}
	default: // "autonomous" and fallback
		b.WriteString("You should proactively remember important information without being asked. ")
		b.WriteString("Remember: decisions and their rationale, user preferences, errors and their fixes, ")
		b.WriteString("project context, important facts, and anything the user might need later. ")
		b.WriteString("Before starting any task, recall relevant memories. ")
		b.WriteString("After completing work, remember key outcomes.\n")
	}

	// Enrichment guidance based on behavior mode + inline enrichment setting
	if resolved.InlineEnrichment != "background_only" && resolved.InlineEnrichment != "disabled" {
		b.WriteString("\n## Enrichment\n\n")
		switch resolved.BehaviorMode {
		case "autonomous":
			b.WriteString("When remembering, include type, summary, and any entities you can identify. ")
			b.WriteString("This data is stored directly and avoids extra background processing. ")
			b.WriteString("Example: `{\"content\": \"...\", \"type\": \"decision\", \"summary\": \"Chose PostgreSQL for persistence\", ")
			b.WriteString("\"entities\": [{\"name\": \"PostgreSQL\", \"type\": \"database\"}]}`\n")
		case "selective":
			b.WriteString("Include type and summary when remembering decisions and errors. ")
			b.WriteString("This improves retrieval quality without extra processing cost.\n")
		case "custom":
			// Custom mode: no enrichment guidance — user controls behavior.
		default:
			// "prompted": don't mention enrichment.
		}
	}

	// Quick Reference
	b.WriteString("\n## Available Tools\n\n")
	b.WriteString("- **muninn_remember** — Store a new memory\n")
	b.WriteString("- **muninn_remember_batch** — Store multiple memories at once (max 50)\n")
	b.WriteString("- **muninn_recall** — Search memories by semantic context\n")
	b.WriteString("- **muninn_read** — Fetch a single memory by ID\n")
	b.WriteString("- **muninn_forget** — Soft-delete a memory\n")
	b.WriteString("- **muninn_link** — Create associations between memories\n")
	b.WriteString("- **muninn_contradictions** — Check for known contradictions\n")
	b.WriteString("- **muninn_status** — Get vault health and stats\n")
	b.WriteString("- **muninn_evolve** — Update a memory with new information\n")
	b.WriteString("- **muninn_consolidate** — Merge related memories into one\n")
	b.WriteString("- **muninn_session** — Get recent memory activity summary\n")
	b.WriteString("- **muninn_decide** — Record a decision with rationale\n")
	b.WriteString("- **muninn_restore** — Recover a soft-deleted memory\n")
	b.WriteString("- **muninn_traverse** — Explore the memory graph from a starting node\n")
	b.WriteString("- **muninn_explain** — Show score breakdown for a memory\n")
	b.WriteString("- **muninn_state** — Transition a memory's lifecycle state\n")
	b.WriteString("- **muninn_list_deleted** — List recoverable deleted memories\n")
	b.WriteString("- **muninn_retry_enrich** — Re-queue a memory for enrichment\n")

	// Vault Configuration Summary
	b.WriteString("\n## Vault Configuration\n\n")
	fmt.Fprintf(&b, "- Memories stored: %d\n", stats.EngramCount)
	fmt.Fprintf(&b, "- Behavior mode: %s\n", resolved.BehaviorMode)
	fmt.Fprintf(&b, "- Hebbian learning: %s\n", enabledStr(resolved.HebbianEnabled))
	fmt.Fprintf(&b, "- Predictive activation (PAS): %s\n", enabledStr(resolved.PredictiveActivation))
	fmt.Fprintf(&b, "- Graph hop depth: %d\n", resolved.HopDepth)
	fmt.Fprintf(&b, "- Temporal decay: %s\n", enabledStr(resolved.TemporalEnabled))
	fmt.Fprintf(&b, "- Inline enrichment: %s\n", resolved.InlineEnrichment)
	if resolved.MaxEngrams > 0 {
		fmt.Fprintf(&b, "- Max engrams: %d\n", resolved.MaxEngrams)
	}
	if resolved.RetentionDays > 0 {
		fmt.Fprintf(&b, "- Retention: %.0f days\n", resolved.RetentionDays)
	}

	// Memory quality guidance
	b.WriteString("\n## Writing Effective Memories\n\n")
	b.WriteString("**Keep memories atomic.** Each memory should capture one concept, one decision, or one fact. ")
	b.WriteString("If a conversation covers multiple topics, store each as a separate memory. ")
	b.WriteString("Use muninn_remember_batch to store multiple atomic memories efficiently in a single call.\n\n")
	b.WriteString("Why this matters:\n")
	b.WriteString("- Atomic memories produce sharper embeddings, so recall is more precise.\n")
	b.WriteString("- Associations between small, focused memories are more meaningful than links to monolithic blocks.\n")
	b.WriteString("- Contradiction detection works better when each memory makes one clear claim.\n")
	b.WriteString("- Deduplication can identify overlaps more accurately.\n\n")
	b.WriteString("**Bad:** \"We discussed auth, decided on JWTs with 15-min expiry, and Tom will implement rate limiting at 100 req/s.\"\n")
	b.WriteString("**Good:** Three separate memories:\n")
	b.WriteString("  1. \"Decided on JWTs with 15-minute expiry for authentication\" (type: decision)\n")
	b.WriteString("  2. \"Tom is implementing the auth system\" (type: task)\n")
	b.WriteString("  3. \"API rate limit set to 100 requests/second per client\" (type: decision)\n")

	// Tips
	b.WriteString("\n## Tips\n\n")
	b.WriteString("- Use muninn_recall with mode='deep' for thorough searches across the memory graph.\n")
	b.WriteString("- Use muninn_link to connect related memories and strengthen the knowledge graph.\n")
	b.WriteString("- Use muninn_decide to record decisions — they automatically link to supporting evidence.\n")
	b.WriteString("- Use muninn_evolve instead of forget+remember when updating existing information.\n")
	b.WriteString("- Use muninn_remember_batch when storing multiple memories from the same conversation.\n")

	return b.String()
}

func enabledStr(v bool) string {
	if v {
		return "enabled"
	}
	return "disabled"
}
