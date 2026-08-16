package db

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AtlasMigration struct {
	Version   string
	Name      string
	UpSQL     string
	DownSQL   string
	Checksum  string // sha256:hex
}

// RunAtlasMigrations reads Atlas-format migrations from dir and applies pending ones.
func RunAtlasMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	// Read migration files
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir %s: %w", dir, err)
	}

	// Group by version prefix (e.g. "202607020001")
	type filePair struct {
		up, down   string
		upContent  string
		downContent string
		checksum   string
	}
	groups := make(map[string]*filePair)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		// Parse: 202607020001_initial.up.sql
		parts := strings.SplitN(e.Name(), "_", 2)
		if len(parts) < 2 {
			continue
		}
		version := parts[0]
		remainder := parts[1] // "initial.up.sql" or "initial.down.sql"

		if _, ok := groups[version]; !ok {
			groups[version] = &filePair{}
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", e.Name(), err)
		}

		if strings.HasSuffix(remainder, ".up.sql") {
			groups[version].up = e.Name()
			groups[version].upContent = string(data)
		} else if strings.HasSuffix(remainder, ".down.sql") {
			groups[version].down = e.Name()
			groups[version].downContent = string(data)
		}
	}

	// Compute checksums and verify against atlas.sum
	sumFile := filepath.Join(dir, "atlas.sum")
	sumMap := make(map[string]string)
	if sumData, err := os.ReadFile(sumFile); err == nil {
		for _, line := range strings.Split(string(sumData), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Format: "h1:base64hash path/to/file.sql"
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 {
				sumMap[strings.TrimSpace(parts[1])] = strings.TrimSpace(parts[0])
			}
		}
	}

	// Sort versions
	var versions []string
	for v := range groups {
		versions = append(versions, v)
	}
	sort.Strings(versions)

	// Create tracking table
	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			checksum VARCHAR(128),
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// Apply pending
	applied := 0
	for _, v := range versions {
		pair := groups[v]
		if pair.up == "" {
			continue
		}

		var count int
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = $1", parseVersion(v)).Scan(&count); err != nil {
			return fmt.Errorf("check migration %s: %w", v, err)
		}
		if count > 0 {
			continue
		}

		// Verify checksum
		expectedSum := sumMap[pair.up]
		if expectedSum != "" {
			actual := computeChecksum(pair.upContent)
			if actual != expectedSum {
				return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", pair.up, expectedSum, actual)
			}
		}

		slog.Info("applying migration", "version", v, "file", pair.up)

		// Run in transaction
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", pair.up, err)
		}

		_, err = tx.Exec(ctx, pair.upContent)
		if err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", pair.up, err)
		}

		_, err = tx.Exec(ctx, "INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)",
			parseVersion(v), pair.up, expectedSum)
		if err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("record %s: %w", pair.up, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s: %w", pair.up, err)
		}
		applied++
	}

	slog.Info("migrations complete", "applied", applied, "total", len(versions))
	return nil
}

// DownAtlasMigration rolls back the last N migrations.
func DownAtlasMigration(ctx context.Context, pool *pgxpool.Pool, dir string, steps int) error {
	// Read applied migrations (most recent first)
	rows, err := pool.Query(ctx, "SELECT version, name FROM schema_migrations ORDER BY version DESC LIMIT $1", steps)
	if err != nil {
		return fmt.Errorf("query applied migrations: %w", err)
	}
	defer rows.Close()

	type applied struct{ version int64; name string }
	var appliedList []applied
	for rows.Next() {
				var a applied
		if err := rows.Scan(&a.version, &a.name); err != nil {
			return fmt.Errorf("scan applied migration: %w", err)
		}
		appliedList = append(appliedList, a)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate migrations: %w", err)
	}

	for _, a := range appliedList {
		// Find corresponding .down.sql
		downFile := filepath.Join(dir, strings.Replace(a.name, ".up.sql", ".down.sql", 1))
		data, err := os.ReadFile(downFile)
		if err != nil {
			return fmt.Errorf("read down migration for version %d: %w", a.version, err)
		}

		slog.Info("rolling back", "version", a.version, "file", a.name)

		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, string(data)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("down migration %d: %w", a.version, err)
		}

		if _, err := tx.Exec(ctx, "DELETE FROM schema_migrations WHERE version = $1", a.version); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("delete record %d: %w", a.version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit rollback %d: %w", a.version, err)
		}
	}

	return nil
}

// EnsureDatabase creates the database if it doesn't exist.
func EnsureDatabase(ctx context.Context, dsn string) error {
	// Parse DSN as URL: postgres://user:pass@host:port/dbname?sslmode=...
	// Connect to the 'postgres' default database and CREATE DATABASE if needed
	u, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("parse dsn: %w", err)
	}

	// Extract target DB name from the path (last segment after /)
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		return nil // no target database, skip
	}

	// Build admin DSN connecting to 'postgres' database instead
	u.Path = "/postgres"
	adminDSN := u.String()

	adminPool, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		return fmt.Errorf("connect to postgres admin: %w", err)
	}
	defer adminPool.Close()

	var exists int
	adminPool.QueryRow(ctx, "SELECT 1 FROM pg_database WHERE datname = $1", dbName).Scan(&exists)
	if exists == 0 {
		slog.Info("creating database", "name", dbName)
		_, err := adminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", pqQuoteIdentifier(dbName)))
		if err != nil {
			return fmt.Errorf("create database %s: %w", dbName, err)
		}
		slog.Info("database created", "name", dbName)
	}

	return nil
}

func parseVersion(v string) int64 {
	var ver int64
	fmt.Sscanf(v, "%d", &ver)
	return ver
}

func computeChecksum(content string) string {
	h := sha256.Sum256([]byte(content))
	return "h1:" + base64.StdEncoding.EncodeToString(h[:])
}

func pqQuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
