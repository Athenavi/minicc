package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"minicc/internal/admin/models"

	"github.com/google/uuid"
)

// APIKeyStore defines the interface for API key persistence operations.
type APIKeyStore interface {
	Create(ctx context.Context, key *models.APIKey) error
	GetByID(ctx context.Context, id string) (*models.APIKey, error)
	GetByHash(ctx context.Context, hash string) (*models.APIKey, error)
	List(ctx context.Context, filter models.ListAPIKeyFilter) ([]models.APIKey, error)
	Total(ctx context.Context, filter models.ListAPIKeyFilter) (int, error)
	Update(ctx context.Context, id string, updates models.UpdateAPIKeyRequest) error
	UpdateStatus(ctx context.Context, id string, status string) error
	Delete(ctx context.Context, id string) error
	IncrementUsage(ctx context.Context, id string, credits int) error
	BulkUpdateStatus(ctx context.Context, ids []string, status string) error
}

// PostgreSQLAPIKeyStore implements APIKeyStore using PostgreSQL.
type PostgreSQLAPIKeyStore struct {
	db *sql.DB
}

// NewPostgreSQLAPIKeyStore creates a new PostgreSQL API key store.
func NewPostgreSQLAPIKeyStore(db *sql.DB) *PostgreSQLAPIKeyStore {
	return &PostgreSQLAPIKeyStore{db: db}
}

// Create inserts a new API key into the database.
func (s *PostgreSQLAPIKeyStore) Create(ctx context.Context, key *models.APIKey) error {
	now := time.Now()
	key.ID = uuid.New().String()
	key.CreatedAt = now
	key.UpdatedAt = now
	
	if key.Status == "" {
		key.Status = "active"
	}
	
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_api_keys (
			id, key_hash, name, tenant_id, user_id,
			monthly_quota, used_count, used_credits,
			status, expires_at, created_at, updated_at,
			created_by, description, allowed_models, rate_limit_qps
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`,
		key.ID,
		key.KeyHash,
		key.Name,
		key.TenantID,
		key.UserID,
		key.MonthlyQuota,
		key.UsedCount,
		key.UsedCredits,
		key.Status,
		key.ExpiresAt,
		now,
		now,
		key.CreatedBy,
		key.Description,
		key.AllowedModels,
		key.RateLimitQPS,
	)
	
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}
	
	_, err = result.RowsAffected()
	return err
}

// GetByID retrieves an API key by its ID.
func (s *PostgreSQLAPIKeyStore) GetByID(ctx context.Context, id string) (*models.APIKey, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, key_hash, name, tenant_id, user_id,
		       monthly_quota, used_count, used_credits,
		       status, expires_at, created_at, updated_at,
		       created_by, description, allowed_models, rate_limit_qps
		FROM admin_api_keys
		WHERE id = $1
	`, id)
	
	return scanAPIKey(row)
}

// GetByHash retrieves an API key by its hash (for authentication).
func (s *PostgreSQLAPIKeyStore) GetByHash(ctx context.Context, hash string) (*models.APIKey, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, key_hash, name, tenant_id, user_id,
		       monthly_quota, used_count, used_credits,
		       status, expires_at, created_at, updated_at,
		       created_by, description, allowed_models, rate_limit_qps
		FROM admin_api_keys
		WHERE key_hash = $1
	`, hash)
	
	return scanAPIKey(row)
}

// List retrieves a paginated list of API keys with optional filters.
func (s *PostgreSQLAPIKeyStore) List(ctx context.Context, filter models.ListAPIKeyFilter) ([]models.APIKey, error) {
	whereClause := "WHERE 1=1"
	var args []interface{}
	argIdx := 1
	
	if filter.Status != nil {
		whereClause += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, *filter.Status)
		argIdx++
	}
	
	if filter.TenantID != nil {
		whereClause += fmt.Sprintf(" AND tenant_id = $%d", argIdx)
		args = append(args, *filter.TenantID)
		argIdx++
	}
	
	if filter.ExpiresBefore != nil {
		whereClause += fmt.Sprintf(" AND expires_at < $%d", argIdx)
		args = append(args, *filter.ExpiresBefore)
		argIdx++
	}
	
	if filter.Search != nil {
		whereClause += fmt.Sprintf(" AND name ILIKE $%d", argIdx)
		args = append(args, "%"+*filter.Search+"%")
		argIdx++
	}
	
	query := fmt.Sprintf(`
		SELECT id, key_hash, name, tenant_id, user_id,
		       monthly_quota, used_count, used_credits,
		       status, expires_at, created_at, updated_at,
		       created_by, description, allowed_models, rate_limit_qps
		FROM admin_api_keys
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)
	
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()
	
	var keys []models.APIKey
	for rows.Next() {
		key, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *key)
	}
	
	return keys, nil
}

