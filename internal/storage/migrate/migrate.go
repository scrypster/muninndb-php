package migrate

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"sort"

	"github.com/cockroachdb/pebble"
)

// migrationVersionKey is the Pebble key that stores the last successfully
// applied migration version. Distinct from the schema version key used by
// replication.CheckAndSetSchemaVersion.
var migrationVersionKey = []byte{0xFF, 'm', 'i', 'g', '_', 'v', 'e', 'r'}

// Migration represents a versioned schema migration.
type Migration struct {
	Version     int
	Description string
	Up          func(db *pebble.DB) error
}

// Runner executes registered migrations in version order against a Pebble DB.
type Runner struct {
	migrations []Migration
	db         *pebble.DB
}

// NewRunner creates a migration runner for the given Pebble database.
func NewRunner(db *pebble.DB) *Runner {
	return &Runner{db: db}
}

// Register adds a migration to the runner. Migrations are sorted by version
// before execution, so registration order does not matter.
func (r *Runner) Register(m Migration) {
	r.migrations = append(r.migrations, m)
}

// Run executes all registered migrations whose version exceeds the currently
// stored migration version, in ascending version order. Each successful
// migration durably updates the stored version before proceeding to the next.
// Returns the number of applied migrations and the first error encountered.
func (r *Runner) Run() (applied int, err error) {
	if len(r.migrations) == 0 {
		return 0, nil
	}

	sort.Slice(r.migrations, func(i, j int) bool {
		return r.migrations[i].Version < r.migrations[j].Version
	})

	current, err := readMigrationVersion(r.db)
	if err != nil {
		return 0, fmt.Errorf("migrate: read version: %w", err)
	}

	for _, m := range r.migrations {
		if m.Version <= current {
			continue
		}
		slog.Info("applying migration", "version", m.Version, "description", m.Description)
		if err := m.Up(r.db); err != nil {
			return applied, fmt.Errorf("migrate: version %d (%s): %w", m.Version, m.Description, err)
		}
		if err := writeMigrationVersion(r.db, m.Version); err != nil {
			return applied, fmt.Errorf("migrate: persist version %d: %w", m.Version, err)
		}
		applied++
	}
	return applied, nil
}

func readMigrationVersion(db *pebble.DB) (int, error) {
	val, closer, err := db.Get(migrationVersionKey)
	if err == pebble.ErrNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer closer.Close()
	if len(val) < 8 {
		return 0, fmt.Errorf("migrate: corrupt version value (len=%d)", len(val))
	}
	return int(binary.BigEndian.Uint64(val)), nil
}

func writeMigrationVersion(db *pebble.DB, v int) error {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(v))
	return db.Set(migrationVersionKey, buf, pebble.Sync)
}
