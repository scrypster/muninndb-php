package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/scrypster/muninndb/internal/storage"
	"github.com/scrypster/muninndb/internal/transport/mbp"
)

// ── helpers ─────────────────────────────────────────────────────────────────

// newTestServerWith creates a server backed by the supplied engine.
func newTestServerWith(eng EngineInterface) *MCPServer {
	return New(":0", eng, "")
}

// extractInnerJSON decodes the MCP textContent envelope and returns the inner
// JSON as a map. The result from sendResult(…, textContent(mustJSON(v))) is:
//
//	{"content":[{"type":"text","text":"<inner json>"}]}
func extractInnerJSON(t *testing.T, resp JSONRPCResponse) map[string]any {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}
	wrapper, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected result to be an object, got %T", resp.Result)
	}
	contents, ok := wrapper["content"].([]any)
	if !ok || len(contents) == 0 {
		t.Fatal("expected result.content to be a non-empty array")
	}
	item, ok := contents[0].(map[string]any)
	if !ok {
		t.Fatalf("expected result.content[0] to be an object, got %T", contents[0])
	}
	text, ok := item["text"].(string)
	if !ok {
		t.Fatalf("expected result.content[0].text to be a string, got %T", item["text"])
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal inner JSON: %v — text was: %s", err, text)
	}
	return out
}

