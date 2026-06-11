package main

import (
	"fmt"
	"os"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
)

// migrate applies pending SQL migrations from the schema/ directory.
// Migrations are numbered SQL files (001_initial.sql, 002_belief_meta.sql, ...).
// Applied migrations are tracked in a schema_migrations table.
// Phase 1 stub: connects to the database but does not apply migrations.
// Full migration runner lands in Phase 2 alongside the memory schema.

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		dsn = "sqlite://./agent.db"
	}

	store, err := platform.NewStore(dsn)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer store.Close()

	fmt.Printf("migrate: connected to %s database\n", store.Driver)
	fmt.Println("migrate: Phase 1 stub — full migration runner lands in Phase 2")
	fmt.Println("migrate: schema/ directory will contain numbered SQL migration files")

	return nil
}
