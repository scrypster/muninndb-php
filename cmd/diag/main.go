package main

import (
	"context"
	"fmt"
	"os"

	"github.com/scrypster/muninndb/internal/index/fts"
	"github.com/scrypster/muninndb/internal/storage"
)

func main() {
	dataPath := "/tmp/muninndb-e2e/pebble"
	if len(os.Args) > 1 {
		dataPath = os.Args[1]
	}

	db, err := storage.OpenPebble(dataPath, storage.DefaultOptions())
	if err != nil {
		fmt.Printf("ERROR opening pebble: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	store := storage.NewPebbleStore(db, storage.PebbleStoreConfig{CacheSize: 1000})
	ws := store.VaultPrefix("test")
	fmt.Printf("vault prefix for 'test': %x\n", ws)

	ctx := context.Background()

	// Check RecentActive
	ids, err := store.RecentActive(ctx, ws, 20)
	fmt.Printf("RecentActive: %d ids, err=%v\n", len(ids), err)
	for i, id := range ids {
		fmt.Printf("  [%d] %s\n", i, id.String())
	}

	// Check FTS search
	ftsIdx := fts.New(db)
	queries := []string{"golang fast compiled", "test", "test content"}
	for _, q := range queries {
		results, err := ftsIdx.Search(ctx, ws, q, 10)
		fmt.Printf("FTS search %q: %d results, err=%v\n", q, len(results), err)
		for i, r := range results {
			fmt.Printf("  [%d] id=%x score=%.4f\n", i, r.ID, r.Score)
		}
	}

	// Try to read the first engram if any
	if len(ids) > 0 {
		eng, err := store.GetEngram(ctx, ws, ids[0])
		if err != nil {
			fmt.Printf("GetEngram error: %v\n", err)
		} else {
			fmt.Printf("First engram: concept=%q content=%q\n", eng.Concept, eng.Content)
		}
	}
}
