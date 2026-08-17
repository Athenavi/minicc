package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"minicc/internal/admin/models"

	"github.com/google/uuid"
)

// TenantStore defines the interface for tenant persistence operations.
type TenantStore interface {
	Create(ctx context.Context, tenant *models.Tenant) error
	GetByID(ctx context.Context, id string) (*models.Tenant, error)
	GetByTenantID(ctx context.Context, tenantID string) (*models.Tenant, error)
	List(ctx context.Context, filter models.ListTenantFilter) ([]models.Tenant, error)
	Total(ctx context.Context, filter models.ListTenantFilter) (int, error)
	Update(ctx context.Context, tenantID string, updates models.UpdateTenantRequest) error
	Suspend(ctx context.Context, tenantID string) error
	Delete(ctx context.Context, tenantID string) error
	RecordUsage(ctx context.Context, usage *models.TenantUsage) error
	GetUsage(ctx context.Context, tenantID string, startDate, endDate time.Time) ([]models.TenantUsage, error)
}

// PostgreSQLTenantStore implements TenantStore using PostgreSQL.
type PostgreSQLTenantStore struct {
	db *sql.DB
}

// NewPostgreSQLTenantStore creates a new PostgreSQL tenant store.
func NewPostgreSQLTenantStore(db *sql.DB) *PostgreSQLTenantStore {
	return &PostgreSQLTenantStore{db: db}
}

// Create inserts a new tenant into the database.
func (s *PostgreSQLTenantStore) Create(ctx context.Context, tenant *models.Tenant) error {
	now := time.Now()
	tenant.ID = uuid.New().String()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	
	if tenant.Status == "" {
		tenant.Status = "active"
	}
	
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_tenants (
			id, tenant_id, name, company_name, contact_email, contact_phone,
			max_api_keys, max_models, monthly_quota, max_concurrent_sessions,
			status, expires_at, created_at, updated_at, created_by, features
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`,
		tenant.ID,
		tenant.TenantID,
		tenant.Name,
		tenant.CompanyName,
		tenant.ContactEmail,
		tenant.ContactPhone,
		tenant.MaxAPIKeys,
		tenant.MaxModels,
		tenant.MonthlyQuota,
		tenant.MaxConcurrentSessions,
		tenant.Status,
		tenant.ExpiresAt,
		now,
		now,
		tenant.CreatedBy,
		tenant.Features,
	)
	
	if err != nil {
		return fmt.Errorf("create tenant: %w", err)
	}
	
	_, err = result.RowsAffected()
	return err
}

// GetByID retrieves a tenant by its ID.
func (s *PostgreSQLTenantStore) GetByID(ctx context.Context, id string) (*models.Tenant, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, company_name, contact_email, contact_phone,
		       max_api_keys, max_models, monthly_quota, max_concurrent_sessions,
		       status, expires_at, created_at, updated_at, created_by, features
		FROM admin_tenants
		WHERE id = $1
	`, id)
	
	return scanTenant(row)
}

// GetByTenantID retrieves a tenant by its tenant_id.
func (s *PostgreSQLTenantStore) GetByTenantID(ctx context.Context, tenantID string) (*models.Tenant, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, company_name, contact_email, contact_phone,
		       max_api_keys, max_models, monthly_quota, max_concurrent_sessions,
		       status, expires_at, created_at, updated_at, created_by, features
		FROM admin_tenants
		WHERE tenant_id = $1
	`, tenantID)
	
	return scanTenant(row)
}

// List retrieves a paginated list of tenants with optional filters.
func (s *PostgreSQLTenantStore) List(ctx context.Context, filter models.ListTenantFilter) ([]models.Tenant, error) {
	whereClause := "WHERE 1=1"
	var args []interface{}
	argIdx := 1
	
	if filter.Status != nil {
		whereClause += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, *filter.Status)
		argIdx++
	}
	
	if filter.Search != nil {
		whereClause += fmt.Sprintf(" AND (name ILIKE $%d OR tenant_id ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+*filter.Search+"%", "%"+*filter.Search+"%")
		argIdx++
	}
	
	query := fmt.Sprintf(`
		SELECT id, tenant_id, name, company_name, contact_email, contact_phone,
		       max_api_keys, max_models, monthly_quota, max_concurrent_sessions,
		       status, expires_at, created_at, updated_at, created_by, features
		FROM admin_tenants
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)
	
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()
	
	var tenants []models.Tenant
	for rows.Next() {
		tenant, err := scanTenant(rows)
		if err != nil {
			return nil, err
		}
		tenants = append(tenants, *tenant)
	}
	
	return tenants, nil
}