// decodeResp decodes the raw HTTP response body into a JSONRPCResponse.
func decodeResp(t *testing.T, body string) JSONRPCResponse {
	t.Helper()
	var resp JSONRPCResponse
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

// ── per-handler error-injection engines ─────────────────────────────────────
// Each embeds fakeEngine so the other 16 methods are covered.

type restoreErrEngine struct{ fakeEngine }

func (e *restoreErrEngine) Restore(_ context.Context, _ string, _ string) (*RestoreResult, error) {
	return nil, fmt.Errorf("engram not found or recovery window expired")
}

type traverseErrEngine struct{ fakeEngine }

func (e *traverseErrEngine) Traverse(_ context.Context, _ string, _ *TraverseRequest) (*TraverseResult, error) {
	return nil, fmt.Errorf("start node not found")
}

type explainErrEngine struct{ fakeEngine }

func (e *explainErrEngine) Explain(_ context.Context, _ string, _ *ExplainRequest) (*ExplainResult, error) {
	return nil, fmt.Errorf("engram not found")
}

type stateErrEngine struct{ fakeEngine }

func (e *stateErrEngine) UpdateState(_ context.Context, _ string, _ string, _ string, _ string) error {
	return fmt.Errorf("invalid transition: archived has no valid next states")
}

type listDeletedErrEngine struct{ fakeEngine }

func (e *listDeletedErrEngine) ListDeleted(_ context.Context, _ string, _ int) ([]DeletedEngram, error) {
	return nil, fmt.Errorf("storage error")
}

type retryEnrichErrEngine struct{ fakeEngine }

func (e *retryEnrichErrEngine) RetryEnrich(_ context.Context, _ string, _ string) (*RetryEnrichResult, error) {
	return nil, fmt.Errorf("engram not found")
}

// traverseWithNodesEngine returns a populated TraverseResult for shape tests.
type traverseWithNodesEngine struct{ fakeEngine }

func (e *traverseWithNodesEngine) Traverse(_ context.Context, _ string, _ *TraverseRequest) (*TraverseResult, error) {
	return &TraverseResult{
		Nodes: []TraversalNode{
			{ID: "s1", Concept: "start node", HopDist: 0},
			{ID: "n1", Concept: "neighbor", HopDist: 1},
		},
		Edges: []TraversalEdge{
			{FromID: "s1", ToID: "n1", RelType: "relates_to", Weight: 0.9},
		},
		TotalReachable: 2,
		QueryMs:        1.5,
	}, nil
}

// listDeletedWithEntriesEngine returns a populated deleted list for shape tests.
type listDeletedWithEntriesEngine struct{ fakeEngine }

func (e *listDeletedWithEntriesEngine) ListDeleted(_ context.Context, _ string, _ int) ([]DeletedEngram, error) {
	return []DeletedEngram{
		{
			ID:               "del-1",
			Concept:          "old decision",
			DeletedAt:        time.Now().Add(-1 * time.Hour),
			RecoverableUntil: time.Now().Add(167 * time.Hour),
			Tags:             []string{"arch"},
		},
	}, nil
}

// noPluginsEngine returns a RetryEnrichResult with the Note field populated.
type noPluginsEngine struct{ fakeEngine }

func (e *noPluginsEngine) RetryEnrich(_ context.Context, _ string, id string) (*RetryEnrichResult, error) {
	return &RetryEnrichResult{
		EngramID:        id,
		PluginsQueued:   []string{},
		AlreadyComplete: []string{},
		Note:            "No enrichment plugins are registered",
	}, nil
}

// limitTrackingEngine records the limit value received by ListDeleted.
type limitTrackingEngine struct {
	fakeEngine
	lastLimit int
}

func (e *limitTrackingEngine) ListDeleted(_ context.Context, _ string, limit int) ([]DeletedEngram, error) {
	e.lastLimit = limit
	return []DeletedEngram{}, nil
}

// ── muninn_restore ──────────────────────────────────────────────────────────

func TestHandleRestoreHappyPath(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_restore","arguments":{"vault":"default","id":"abc123"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestHandleRestoreResponseShape(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_restore","arguments":{"vault":"default","id":"abc123"}}}`
	w := postRPC(t, srv, body)
	content := extractInnerJSON(t, decodeResp(t, w.Body.String()))

	for _, field := range []string{"id", "concept", "restored", "state"} {
		if _, ok := content[field]; !ok {
			t.Errorf("response missing field: %q", field)
		}
	}
	if restored, _ := content["restored"].(bool); !restored {
		t.Errorf("restored field should be true, got %v", content["restored"])
	}
}

func TestHandleRestoreMissingID(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_restore","arguments":{"vault":"default"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Errorf("expected -32602, got %v", resp.Error)
	}
}

func TestHandleRestoreEngineError(t *testing.T) {
	srv := newTestServerWith(&restoreErrEngine{})
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_restore","arguments":{"vault":"default","id":"gone"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error == nil || resp.Error.Code != -32000 {
		t.Errorf("expected -32000 for engine error, got %v", resp.Error)
	}
	if !strings.Contains(resp.Error.Message, "recovery window") {
		t.Errorf("error message should mention recovery window, got: %s", resp.Error.Message)
	}
}

// ── muninn_traverse ─────────────────────────────────────────────────────────

func TestHandleTraverseHappyPath(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_traverse","arguments":{"vault":"default","start_id":"node1"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestHandleTraverseResponseShape(t *testing.T) {
	srv := newTestServerWith(&traverseWithNodesEngine{})
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_traverse","arguments":{"vault":"default","start_id":"s1"}}}`
	w := postRPC(t, srv, body)
	content := extractInnerJSON(t, decodeResp(t, w.Body.String()))

	for _, field := range []string{"nodes", "edges", "total_reachable", "query_ms"} {
		if _, ok := content[field]; !ok {
			t.Errorf("response missing field: %q", field)
		}
	}

	nodes, ok := content["nodes"].([]any)
	if !ok || len(nodes) == 0 {
		t.Fatal("expected non-empty nodes array")
	}
	node, ok := nodes[0].(map[string]any)
	if !ok {
		t.Fatal("nodes[0] is not an object")
	}
	if _, ok := node["hop_dist"]; !ok {
		t.Error("nodes[0] missing required field 'hop_dist'")
	}
	// Start node must have hop_dist == 0
	if dist, _ := node["hop_dist"].(float64); dist != 0 {
		t.Errorf("start node hop_dist = %v, want 0", dist)
	}
}

func TestHandleTraverseMissingStartID(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_traverse","arguments":{"vault":"default"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Errorf("expected -32602, got %v", resp.Error)
	}
}

func TestHandleTraverseCapsBounds(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_traverse","arguments":{"vault":"default","start_id":"n1","max_hops":99,"max_nodes":9999}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error != nil {
		t.Errorf("expected success after capping bounds, got error: %v", resp.Error)
	}
}

func TestHandleTraverseWithRelTypes(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_traverse","arguments":{"vault":"default","start_id":"n1","rel_types":["depends_on","supports"]}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error != nil {
		t.Errorf("rel_types optional param should be accepted, got error: %v", resp.Error)
	}
}

func TestHandleTraverseEngineError(t *testing.T) {
	srv := newTestServerWith(&traverseErrEngine{})
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_traverse","arguments":{"vault":"default","start_id":"bad"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error == nil || resp.Error.Code != -32000 {
		t.Errorf("expected -32000 for engine error, got %v", resp.Error)
	}
}

