package models

import (
	"time"
)

// Tenant represents a multi-tenant organization.
type Tenant struct {
	ID   string `json:"id"`

	// 基本信息
	TenantID     string  `json:"tenant_id"`
	Name         string  `json:"name"`
	CompanyName  string  `json:"company_name,omitempty"`
	ContactEmail *string `json:"contact_email,omitempty"`
	ContactPhone *string `json:"contact_phone,omitempty"`

	// 配额控制
	MaxAPIKeys          int    `json:"max_api_keys"`
	MaxModels           int    `json:"max_models"`
	MonthlyQuota        int64  `json:"monthly_quota"` // 0 = unlimited
	MaxConcurrentSessions int  `json:"max_concurrent_sessions"`

	// 状态管理
	Status    string     `json:"status"` // active/suspended/expired
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	CreatedBy *string    `json:"created_by,omitempty"`

	// 高级特性
	Features map[string]bool `json:"features"`
}

// CreateTenantRequest represents the request to create a new tenant.
type CreateTenantRequest struct {
	TenantID              string         `json:"tenant_id" binding:"required"`
	Name                  string         `json:"name" binding:"required"`
	CompanyName           string         `json:"company_name"`
	ContactEmail          string         `json:"contact_email"`
	ContactPhone          string         `json:"contact_phone"`
	MaxAPIKeys            int            `json:"max_api_keys"`
	MaxModels             int            `json:"max_models"`
	MonthlyQuota          int64          `json:"monthly_quota"`
	MaxConcurrentSessions int            `json:"max_concurrent_sessions"`
	ExpiresAt             *time.Time     `json:"expires_at"`
	Features              map[string]bool `json:"features"`
}

// UpdateTenantRequest represents the request to update a tenant.
type UpdateTenantRequest struct {
	Name                  *string         `json:"name"`
	MonthlyQuota          *int64          `json:"monthly_quota"`
	MaxConcurrentSessions *int            `json:"max_concurrent_sessions"`
	ExpiresAt             *time.Time      `json:"expires_at"`
	Status                *string         `json:"status"`
	Features              map[string]bool `json:"features"`
}

// ListTenantFilter represents filters for listing tenants.
type ListTenantFilter struct {
	Page     int     `form:"page,default=1"`
	PageSize int     `form:"page_size,default=20"`
	Status   *string `form:"status"`
	Search   *string `form:"search"` // search by name or tenant_id
}

// TenantUsage represents daily usage statistics for a tenant.
type TenantUsage struct {
	ID             string  `json:"id"`
	TenantID       string  `json:"tenant_id"`
	StatDate       time.Time `json:"stat_date"`
	APICalls       int64   `json:"api_calls"`
	TokensUsed     int64   `json:"tokens_used"`
	CreditsConsumed int64  `json:"credits_consumed"`
	StorageMB      float64 `json:"storage_mb"`
	CreatedAt      time.Time `json:"created_at"`
}
