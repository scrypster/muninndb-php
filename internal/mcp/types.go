package mcp

import (
	"encoding/json"
	"time"
)

// JSON-RPC 2.0 envelope types

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	ID      json.RawMessage `json:"id,omitempty"`
	Params  *JSONRPCParams  `json:"params,omitempty"`
}

type JSONRPCParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// AuthContext is returned by authFromRequest. Struct (not bool) so scopes can be added later.
type AuthContext struct {
	Token      string
	Authorized bool
}

// ToolDefinition is one entry in the tools/list response.
type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

// MCP domain types (used by EngineInterface and handlers)

type WriteResult struct {
	ID      string `json:"id"`
	Concept string `json:"concept"`
	Hint    string `json:"hint,omitempty"`
}

type Memory struct {
	ID         string    `json:"id"`
	Concept    string    `json:"concept"`
	Content    string    `json:"content"` // truncated to 500 chars if no Summary
	Summary    string    `json:"summary,omitempty"`
	Score      float64   `json:"score,omitempty"`
	Confidence float32   `json:"confidence"`
	Why        string    `json:"why,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	State      string    `json:"state"`
	CreatedAt  time.Time `json:"created_at"`
}

type ContradictionPair struct {
	IDa        string    `json:"id_a"`
	ConceptA   string    `json:"concept_a"`
	IDb        string    `json:"id_b"`
	ConceptB   string    `json:"concept_b"`
	DetectedAt time.Time `json:"detected_at"`
}

type VaultStatus struct {
	Vault         string `json:"vault"`
	TotalMemories int64  `json:"total_memories"`
	Health        string `json:"health"`
}

type SessionEntry struct {
	ID        string    `json:"id"`
	Concept   string    `json:"concept"`
	CreatedAt time.Time `json:"created_at"`
}

type SessionSummary struct {
	Writes      []SessionEntry `json:"writes"`
	Activations int            `json:"activations"`
	Since       time.Time      `json:"since"`
}

type ConsolidateResult struct {
	ID       string   `json:"id"`
	Archived []string `json:"archived"`
	Warnings []string `json:"warnings,omitempty"`
}

// Epic 18: New types for tools 12-17

// RestoreResult is returned by Restore on success.
type RestoreResult struct {
	ID      string `json:"id"`
	Concept string `json:"concept"`
	State   string `json:"state"`
}

// TraverseRequest defines parameters for a BFS graph traversal.
type TraverseRequest struct {
	StartID  string
	MaxHops  int
	MaxNodes int
	RelTypes []string
}

// TraverseResult is the output of a BFS graph traversal.
type TraverseResult struct {
	Nodes          []TraversalNode `json:"nodes"`
	Edges          []TraversalEdge `json:"edges"`
	TotalReachable int             `json:"total_reachable"`
	QueryMs        float64         `json:"query_ms"`
}

// TraversalNode is a single node returned in a traversal.
type TraversalNode struct {
	ID      string `json:"id"`
	Concept string `json:"concept"`
	HopDist int    `json:"hop_dist"`
	Summary string `json:"summary,omitempty"`
}

// TraversalEdge is an association edge returned in a traversal.
type TraversalEdge struct {
	FromID  string  `json:"from_id"`
	ToID    string  `json:"to_id"`
	RelType string  `json:"rel_type"`
	Weight  float32 `json:"weight"`
}

// ExplainRequest defines the context for a score explanation.
type ExplainRequest struct {
	EngramID string
	Query    []string
}

// ExplainComponents holds the per-component score breakdown.
type ExplainComponents struct {
	FullTextRelevance  float64 `json:"full_text_relevance"`
	SemanticSimilarity float64 `json:"semantic_similarity"`
	DecayFactor        float64 `json:"decay_factor"`
	HebbianBoost       float64 `json:"hebbian_boost"`
	AccessFrequency    float64 `json:"access_frequency"`
	Confidence         float64 `json:"confidence"`
}

// ExplainResult breaks down why an engram scored as it did for a given query.
type ExplainResult struct {
	EngramID    string            `json:"engram_id"`
	Concept     string            `json:"concept"`
	FinalScore  float64           `json:"final_score"`
	Components  ExplainComponents `json:"components"`
	FTSMatches  []string          `json:"fts_matches"`
	AssocPath   []string          `json:"assoc_path"`
	WouldReturn bool              `json:"would_return"`
	Threshold   float64           `json:"threshold"`
}

// DeletedEngram is a summary of a soft-deleted engram still within the recovery window.
type DeletedEngram struct {
	ID               string    `json:"id"`
	Concept          string    `json:"concept"`
	DeletedAt        time.Time `json:"deleted_at"`
	RecoverableUntil time.Time `json:"recoverable_until"`
	Tags             []string  `json:"tags,omitempty"`
}

// RetryEnrichResult reports which plugins were queued for re-processing.
type RetryEnrichResult struct {
	EngramID        string   `json:"engram_id"`
	PluginsQueued   []string `json:"plugins_queued"`
	AlreadyComplete []string `json:"already_complete"`
	Note            string   `json:"note,omitempty"`
}

// --- Cognitive push notification param types ---
// These are pre-serialized to json.RawMessage at emission sites.

// ContradictionParams is the params payload for "notifications/muninn/contradiction".
type ContradictionParams struct {
	IDa     string `json:"id_a"`
	IDb     string `json:"id_b"`
	Concept string `json:"concept,omitempty"`
}

// ActivationParams is the params payload for "notifications/muninn/activation".
type ActivationParams struct {
	ID      string  `json:"id"`
	Concept string  `json:"concept"`
	Score   float64 `json:"score"`
	Vault   string  `json:"vault"`
}

// AssociationParams is the params payload for "notifications/muninn/association".
type AssociationParams struct {
	SourceID string  `json:"source_id"`
	TargetID string  `json:"target_id"`
	Weight   float32 `json:"weight"`
}
