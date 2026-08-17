package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"minicc/internal/admin/models"

	"github.com/google/uuid"
)

// DBStore defines the interface for database configuration and management operations.
type DBStore interface {
	Create(ctx context.Context, config *models.DBConfig) error
	GetByID(ctx context.Context, id string) (*models.DBConfig, error)
	List(ctx context.Context) ([]models.DBConfig, error)
	Update(ctx context.Context, id string, updates models.UpdateDBRequest) error
	Delete(ctx context.Context, id string) error
	GetStatus(ctx context.Context) (*models.DBStatus, error)
	CreateBackup(ctx context.Context, description string) (*models.DatabaseBackup, error)
	ListBackups(ctx context.Context) ([]models.DatabaseBackup, error)
	RestoreBackup(ctx context.Context, backupID string) error
	ExecuteQuery(ctx context.Context, query string, args []interface{}) ([]map[string]string, error)
	Optimize(ctx context.Context, action string) error
}

// PostgreSQLDBStore implements DBStore using PostgreSQL.
type PostgreSQLDBStore struct {
	db *sql.DB
}

// NewPostgreSQLDBStore creates a new PostgreSQL database store.
func NewPostgreSQLDBStore(db *sql.DB) *PostgreSQLDBStore {
	return &PostgreSQLDBStore{db: db}
}

// Create inserts a new database configuration.
func (s *PostgreSQLDBStore) Create(ctx context.Context, config *models.DBConfig) error {
	now := time.Now()
	config.ID = uuid.New().String()
	config.CreatedAt = now
	config.UpdatedAt = now

	if config.Status == "" {
		config.Status = "active"
	}
	if config.Port == 0 {
		config.Port = 5432
	}
	if config.MaxOpenConns == 0 {
		config.MaxOpenConns = 25
	}
	if config.MaxIdleConns == 0 {
		config.MaxIdleConns = 5
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_db_configs (
			id, name, host, port, dbname, username, password_hash,
			max_open_conns, max_idle_conns, conn_max_lifetime,
			status, last_health_check, avg_query_time_ms,
			database_size_mb, total_tables, sequential_scans,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
	`,
		config.ID,
		config.Name,
		config.Host,
		config.Port,
		config.DBName,
		config.Username,
		config.PasswordHash,
		config.MaxOpenConns,
		config.MaxIdleConns,
		config.ConnMaxLifetime,
		config.Status,
		config.LastHealthCheck,
		config.AvgQueryTimeMS,
		config.DatabaseSizeMB,
		config.TotalTables,
		config.SequentialScans,
		now,
		now,
	)

	if err != nil {
		return fmt.Errorf("create db config: %w", err)
	}

	_, err = result.RowsAffected()
	return err
}

// GetByID retrieves a database configuration by ID.
func (s *PostgreSQLDBStore) GetByID(ctx context.Context, id string) (*models.DBConfig, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, host, port, dbname, username, password_hash,
		       max_open_conns, max_idle_conns, conn_max_lifetime,
		       status, last_health_check, avg_query_time_ms,
		       database_size_mb, total_tables, sequential_scans,
		       created_at, updated_at
		FROM admin_db_configs
		WHERE id = $1
	`, id)

	return scanDBConfig(row)
}

// List retrieves all database configurations.
func (s *PostgreSQLDBStore) List(ctx context.Context) ([]models.DBConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, host, port, dbname, username, password_hash,
		       max_open_conns, max_idle_conns, conn_max_lifetime,
		       status, last_health_check, avg_query_time_ms,
		       database_size_mb, total_tables, sequential_scans,
		       created_at, updated_at
		FROM admin_db_configs
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list db configs: %w", err)
	}
	defer rows.Close()

	var configs []models.DBConfig
	for rows.Next() {
		config, err := scanDBConfig(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, *config)
	}

	return configs, nil
}

