package db

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Migration struct {
	Version int
	Name    string
	SQL     string
}

func RunMigrations(ctx context.Context) error {
	if Pool == nil {
		return fmt.Errorf("database not connected")
	}

	// Create migrations tracking table
	_, err := Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INT PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// Read migration files
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var migrations []Migration
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		var version int
		var name string
		if _, err := fmt.Sscanf(entry.Name(), "%d_%s", &version, &name); err != nil {
			// Try simpler format
			parts := strings.SplitN(entry.Name(), "_", 2)
			if len(parts) < 1 {
				continue
			}
			fmt.Sscanf(parts[0], "%d", &version)
			name = strings.TrimSuffix(parts[len(parts)-1], ".sql")
		}
		name = strings.TrimSuffix(name, ".sql")

		data, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		migrations = append(migrations, Migration{
			Version: version,
			Name:    name,
			SQL:     string(data),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	// Apply pending migrations
	applied := 0
	for _, m := range migrations {
		var count int
		err := Pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = $1", m.Version).Scan(&count)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", m.Version, err)
		}
		if count > 0 {
			continue
		}

		slog.Info("applying migration", "version", m.Version, "name", m.Name)
		_, err = Pool.Exec(ctx, m.SQL)
		if err != nil {
			return fmt.Errorf("apply migration %d (%s): %w", m.Version, m.Name, err)
		}

		_, err = Pool.Exec(ctx, "INSERT INTO schema_migrations (version, name) VALUES ($1, $2)",
			m.Version, m.Name)
		if err != nil {
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}
		applied++
	}

	slog.Info("migrations complete", "applied", applied, "total", len(migrations))
	return nil
}
