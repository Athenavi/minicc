package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"minicc/internal/admin/models"

	"github.com/google/uuid"
)

// DomainStore defines the interface for domain persistence operations.
type DomainStore interface {
	Create(ctx context.Context, domain *models.Domain) error
	GetByID(ctx context.Context, id string) (*models.Domain, error)
	GetByDomain(ctx context.Context, domain string) (*models.Domain, error)
	List(ctx context.Context, filter models.ListDomainFilter) ([]models.Domain, error)
	Update(ctx context.Context, domainID string, updates models.UpdateDomainRequest) error
	Delete(ctx context.Context, domainID string) error
	VerifyDNS(ctx context.Context, domainID string) error
	RenewSSL(ctx context.Context, domainID string) error
}

// PostgreSQLDomainStore implements DomainStore using PostgreSQL.
type PostgreSQLDomainStore struct {
	db *sql.DB
}

// NewPostgreSQLDomainStore creates a new PostgreSQL domain store.
func NewPostgreSQLDomainStore(db *sql.DB) *PostgreSQLDomainStore {
	return &PostgreSQLDomainStore{db: db}
}

// Create inserts a new domain into the database.
func (s *PostgreSQLDomainStore) Create(ctx context.Context, domain *models.Domain) error {
	now := time.Now()
	domain.ID = uuid.New().String()
	domain.CreatedAt = now
	domain.UpdatedAt = now
	
	if domain.Status == "" {
		domain.Status = "active"
	}
	
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_domains (
			id, domain, tenant_id, dns_provider, dns_record_id, cname_target,
			ssl_status, ssl_expires_at, auto_renew, status, verified_at, verified_by,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`,
		domain.ID,
		domain.Domain,
		domain.TenantID,
		domain.DNSProvider,
		domain.DNSRecordID,
		domain.CNAMETarget,
		domain.SSLStatus,
		domain.SSLEXpiresAt,
		domain.AutoRenew,
		domain.Status,
		domain.VerifiedAt,
		domain.VerifiedBy,
		now,
		now,
	)
	
	if err != nil {
		return fmt.Errorf("create domain: %w", err)
	}
	
	_, err = result.RowsAffected()
	return err
}

// GetByID retrieves a domain by its ID.
func (s *PostgreSQLDomainStore) GetByID(ctx context.Context, id string) (*models.Domain, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, domain, tenant_id, dns_provider, dns_record_id, cname_target,
		       ssl_status, ssl_expires_at, auto_renew, status, verified_at, verified_by,
		       created_at, updated_at
		FROM admin_domains
		WHERE id = $1
	`, id)
	
	return scanDomain(row)
}

// GetByDomain retrieves a domain by its domain name.
func (s *PostgreSQLDomainStore) GetByDomain(ctx context.Context, domainName string) (*models.Domain, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, domain, tenant_id, dns_provider, dns_record_id, cname_target,
		       ssl_status, ssl_expires_at, auto_renew, status, verified_at, verified_by,
		       created_at, updated_at
		FROM admin_domains
		WHERE domain = $1
	`, domainName)
	
	return scanDomain(row)
}

// List retrieves a paginated list of domains with optional filters.
func (s *PostgreSQLDomainStore) List(ctx context.Context, filter models.ListDomainFilter) ([]models.Domain, error) {
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
		SELECT id, domain, tenant_id, dns_provider, dns_record_id, cname_target,
		       ssl_status, ssl_expires_at, auto_renew, status, verified_at, verified_by,
		       created_at, updated_at
		FROM admin_domains
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)
	
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}
	defer rows.Close()
	
	var domains []models.Domain
	for rows.Next() {
		domain, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		domains = append(domains, *domain)
	}
	
	return domains, nil
}

// Update modifies an existing domain.
func (s *PostgreSQLDomainStore) Update(ctx context.Context, domainID string, updates models.UpdateDomainRequest) error {
	var setClauses []string
	var args []interface{}
	argIdx := 1
	
	if updates.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *updates.Status)
		argIdx++
	}
	
	if updates.SSLStatus != nil {
		setClauses = append(setClauses, fmt.Sprintf("ssl_status = $%d", argIdx))
		args = append(args, *updates.SSLStatus)
		argIdx++
	}
	
	if updates.AutoRenew != nil {
		setClauses = append(setClauses, fmt.Sprintf("auto_renew = $%d", argIdx))
		args = append(args, *updates.AutoRenew)
		argIdx++
	}
	
	if updates.SSLEXpiresAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("ssl_expires_at = $%d", argIdx))
		args = append(args, *updates.SSLEXpiresAt)
		argIdx++
	}
	
	if len(setClauses) == 0 {
		return nil
	}
	
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	args = append(args, domainID)
	
	query := fmt.Sprintf(`
		UPDATE admin_domains
		SET %s
		WHERE id = $%d
	`, join(", ", setClauses), argIdx+1)
	
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update domain: %w", err)
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return fmt.Errorf("domain not found: %s", domainID)
	}
	
	return nil
}

// Delete removes a domain from the database.
func (s *PostgreSQLDomainStore) Delete(ctx context.Context, domainID string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM admin_domains
		WHERE id = $1
	`, domainID)
	
	if err != nil {
		return fmt.Errorf("delete domain: %w", err)
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return fmt.Errorf("domain not found: %s", domainID)
	}
	
	return nil
}

// VerifyDNS marks a domain as verified.
func (s *PostgreSQLDomainStore) VerifyDNS(ctx context.Context, domainID string) error {
	now := time.Now()
	result, err := s.db.ExecContext(ctx, `
		UPDATE admin_domains
		SET status = 'active', verified_at = $1, updated_at = $2
		WHERE id = $3
	`, now, now, domainID)
	
	if err != nil {
		return fmt.Errorf("verify domain: %w", err)
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return fmt.Errorf("domain not found: %s", domainID)
	}
	
	return nil
}

// RenewSSL updates the SSL certificate expiration date.
func (s *PostgreSQLDomainStore) RenewSSL(ctx context.Context, domainID string, expiresAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE admin_domains
		SET ssl_status = 'active', ssl_expires_at = $1, updated_at = NOW()
		WHERE id = $2
	`, expiresAt, domainID)
	
	if err != nil {
		return fmt.Errorf("renew ssl: %w", err)
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return fmt.Errorf("domain not found: %s", domainID)
	}
	
	return nil
}

// scanDomain scans a row into a Domain struct.
func scanDomain(row scanner) (*models.Domain, error) {
	var domain models.Domain
	var dnsProvider, dnsRecordID, cnamTarget, verifiedBy sql.NullString
	var verifiedAt sql.NullTime
	
	err := row.Scan(
		&domain.ID, &domain.Domain, &domain.TenantID,
		&dnsProvider, &dnsRecordID, &cnamTarget,
		&domain.SSLStatus, &domain.SSLEXpiresAt, &domain.AutoRenew,
		&domain.Status, &verifiedAt, &verifiedBy,
		&domain.CreatedAt, &domain.UpdatedAt,
	)
	
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("domain not found")
		}
		return nil, err
	}
	
	if dnsProvider.Valid {
		domain.DNSProvider = &dnsProvider.String
	}
	if dnsRecordID.Valid {
		domain.DNSRecordID = &dnsRecordID.String
	}
	if cnamTarget.Valid {
		domain.CNAMETarget = &cnamTarget.String
	}
	if verifiedBy.Valid {
		domain.VerifiedBy = &verifiedBy.String
	}
	if verifiedAt.Valid {
		domain.VerifiedAt = &verifiedAt.Time
	}
	
	return &domain, nil
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
