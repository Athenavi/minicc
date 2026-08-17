package models

import (
	"time"
)

// APIKey represents an API key with quota and lifecycle management.
type APIKey struct {
	ID string `json:"id"`
	
	// 基本信息
	Name         string `json:"name"`
	TenantID     string `json:"tenant_id"`
	UserID       *string `json:"user_id,omitempty"`
	
	// 配额控制
	MonthlyQuota int    `json:"monthly_quota"` // 0 = unlimited
	UsedCount    int64  `json:"used_count"`
	UsedCredits  int64  `json:"used_credits"`
	
	// 状态管理
	Status       string    `json:"status"` // active/expired/suspended
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	
	// 元数据
	CreatedBy    *string   `json:"created_by,omitempty"`
	Description  *string   `json:"description,omitempty"`
	AllowedModels []string `json:"allowed_models"`
	RateLimitQPS int       `json:"rate_limit_qps"`
	
	// 隐藏字段 (不返回给前端)
	KeyHash string `json:"-"`
}

// CreateAPIKeyRequest represents the request to create a new API key.
type CreateAPIKeyRequest struct {
	Name          string   `json:"name" binding:"required"`
	TenantID      string   `json:"tenant_id" binding:"required"`
	MonthlyQuota  int      `json:"monthly_quota"`
	ExpiresAt     *time.Time `json:"expires_at"`
	AllowedModels []string `json:"allowed_models"`
	RateLimitQPS  int      `json:"rate_limit_qps"`
	Description   string   `json:"description"`
}

// UpdateAPIKeyRequest represents the request to update an API key.
type UpdateAPIKeyRequest struct {
	Name          *string  `json:"name"`
	MonthlyQuota  *int     `json:"monthly_quota"`
	ExpiresAt     *time.Time `json:"expires_at"`
	Status        *string  `json:"status"`
	Description   *string  `json:"description"`
	RateLimitQPS  *int     `json:"rate_limit_qps"`
}

// ListAPIKeyFilter represents filters for listing API keys.
type ListAPIKeyFilter struct {
	Page         int    `form:"page,default=1"`
	PageSize     int    `form:"page_size,default=20"`
	Status       *string `form:"status"`
	TenantID     *string `form:"tenant_id"`
	ExpiresBefore *time.Time `form:"expires_before"`
	Search       *string `form:"search"` // search by name
}
