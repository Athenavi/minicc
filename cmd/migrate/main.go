package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/db"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]

	// Read connection config from env (same vars as main server)
	dsn := getEnv("POSTGRES_DSN", "postgres://minicc:minicc@localhost:5432/minicc?sslmode=disable")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch cmd {
	case "up":
		runUp(ctx, dsn)
	case "down":
		steps := 1
		if len(os.Args) > 2 {
			fmt.Sscanf(os.Args[2], "%d", &steps)
		}
		runDown(ctx, dsn, steps)
	case "status":
		runStatus(ctx, dsn)
	case "create":
		if len(os.Args) < 3 {
			fmt.Println("Usage: minicc-migrate create <name>")
			os.Exit(1)
		}
		runCreate(os.Args[2])
	case "ensure-db":
		runEnsureDB(ctx, dsn)
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`MiniCC Database Migration CLI

Usage:
  minicc-migrate up                    Apply all pending migrations
  minicc-migrate down [n]              Roll back n migrations (default 1)
  minicc-migrate status                Show migration status
  minicc-migrate create <name>         Create a new migration pair
  minicc-migrate ensure-db             Create the database if it doesn't exist

Environment:
  POSTGRES_DSN   Database connection string (default: postgres://minicc:minicc@localhost:5432/minicc?sslmode=disable)
`)
}

func runUp(ctx context.Context, dsn string) {
	slog.Info("connecting to database")
	if err := db.ConnectPostgres(ctx, dsn, 2, 1); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	defer db.ClosePostgres()

	slog.Info("applying migrations", "dir", "migrations")
	if err := db.RunAtlasMigrations(ctx, db.Pool, "migrations"); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	slog.Info("migrations applied successfully")
}

func runDown(ctx context.Context, dsn string, steps int) {
	if steps <= 0 {
		fmt.Println("Nothing to roll back")
		return
	}

	slog.Info("connecting to database")
	if err := db.ConnectPostgres(ctx, dsn, 2, 1); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	defer db.ClosePostgres()

	slog.Info("rolling back migrations", "steps", steps)
	if err := db.DownAtlasMigration(ctx, db.Pool, "migrations", steps); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	slog.Info("rollback complete")
}

func runStatus(ctx context.Context, dsn string) {
	if err := db.ConnectPostgres(ctx, dsn, 2, 1); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	defer db.ClosePostgres()

	rows, err := db.Pool.Query(ctx,
		"SELECT version, name, checksum, applied_at FROM schema_migrations ORDER BY version DESC")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: query migrations: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	fmt.Println("Applied migrations:")
	count := 0
	for rows.Next() {
		var version int64
		var name, checksum string
		var appliedAt time.Time
		if err := rows.Scan(&version, &name, &checksum, &appliedAt); err != nil {
			continue
		}
		fmt.Printf("  %d  %s  %s  %s\n", version, name, appliedAt.Format(time.RFC3339), checksum)
		count++
	}
	if count == 0 {
		fmt.Println("  (none)")
	}
	fmt.Printf("\nTotal: %d migrations applied\n", count)
}

func runCreate(name string) {
	name = strings.ReplaceAll(strings.ToLower(name), " ", "_")
	timestamp := time.Now().Format("20060102150405")

	upFile := fmt.Sprintf("migrations/%s_%s.up.sql", timestamp, name)
	downFile := fmt.Sprintf("migrations/%s_%s.down.sql", timestamp, name)

	upContent := fmt.Sprintf("-- %s: up\n-- Write your migration SQL here.\n\n", name)
	downContent := fmt.Sprintf("-- %s: down\n-- Write your rollback SQL here.\n\n", name)

	if err := os.WriteFile(upFile, []byte(upContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: create %s: %v\n", upFile, err)
		os.Exit(1)
	}
	fmt.Printf("Created: %s\n", upFile)

	if err := os.WriteFile(downFile, []byte(downContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: create %s: %v\n", downFile, err)
		os.Exit(1)
	}
	fmt.Printf("Created: %s\n", downFile)
}

func runEnsureDB(ctx context.Context, dsn string) {
	slog.Info("ensuring database exists")
	if err := db.EnsureDatabase(ctx, dsn); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	slog.Info("database ready")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