// ── muninn_explain ──────────────────────────────────────────────────────────

func TestHandleExplainHappyPath(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_explain","arguments":{"vault":"default","engram_id":"e1","query":["JWT","auth"]}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestHandleExplainResponseShape(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_explain","arguments":{"vault":"default","engram_id":"e1","query":["JWT"]}}}`
	w := postRPC(t, srv, body)
	content := extractInnerJSON(t, decodeResp(t, w.Body.String()))

	// Top-level required fields
	for _, field := range []string{"engram_id", "final_score", "components", "fts_matches", "assoc_path", "would_return", "threshold"} {
		if _, ok := content[field]; !ok {
			t.Errorf("response missing field: %q", field)
		}
	}

	// All 6 component sub-fields must be present
	components, ok := content["components"].(map[string]any)
	if !ok {
		t.Fatal("components field is not an object")
	}
	for _, comp := range []string{
		"full_text_relevance", "semantic_similarity", "decay_factor",
		"hebbian_boost", "access_frequency", "confidence",
	} {
		if _, ok := components[comp]; !ok {
			t.Errorf("components missing field: %q", comp)
		}
	}

	// would_return and threshold must be present and typed correctly
	if _, ok := content["would_return"].(bool); !ok {
		t.Error("would_return should be a bool")
	}
	if _, ok := content["threshold"].(float64); !ok {
		t.Error("threshold should be a number")
	}
}

func TestHandleExplainMissingEngramID(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_explain","arguments":{"vault":"default","query":["test"]}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Errorf("expected -32602, got %v", resp.Error)
	}
}

func TestHandleExplainEmptyQuery(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_explain","arguments":{"vault":"default","engram_id":"e1","query":[]}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Errorf("expected -32602 for empty query, got %v", resp.Error)
	}
}

func TestHandleExplainMissingQuery(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_explain","arguments":{"vault":"default","engram_id":"e1"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Errorf("expected -32602 for missing query, got %v", resp.Error)
	}
}

func TestHandleExplainEngineError(t *testing.T) {
	srv := newTestServerWith(&explainErrEngine{})
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_explain","arguments":{"vault":"default","engram_id":"gone","query":["x"]}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error == nil || resp.Error.Code != -32000 {
		t.Errorf("expected -32000 for engine error, got %v", resp.Error)
	}
}

// ── muninn_state ─────────────────────────────────────────────────────────────

func TestHandleStateHappyPath(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_state","arguments":{"vault":"default","id":"e1","state":"active"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestHandleStateResponseShape(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_state","arguments":{"vault":"default","id":"e1","state":"completed"}}}`
	w := postRPC(t, srv, body)
	content := extractInnerJSON(t, decodeResp(t, w.Body.String()))

	for _, field := range []string{"id", "state", "updated"} {
		if _, ok := content[field]; !ok {
			t.Errorf("response missing field: %q", field)
		}
	}
	if updated, _ := content["updated"].(bool); !updated {
		t.Errorf("updated field should be true, got %v", content["updated"])
	}
	if content["id"] != "e1" {
		t.Errorf("id = %v, want e1", content["id"])
	}
	if content["state"] != "completed" {
		t.Errorf("state = %v, want completed", content["state"])
	}
}

func TestHandleStateInvalidState(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_state","arguments":{"vault":"default","id":"e1","state":"limbo"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Errorf("expected -32602 for invalid state, got %v", resp.Error)
	}
}

func TestHandleStateAllValidStates(t *testing.T) {
	srv := newTestServer()
	states := []string{"planning", "active", "paused", "blocked", "completed", "cancelled", "archived"}
	for _, state := range states {
		t.Run(state, func(t *testing.T) {
			body := fmt.Sprintf(`{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_state","arguments":{"vault":"default","id":"e1","state":%q}}}`, state)
			w := postRPC(t, srv, body)
			resp := decodeResp(t, w.Body.String())
			if resp.Error != nil {
				t.Errorf("state %q: unexpected error: %v", state, resp.Error)
			}
		})
	}
}

func TestHandleStateWithOptionalReason(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_state","arguments":{"vault":"default","id":"e1","state":"paused","reason":"waiting for design review"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error != nil {
		t.Errorf("optional reason should be accepted, got error: %v", resp.Error)
	}
}

