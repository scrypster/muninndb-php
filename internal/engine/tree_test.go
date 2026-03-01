package engine

import (
	"context"
	"testing"
)

func TestRememberAndRecallTree(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()
	vault := "tree-test"

	req := &RememberTreeRequest{
		Vault: vault,
		Root: TreeNodeInput{
			Concept: "Project Alpha",
			Content: "Top-level project plan",
			Type:    "goal",
			Children: []TreeNodeInput{
				{
					Concept: "Phase 1",
					Content: "First phase",
					Type:    "goal",
					Children: []TreeNodeInput{
						{Concept: "Task 1.1", Content: "First task", Type: "task"},
						{Concept: "Task 1.2", Content: "Second task", Type: "task"},
					},
				},
				{
					Concept: "Phase 2",
					Content: "Second phase",
					Type:    "goal",
				},
			},
		},
	}

	result, err := eng.RememberTree(ctx, req)
	if err != nil {
		t.Fatalf("RememberTree: %v", err)
	}
	if result.RootID == "" {
		t.Fatal("expected non-empty root ID")
	}
	if len(result.NodeMap) != 5 { // root + 2 phases + 2 tasks
		t.Fatalf("NodeMap: got %d entries, want 5", len(result.NodeMap))
	}

	tree, err := eng.RecallTree(ctx, vault, result.RootID, 10)
	if err != nil {
		t.Fatalf("RecallTree: %v", err)
	}
	if tree.Concept != "Project Alpha" {
		t.Errorf("root concept: got %q, want %q", tree.Concept, "Project Alpha")
	}
	if len(tree.Children) != 2 {
		t.Fatalf("root children: got %d, want 2", len(tree.Children))
	}
	if tree.Children[0].Concept != "Phase 1" {
		t.Errorf("first child: got %q, want Phase 1", tree.Children[0].Concept)
	}
	if len(tree.Children[0].Children) != 2 {
		t.Fatalf("phase 1 tasks: got %d, want 2", len(tree.Children[0].Children))
	}
	if tree.Children[0].Children[0].Concept != "Task 1.1" {
		t.Errorf("first task: got %q, want Task 1.1", tree.Children[0].Children[0].Concept)
	}
	if tree.Children[0].Children[1].Concept != "Task 1.2" {
		t.Errorf("second task: got %q, want Task 1.2", tree.Children[0].Children[1].Concept)
	}
	// Phase 2 must come after Phase 1 (ordinal order)
	if tree.Children[1].Concept != "Phase 2" {
		t.Errorf("second child: got %q, want Phase 2", tree.Children[1].Concept)
	}
}

func TestRecallTree_LeafNode(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()
	vault := "leaf-tree"

	result, err := eng.RememberTree(ctx, &RememberTreeRequest{
		Vault: vault,
		Root:  TreeNodeInput{Concept: "Solo leaf", Content: "I am alone", Type: "task"},
	})
	if err != nil {
		t.Fatal(err)
	}

	tree, err := eng.RecallTree(ctx, vault, result.RootID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Concept != "Solo leaf" {
		t.Errorf("concept: got %q", tree.Concept)
	}
	if len(tree.Children) != 0 {
		t.Errorf("expected no children, got %d", len(tree.Children))
	}
}