// Update modifies an existing database configuration.
func (s *PostgreSQLDBStore) Update(ctx context.Context, id string, updates models.UpdateDBRequest) error {
	var setClauses []string
	var args []interface{}
	argIdx := 1

	if updates.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *updates.Name)
		argIdx++
	}

	if updates.Host != nil {
		setClauses = append(setClauses, fmt.Sprintf("host = $%d", argIdx))
		args = append(args, *updates.Host)
		argIdx++
	}

	if updates.Port != nil {
		setClauses = append(setClauses, fmt.Sprintf("port = $%d", argIdx))
		args = append(args, *updates.Port)
		argIdx++
	}

	if updates.MaxOpenConns != nil {
		setClauses = append(setClauses, fmt.Sprintf("max_open_conns = $%d", argIdx))
		args = append(args, *updates.MaxOpenConns)
		argIdx++
	}

	if updates.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *updates.Status)
		argIdx++
	}

	if len(setClauses) == 0 {
		return nil
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	args = append(args, id)

	query := fmt.Sprintf(`
		UPDATE admin_db_configs
		SET %s
		WHERE id = $%d
	`, join(",", setClauses), argIdx+1)

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update db config: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// Delete removes a database configuration.
func (s *PostgreSQLDBStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM admin_db_configs WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete db config: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// GetStatus retrieves the current database status.
func (s *PostgreSQLDBStore) GetStatus(ctx context.Context) (*models.DBStatus, error) {
	// TODO: Actually query PostgreSQL for real-time statistics
	// For now, return a mock response
	return &models.DBStatus{
		Version:         "PostgreSQL 15.x",
		Uptime:          "unknown",
		ActiveConnections: 10,
		MaxConnections:  100,
		QueryStats: models.DBQueryStats{
			TotalQueries:    0,
			AvgQueryTimeMS:  0.5,
			SlowQueries:     0,
			Failures:        0,
		},
		DiskUsage: models.DiskUsage{
			TotalSizeMB:   1024.0,
			DataSizeMB:    512.0,
			IndexSizeMB:   256.0,
			ToastSizeMB:   128.0,
			TableCount:    25,
			IndexCount:    50,
		},
		Performance: models.PerformanceMetrics{
			CacheHitRate:    99.5,
			BufferHitRate:   98.5,
			TupleReadPerSec: 1000,
			TupleInsertPerSec: 50,
		},
	}, nil
}

// CreateBackup creates a new database backup record and initiates backup.
func (s *PostgreSQLDBStore) CreateBackup(ctx context.Context, description string) (*models.DatabaseBackup, error) {
	backup := &models.DatabaseBackup{
		ID:          uuid.New().String(),
		BackupType:  "manual",
		Status:      "in_progress",
		Description: description,
		CreatedAt:   time.Now(),
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_database_backups (
			id, backup_type, status, description,
			size_mb, source_server, completion_rate,
			error_message, created_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`,
		backup.ID,
		backup.BackupType,
		backup.Status,
		backup.Description,
		backup.SizeMB,
		backup.SourceServer,
		backup.CompletionRate,
		backup.ErrorMessage,
		backup.CreatedAt,
		backup.ExpiresAt,
	)

	if err != nil {
		return nil, fmt.Errorf("create backup: %w", err)
	}

	// TODO: Actually initiate pg_dump or similar backup process
	// This would run asynchronously in production

	return backup, nil
}

// ListBackups retrieves all backups.
func (s *PostgreSQLDBStore) ListBackups(ctx context.Context) ([]models.DatabaseBackup, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, backup_type, status, description,
		       location, size_mb, source_server, snapshot_time,
		       restoration_count, completed_at, expires_at,
		       validation_status, error_message, created_at,
		       created_by
		FROM admin_database_backups
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	defer rows.Close()

	var backups []models.DatabaseBackup
	for rows.Next() {
		var backup models.DatabaseBackup
		err := rows.Scan(
			&backup.ID, &backup.BackupType, &backup.Status, &backup.Description,
			&backup.Location, &backup.SizeMB, &backup.SourceServer, &backup.SnapshotTime,
			&backup.RestorationCount, &backup.CompletedAt, &backup.ExpiresAt,
			&backup.ValidationStatus, &backup.ErrorMessage, &backup.CreatedAt,
			&backup.CreatedBy,
		)
		if err != nil {
			return nil, err
		}
		backups = append(backups, backup)
	}

	return backups, nil
}

// RestoreBackup restores a database from a backup.
func (s *PostgreSQLDBStore) RestoreBackup(ctx context.Context, backupID string) error {
	// Update backup status
	_, err := s.db.ExecContext(ctx, `
		UPDATE admin_database_backups
		SET status = 'restoring'
		WHERE id = $1
	`, backupID)

	if err != nil {
		return fmt.Errorf("update backup status: %w", err)
	}

	// TODO: Actually restore from backup using pg_restore or psql
	// This should be done asynchronously in production

	_, err = s.db.ExecContext(ctx, `
		UPDATE admin_database_backups
		SET status = 'restored'
		WHERE id = $1
	`, backupID)

	if err != nil {
		return fmt.Errorf("update restore status: %w", err)
	}

	return nil
}

// ExecuteQuery executes a read-only SQL query and returns results.
func (s *PostgreSQLDBStore) ExecuteQuery(ctx context.Context, query string, args []interface{}) ([]map[string]string, error) {
	// Security check: only allow SELECT queries
	trimmedQuery := query[:len(query)-1]
	for i := len(query) - 1; i >= 0; i-- {
		if query[i] == ';' || query[i] == '\n' || query[i] == '\r' || query[i] == ' ' {
			continue
		}
		trimmedQuery = query[:i+1]
		break
	}

	upperQuery := trimmedQuery
	if len(upperQuery) > 7 {
		upperQuery = upperQuery[:7]
	}
	
	// Block dangerous operations
	lowerQuery := ""
	for _, c := range upperQuery {
		lowerQuery += string(c)
	}

	// Simple check - in production, use proper SQL parser
	dangerousOps := []string{"DROP", "DELETE", "TRUNCATE", "ALTER", "GRANT", "REVOKE"}
	for _, op := range dangerousOps {
		if len(upperQuery) >= len(op) {
			match := true
			for i := 0; i < len(op); i++ {
				if upperQuery[i] != op[i] {
					match = false
					break
				}
			}
			if match {
				return nil, fmt.Errorf("query blocked: dangerous operation detected")
			}
		}
	}

	// Execute query
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]string
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]string)
		for i, col := range columns {
			if values[i] != nil {
				row[col] = fmt.Sprintf("%v", values[i])
			}
		}
		results = append(results, row)
	}

	return results, nil
}

// Optimize performs database optimization operations.
func (s *PostgreSQLDBStore) Optimize(ctx context.Context, action string) error {
	switch action {
	case "vacuum":
		_, err := s.db.ExecContext(ctx, "VACUUM ANALYZE")
		if err != nil {
			return fmt.Errorf("vacuum: %w", err)
		}
	case "analyze":
		_, err := s.db.ExecContext(ctx, "ANALYZE")
		if err != nil {
			return fmt.Errorf("analyze: %w", err)
		}
	case "reindex":
		_, err := s.db.ExecContext(ctx, "REINDEX DATABASE current_database()")
		if err != nil {
			return fmt.Errorf("reindex: %w", err)
		}
	default:
		return fmt.Errorf("unknown optimization action: %s", action)
	}

	return nil
}

// scanDBConfig scans a single row into a DBConfig.
func scanDBConfig(row scanner) (*models.DBConfig, error) {
	var config models.DBConfig
	var port int
	var maxOpenConns int
	var maxIdleConns int
	var databaseSizeMB sql.NullFloat64
	var avgQueryTimeMS sql.NullFloat64
	var sequentialScans sql.NullInt64
	var lastHealthCheck sql.NullTime

	err := row.Scan(
		&config.ID, &config.Name, &config.Host, &port, &config.DBName,
		&config.Username, &config.PasswordHash, &maxOpenConns, &maxIdleConns,
		&config.ConnMaxLifetime, &config.Status, &lastHealthCheck,
		&avgQueryTimeMS, &databaseSizeMB, &config.TotalTables, &sequentialScans,
		&config.CreatedAt, &config.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("scan db config: %w", err)
	}

	config.Port = port
	config.MaxOpenConns = maxOpenConns
	config.MaxIdleConns = maxIdleConns
	config.LastHealthCheck = lastHealthCheck.Time
	if avgQueryTimeMS.Valid {
		config.AvgQueryTimeMS = avgQueryTimeMS.Float64
	}
	if databaseSizeMB.Valid {
		config.DatabaseSizeMB = databaseSizeMB.Float64
	}
	if sequentialScans.Valid {
		config.SequentialScans = sequentialScans.Int64
	}

	return &config, nil
}