func TestHandleStateMissingID(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_state","arguments":{"vault":"default","state":"active"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Errorf("expected -32602, got %v", resp.Error)
	}
}

func TestHandleStateMissingState(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_state","arguments":{"vault":"default","id":"e1"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Errorf("expected -32602, got %v", resp.Error)
	}
}

func TestHandleStateEngineError(t *testing.T) {
	// Engine returns error for invalid transitions (e.g. archived → planning).
	srv := newTestServerWith(&stateErrEngine{})
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_state","arguments":{"vault":"default","id":"e1","state":"planning"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error == nil || resp.Error.Code != -32000 {
		t.Errorf("expected -32000 for engine transition error, got %v", resp.Error)
	}
}

// ── muninn_list_deleted ──────────────────────────────────────────────────────

func TestHandleListDeletedHappyPath(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_list_deleted","arguments":{"vault":"default"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestHandleListDeletedResponseShape(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_list_deleted","arguments":{"vault":"default"}}}`
	w := postRPC(t, srv, body)
	content := extractInnerJSON(t, decodeResp(t, w.Body.String()))

	if _, ok := content["deleted"]; !ok {
		t.Error("response missing field: \"deleted\"")
	}
	if _, ok := content["count"]; !ok {
		t.Error("response missing field: \"count\"")
	}
	// Empty result: count must equal len(deleted)
	deleted, _ := content["deleted"].([]any)
	count, _ := content["count"].(float64)
	if int(count) != len(deleted) {
		t.Errorf("count=%d does not match len(deleted)=%d", int(count), len(deleted))
	}
}

func TestHandleListDeletedEntryHasRecoverableUntil(t *testing.T) {
	srv := newTestServerWith(&listDeletedWithEntriesEngine{})
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_list_deleted","arguments":{"vault":"default"}}}`
	w := postRPC(t, srv, body)
	content := extractInnerJSON(t, decodeResp(t, w.Body.String()))

	deleted, ok := content["deleted"].([]any)
	if !ok || len(deleted) == 0 {
		t.Fatal("expected non-empty deleted list")
	}
	entry, ok := deleted[0].(map[string]any)
	if !ok {
		t.Fatal("deleted[0] is not an object")
	}
	for _, field := range []string{"id", "concept", "deleted_at", "recoverable_until"} {
		if _, ok := entry[field]; !ok {
			t.Errorf("deleted entry missing field: %q", field)
		}
	}
}

func TestHandleListDeletedEmptyIsNotError(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_list_deleted","arguments":{"vault":"default"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error != nil {
		t.Errorf("empty vault should return success, not error: %v", resp.Error)
	}
}

func TestHandleListDeletedLimitCap(t *testing.T) {
	eng := &limitTrackingEngine{}
	srv := newTestServerWith(eng)
	// Request limit=200; handler must cap to 100 before calling engine.
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_list_deleted","arguments":{"vault":"default","limit":200}}}`
	postRPC(t, srv, body)
	if eng.lastLimit != 100 {
		t.Errorf("expected engine to receive limit=100 after capping, got %d", eng.lastLimit)
	}
}

func TestHandleListDeletedEngineError(t *testing.T) {
	srv := newTestServerWith(&listDeletedErrEngine{})
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_list_deleted","arguments":{"vault":"default"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error == nil || resp.Error.Code != -32000 {
		t.Errorf("expected -32000 for engine error, got %v", resp.Error)
	}
}

// ── muninn_retry_enrich ──────────────────────────────────────────────────────

func TestHandleRetryEnrichHappyPath(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_retry_enrich","arguments":{"vault":"default","id":"e1"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestHandleRetryEnrichResponseShape(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_retry_enrich","arguments":{"vault":"default","id":"e1"}}}`
	w := postRPC(t, srv, body)
	content := extractInnerJSON(t, decodeResp(t, w.Body.String()))

	for _, field := range []string{"engram_id", "plugins_queued", "already_complete"} {
		if _, ok := content[field]; !ok {
			t.Errorf("response missing field: %q", field)
		}
	}
	if _, ok := content["plugins_queued"].([]any); !ok {
		t.Error("plugins_queued should be an array")
	}
	if _, ok := content["already_complete"].([]any); !ok {
		t.Error("already_complete should be an array")
	}
}

func TestHandleRetryEnrichNoPluginsNote(t *testing.T) {
	// AC: "degrades gracefully when no enrich plugin registered (empty queued list + note field)"
	srv := newTestServerWith(&noPluginsEngine{})
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_retry_enrich","arguments":{"vault":"default","id":"e1"}}}`
	w := postRPC(t, srv, body)
	content := extractInnerJSON(t, decodeResp(t, w.Body.String()))

	queued, _ := content["plugins_queued"].([]any)
	if len(queued) != 0 {
		t.Errorf("expected empty plugins_queued when no plugins, got %v", queued)
	}
	note, ok := content["note"].(string)
	if !ok || note == "" {
		t.Error("expected non-empty note field when no plugins registered")
	}
}

