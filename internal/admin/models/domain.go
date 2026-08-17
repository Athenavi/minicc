package models

import (
	"time"
)

// Domain represents a custom domain with SSL certificate.
type Domain struct {
	ID   string `json:"id"`

	// 基本信息
	Domain     string `json:"domain"`
	TenantID   string `json:"tenant_id"`
	DNSProvider *string `json:"dns_provider,omitempty"` // cloudflare / aliyun / tencent
	DNSRecordID *string `json:"dns_record_id,omitempty"`
	CNAMETarget *string `json:"cname_target,omitempty"`

	// SSL 证书
	SSLStatus    string     `json:"ssl_status"` // pending/active/expired/failed
	SSLEXpiresAt *time.Time `json:"ssl_expires_at,omitempty"`
	AutoRenew    bool       `json:"auto_renew"`

	// 状态管理
	Status      string     `json:"status"` // active/inactive/verifying
	VerifiedAt  *time.Time `json:"verified_at,omitempty"`
	VerifiedBy  *string    `json:"verified_by,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateDomainRequest represents the request to create a new domain.
type CreateDomainRequest struct {
	Domain      string   `json:"domain" binding:"required"`
	TenantID    string   `json:"tenant_id" binding:"required"`
	DNSProvider string   `json:"dns_provider"`
	CNAMETarget string   `json:"cname_target"`
	AutoRenew   bool     `json:"auto_renew"`
}

// UpdateDomainRequest represents the request to update a domain.
type UpdateDomainRequest struct {
	Status        *string    `json:"status"`
	SSLStatus     *string    `json:"ssl_status"`
	AutoRenew     *bool      `json:"auto_renew"`
	SSLEXpiresAt  *time.Time `json:"ssl_expires_at"`
}

// ListDomainFilter represents filters for listing domains.
type ListDomainFilter struct {
	Page     int     `form:"page,default=1"`
	PageSize int     `form:"page_size,default=20"`
	Status   *string `form:"status"`
	TenantID *string `form:"tenant_id"`
}