// Total returns the total count of tenants matching the filter.
func (s *PostgreSQLTenantStore) Total(ctx context.Context, filter models.ListTenantFilter) (int, error) {
	whereClause := "WHERE 1=1"
	var args []interface{}
	argIdx := 1
	
	if filter.Status != nil {
		whereClause += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, *filter.Status)
		argIdx++
	}
	
	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM admin_tenants
		%s
	`, whereClause)
	
	var total int
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&total)
	if err != nil {
		return 0, err
	}
	
	return total, nil
}

// Update modifies an existing tenant.
func (s *PostgreSQLTenantStore) Update(ctx context.Context, tenantID string, updates models.UpdateTenantRequest) error {
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
	
	if updates.MaxConcurrentSessions != nil {
		setClauses = append(setClauses, fmt.Sprintf("max_concurrent_sessions = $%d", argIdx))
		args = append(args, *updates.MaxConcurrentSessions)
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
	
	if updates.Features != nil {
		setClauses = append(setClauses, fmt.Sprintf("features = $%d", argIdx))
		args = append(args, updates.Features)
		argIdx++
	}
	
	if len(setClauses) == 0 {
		return nil
	}
	
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	args = append(args, tenantID)
	
	query := fmt.Sprintf(`
		UPDATE admin_tenants
		SET %s
		WHERE tenant_id = $%d
	`, join(", ", setClauses), argIdx+1)
	
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update tenant: %w", err)
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return fmt.Errorf("tenant not found: %s", tenantID)
	}
	
	return nil
}

// Suspend suspends a tenant.
func (s *PostgreSQLTenantStore) Suspend(ctx context.Context, tenantID string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE admin_tenants
		SET status = 'suspended', updated_at = NOW()
		WHERE tenant_id = $1
	`, tenantID)
	
	if err != nil {
		return fmt.Errorf("suspend tenant: %w", err)
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return fmt.Errorf("tenant not found: %s", tenantID)
	}
	
	return nil
}

// Delete removes a tenant from the database.
func (s *PostgreSQLTenantStore) Delete(ctx context.Context, tenantID string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM admin_tenants
		WHERE tenant_id = $1
	`, tenantID)
	
	if err != nil {
		return fmt.Errorf("delete tenant: %w", err)
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return fmt.Errorf("tenant not found: %s", tenantID)
	}
	
	return nil
}

// RecordUsage records daily usage statistics for a tenant.
func (s *PostgreSQLTenantStore) RecordUsage(ctx context.Context, usage *models.TenantUsage) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_tenant_usage (
			tenant_id, stat_date, api_calls, tokens_used, credits_consumed, storage_mb
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (tenant_id, stat_date)
		DO UPDATE SET
			api_calls = admin_tenant_usage.api_calls + EXCLUDED.api_calls,
			tokens_used = admin_tenant_usage.tokens_used + EXCLUDED.tokens_used,
			credits_consumed = admin_tenant_usage.credits_consumed + EXCLUDED.credits_consumed,
			storage_mb = admin_tenant_usage.storage_mb + EXCLUDED.storage_mb
	`,
		usage.TenantID,
		usage.StatDate,
		usage.APICalls,
		usage.TokensUsed,
		usage.CreditsConsumed,
		usage.StorageMB,
	)
	
	if err != nil {
		return fmt.Errorf("record usage: %w", err)
	}
	
	return nil
}

// GetUsage retrieves usage statistics for a tenant within a date range.
func (s *PostgreSQLTenantStore) GetUsage(ctx context.Context, tenantID string, startDate, endDate time.Time) ([]models.TenantUsage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, stat_date, api_calls, tokens_used, credits_consumed, storage_mb, created_at
		FROM admin_tenant_usage
		WHERE tenant_id = $1 AND stat_date BETWEEN $2 AND $3
		ORDER BY stat_date ASC
	`, tenantID, startDate, endDate)
	
	if err != nil {
		return nil, fmt.Errorf("get usage: %w", err)
	}
	defer rows.Close()
	
	var usages []models.TenantUsage
	for rows.Next() {
		var usage models.TenantUsage
		err := rows.Scan(
			&usage.ID, &usage.TenantID, &usage.StatDate,
			&usage.APICalls, &usage.TokensUsed, &usage.CreditsConsumed,
			&usage.StorageMB, &usage.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		usages = append(usages, usage)
	}
	
	return usages, nil
}

// scanTenant scans a row into a Tenant struct.
func scanTenant(row scanner) (*models.Tenant, error) {
	var tenant models.Tenant
	var contactEmail, contactPhone, createdBy sql.NullString
	var features map[string]bool
	
	err := row.Scan(
		&tenant.ID, &tenant.TenantID, &tenant.Name, &tenant.CompanyName,
		&contactEmail, &contactPhone,
		&tenant.MaxAPIKeys, &tenant.MaxModels, &tenant.MonthlyQuota,
		&tenant.MaxConcurrentSessions,
		&tenant.Status, &tenant.ExpiresAt, &tenant.CreatedAt, &tenant.UpdatedAt,
		&createdBy, &features,
	)
	
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tenant not found")
		}
		return nil, err
	}
	
	if contactEmail.Valid {
		tenant.ContactEmail = &contactEmail.String
	}
	if contactPhone.Valid {
		tenant.ContactPhone = &contactPhone.String
	}
	if createdBy.Valid {
		tenant.CreatedBy = &createdBy.String
	}
	tenant.Features = features
	
	return &tenant, nil
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