// Total returns the total count of API keys matching the filter.
func (s *PostgreSQLAPIKeyStore) Total(ctx context.Context, filter models.ListAPIKeyFilter) (int, error) {
	whereClause := "WHERE 1=1"
	var args []interface{}
	argIdx := 1
	
	if filter.Status != nil {
		whereClause += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, *filter.Status)
		argIdx++
	}
	
	if filter.TenantID != nil {
		whereClause += fmt.Sprintf(" AND tenant_id = $%d", argIdx)
		args = append(args, *filter.TenantID)
		argIdx++
	}
	
	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM admin_api_keys
		%s
	`, whereClause)
	
	var total int
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&total)
	if err != nil {
		return 0, err
	}
	
	return total, nil
}

// Update modifies an existing API key.
func (s *PostgreSQLAPIKeyStore) Update(ctx context.Context, id string, updates models.UpdateAPIKeyRequest) error {
	var setClauses []string
	var args []interface{}
	argIdx := 1
	
	if updates.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *updates.Name)
		argIdx++
	}
	
	if updates.MonthlyQuota != nil {
		setClauses = append(setClauses, fmt.Sprintf("monthly_quota = $%d", argIdx))
		args = append(args, *updates.MonthlyQuota)
		argIdx++
	}
	
	if updates.ExpiresAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("expires_at = $%d", argIdx))
		args = append(args, *updates.ExpiresAt)
		argIdx++
	}
	
	if updates.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *updates.Status)
		argIdx++
	}
	
	if updates.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *updates.Description)
		argIdx++
	}
	
	if updates.RateLimitQPS != nil {
		setClauses = append(setClauses, fmt.Sprintf("rate_limit_qps = $%d", argIdx))
		args = append(args, *updates.RateLimitQPS)
		argIdx++
	}
	
	if len(setClauses) == 0 {
		return nil
	}
	
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	args = append(args, id)
	
	query := fmt.Sprintf(`
		UPDATE admin_api_keys
		SET %s
		WHERE id = $%d
	`, join(", ", setClauses), argIdx+1)
	
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update api key: %w", err)
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return fmt.Errorf("api key not found: %s", id)
	}
	
	return nil
}

// UpdateStatus changes the status of an API key.
func (s *PostgreSQLAPIKeyStore) UpdateStatus(ctx context.Context, id string, status string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE admin_api_keys
		SET status = $1, updated_at = NOW()
		WHERE id = $2
	`, status, id)
	
	if err != nil {
		return fmt.Errorf("update api key status: %w", err)
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return fmt.Errorf("api key not found: %s", id)
	}
	
	return nil
}

// Delete removes an API key from the database.
func (s *PostgreSQLAPIKeyStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM admin_api_keys
		WHERE id = $1
	`, id)
	
	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return fmt.Errorf("api key not found: %s", id)
	}
	
	return nil
}

// IncrementUsage increases the usage counter.
func (s *PostgreSQLAPIKeyStore) IncrementUsage(ctx context.Context, id string, credits int) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE admin_api_keys
		SET used_count = used_count + 1,
		    used_credits = used_credits + $1,
		    updated_at = NOW()
		WHERE id = $2
	`, credits, id)
	
	if err != nil {
		return fmt.Errorf("increment api key usage: %w", err)
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return fmt.Errorf("api key not found: %s", id)
	}
	
	return nil
}

// BulkUpdateStatus updates the status of multiple API keys at once.
func (s *PostgreSQLAPIKeyStore) BulkUpdateStatus(ctx context.Context, ids []string, status string) error {
	if len(ids) == 0 {
		return nil
	}
	
	// Build placeholder clause
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids)+1)
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	args[len(ids)] = status
	
	query := fmt.Sprintf(`
		UPDATE admin_api_keys
		SET status = $%d, updated_at = NOW()
		WHERE id IN (%s)
	`, len(ids)+1, join(", ", placeholders))
	
	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("bulk update api key status: %w", err)
	}
	
	return nil
}

// scanAPIKey scans a row into an APIKey struct.
func scanAPIKey(row scanner) (*models.APIKey, error) {
	var key models.APIKey
	var userID, createdBy, description sql.NullString
	var allowedModels []string
	
	err := row.Scan(
		&key.ID, &key.KeyHash, &key.Name, &key.TenantID, &userID,
		&key.MonthlyQuota, &key.UsedCount, &key.UsedCredits,
		&key.Status, &key.ExpiresAt, &key.CreatedAt, &key.UpdatedAt,
		&createdBy, &description, &allowedModels, &key.RateLimitQPS,
	)
	
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("api key not found")
		}
		return nil, err
	}
	
	if userID.Valid {
		key.UserID = &userID.String
	}
	if createdBy.Valid {
		key.CreatedBy = &createdBy.String
	}
	if description.Valid {
		key.Description = &description.String
	}
	key.AllowedModels = allowedModels
	
	return &key, nil
}

// scanner interface for scanning rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func join(separator string, parts []string) string {
	result := ""
	for i, part := range parts {
		if i > 0 {
			result += separator
		}
		result += part
	}
	return result
}