func TestHandleRetryEnrichMissingID(t *testing.T) {
	srv := newTestServer()
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_retry_enrich","arguments":{"vault":"default"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Errorf("expected -32602, got %v", resp.Error)
	}
}

func TestHandleRetryEnrichEngineError(t *testing.T) {
	srv := newTestServerWith(&retryEnrichErrEngine{})
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_retry_enrich","arguments":{"vault":"default","id":"gone"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error == nil || resp.Error.Code != -32000 {
		t.Errorf("expected -32000 for engine error, got %v", resp.Error)
	}
}

// ── relTypeFromString ─────────────────────────────────────────────────────────

func TestRelTypeFromString_AllTypes(t *testing.T) {
	cases := []struct {
		input string
		want  uint16
	}{
		{"supports", uint16(storage.RelSupports)},
		{"contradicts", uint16(storage.RelContradicts)},
		{"depends_on", uint16(storage.RelDependsOn)},
		{"supersedes", uint16(storage.RelSupersedes)},
		{"relates_to", uint16(storage.RelRelatesTo)},
		{"is_part_of", uint16(storage.RelIsPartOf)},
		{"causes", uint16(storage.RelCauses)},
		{"preceded_by", uint16(storage.RelPrecededBy)},
		{"followed_by", uint16(storage.RelFollowedBy)},
		{"created_by_person", uint16(storage.RelCreatedByPerson)},
		{"belongs_to_project", uint16(storage.RelBelongsToProject)},
		{"references", uint16(storage.RelReferences)},
		{"implements", uint16(storage.RelImplements)},
		{"blocks", uint16(storage.RelBlocks)},
		{"resolves", uint16(storage.RelResolves)},
		{"refines", uint16(storage.RelRefines)},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := relTypeFromString(tc.input)
			if got != tc.want {
				t.Errorf("relTypeFromString(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestRelTypeFromString_UnknownDefaultsToRelatesTo(t *testing.T) {
	got := relTypeFromString("foobar")
	want := uint16(storage.RelRelatesTo)
	if got != want {
		t.Errorf("relTypeFromString(%q) = %d, want %d (RelRelatesTo)", "foobar", got, want)
	}
}

func TestRelTypeFromString_EmptyDefaultsToRelatesTo(t *testing.T) {
	got := relTypeFromString("")
	want := uint16(storage.RelRelatesTo)
	if got != want {
		t.Errorf("relTypeFromString(%q) = %d, want %d (RelRelatesTo)", "", got, want)
	}
}

// ── handleRecall profile wiring ───────────────────────────────────────────────

// profileCapturingEngine records the Profile field from the last ActivateRequest.
type profileCapturingEngine struct {
	fakeEngine
	lastProfile string
}

func (e *profileCapturingEngine) Activate(_ context.Context, req *mbp.ActivateRequest) (*mbp.ActivateResponse, error) {
	e.lastProfile = req.Profile
	return &mbp.ActivateResponse{}, nil
}

func TestHandleRecallProfileParamWired(t *testing.T) {
	eng := &profileCapturingEngine{}
	srv := newTestServerWith(eng)
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_recall","arguments":{"vault":"default","context":["auth"],"profile":"causal"}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if eng.lastProfile != "causal" {
		t.Errorf("profile = %q, want %q", eng.lastProfile, "causal")
	}
}

func TestHandleRecallProfileOmittedIsEmpty(t *testing.T) {
	eng := &profileCapturingEngine{}
	srv := newTestServerWith(eng)
	body := `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"muninn_recall","arguments":{"vault":"default","context":["auth"]}}}`
	w := postRPC(t, srv, body)
	resp := decodeResp(t, w.Body.String())
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if eng.lastProfile != "" {
		t.Errorf("profile = %q, want empty string when not provided", eng.lastProfile)
	}
}
