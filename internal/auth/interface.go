package auth

import "context"

// User represents a user in the system.
type User struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	TenantID string `json:"tenant_id"`
}

// AuthService defines the authentication service interface.
type AuthService interface {
	// GetUser retrieves a user by ID.
	GetUser(ctx context.Context, userID string) (*User, error)

	// GetUserByEmail retrieves a user by email within the given tenant.
	// tenantID 闃叉璺ㄧ鎴峰悓 email 璇櫥褰曘€?	GetUserByEmail(ctx context.Context, email, tenantID string) (*User, error)

	// ValidateToken validates a JWT token and returns claims.
	ValidateToken(ctx context.Context, token string) (*Claims, error)

	// CreateUser creates a new user.
	CreateUser(ctx context.Context, user *User) error

	// UpdateUser updates an existing user.
	UpdateUser(ctx context.Context, user *User) error

	// DeleteUser deletes a user by ID.
	DeleteUser(ctx context.Context, userID string) error

	// ListUsers lists users in the given tenant (admin only).
	ListUsers(ctx context.Context, tenantID string) ([]User, error)
}
