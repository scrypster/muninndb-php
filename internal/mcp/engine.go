package mcp

import (
	"context"
	"time"

	"github.com/scrypster/muninndb/internal/auth"
	"github.com/scrypster/muninndb/internal/transport/mbp"
)

// EngineInterface is the API surface the MCP layer uses.
// The first 6 methods delegate directly to engine via mbp types (stable internal contract).
// The last 5 methods are higher-level operations with no MBP counterpart.
// Implemented by mcpEngineAdapter in internal/mcp/engine_adapter.go.
type EngineInterface interface {
	// MBP-backed methods
	Write(ctx context.Context, req *mbp.WriteRequest) (*mbp.WriteResponse, error)
	WriteBatch(ctx context.Context, reqs []*mbp.WriteRequest) ([]*mbp.WriteResponse, []error)
	Activate(ctx context.Context, req *mbp.ActivateRequest) (*mbp.ActivateResponse, error)
	Read(ctx context.Context, req *mbp.ReadRequest) (*mbp.ReadResponse, error)
	Forget(ctx context.Context, req *mbp.ForgetRequest) (*mbp.ForgetResponse, error)
	Link(ctx context.Context, req *mbp.LinkRequest) (*mbp.LinkResponse, error)
	Stat(ctx context.Context, req *mbp.StatRequest) (*mbp.StatResponse, error)

	// Higher-level cognitive operations (tools 1-11)
	GetContradictions(ctx context.Context, vault string) ([]ContradictionPair, error)
	Evolve(ctx context.Context, vault, oldID, newContent, reason string) (*WriteResult, error)
	Consolidate(ctx context.Context, vault string, ids []string, mergedContent string) (*ConsolidateResult, error)
	Session(ctx context.Context, vault string, since time.Time) (*SessionSummary, error)
	Decide(ctx context.Context, vault, decision, rationale string, alternatives, evidenceIDs []string) (*WriteResult, error)

	// Epic 18: tools 12-17

	// Restore un-deletes a soft-deleted engram within the 7-day recovery window.
	// Returns an error if the engram does not exist or the window has passed.
	Restore(ctx context.Context, vault string, id string) (*RestoreResult, error)

	// Traverse performs a bounded BFS from the starting engram, following association edges.
	Traverse(ctx context.Context, vault string, req *TraverseRequest) (*TraverseResult, error)

	// Explain returns the score breakdown for why a specific engram would be returned
	// for a given query context.
	Explain(ctx context.Context, vault string, req *ExplainRequest) (*ExplainResult, error)

	// UpdateState transitions an engram to a new lifecycle state.
	// Invalid transitions return an error describing the valid next states.
	UpdateState(ctx context.Context, vault string, id string, state string, reason string) error

	// ListDeleted returns engrams that have been soft-deleted and are still within
	// the 7-day recovery window, ordered by deletion time descending.
	ListDeleted(ctx context.Context, vault string, limit int) ([]DeletedEngram, error)

	// RetryEnrich re-queues an engram for enrichment by all active plugins that have
	// not yet processed it. Returns an error if the engram is not found.
	RetryEnrich(ctx context.Context, vault string, id string) (*RetryEnrichResult, error)

	// GetVaultPlasticity returns the resolved plasticity config for a vault.
	GetVaultPlasticity(ctx context.Context, vault string) (*auth.ResolvedPlasticity, error)
}
