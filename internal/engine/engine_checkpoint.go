package engine

// Checkpoint creates a Pebble checkpoint (hardlinked point-in-time snapshot) at destDir.
// Safe to call on a live database — Pebble guarantees consistency.
func (e *Engine) Checkpoint(destDir string) error {
	return e.store.GetDB().Checkpoint(destDir)
}
