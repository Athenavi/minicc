package api

import (
	"net/http"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"golang.org/x/crypto/bcrypt"
)

type InstallHandler struct {
	cfg  *config.Config
	auth *auth.Authenticator
}

func NewInstallHandler(cfg *config.Config) *InstallHandler {
	return &InstallHandler{
		cfg:  cfg,
		auth: auth.NewAuthenticator(cfg.JWTSecret, cfg.JWTExpiration),
	}
}

type InstallStatus struct {
	Needed bool   `json:"needed"`
	Reason string `json:"reason,omitempty"`
	DB     bool   `json:"db"`
}

// Status checks if the system needs initialization.
// GET /v1/install/status
func (h *InstallHandler) Status(w http.ResponseWriter, r *http.Request) {
	status := InstallStatus{DB: db.Pool != nil}

	if db.Pool == nil {
		status.Needed = true
		status.Reason = "database not connected"
		OK(w, status)
		return
	}

	// If at least one user with role 'owner' exists, system is initialized
	var count int
	err := db.ReadPool().QueryRow(r.Context(), `SELECT COUNT(*) FROM users WHERE role = 'owner'`).Scan(&count)
	if err != nil || count == 0 {
		status.Needed = true
		status.Reason = "no admin user configured"
	}

	OK(w, status)
}

type SetupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// Setup initializes the system with the first admin user.
// POST /v1/install/setup
func (h *InstallHandler) Setup(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		InternalError(w, "database not available")
		return
	}

	var req SetupRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	// Validate
	if req.Email == "" || req.Password == "" || req.Name == "" {
		BadRequest(w, "email, password, and name are required")
		return
	}
	if len(req.Password) < 8 {
		BadRequest(w, "password must be at least 8 characters")
		return
	}

	// Ensure no admin exists (idempotent)
	var count int
	db.ReadPool().QueryRow(r.Context(), `SELECT COUNT(*) FROM users WHERE role = 'owner'`).Scan(&count)
	if count > 0 {
		BadRequest(w, "system already initialized")
		return
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		InternalError(w, "setup failed")
		return
	}

	// Create owner user using PostgreSQL's gen_random_uuid()
	var userID string
	err = db.Pool.QueryRow(r.Context(),
		`INSERT INTO users (id, tenant_id, email, name, password_hash, role, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, 'owner', NOW(), NOW())
		 RETURNING id`,
		DefaultTenantID, req.Email, req.Name, string(hash),
	).Scan(&userID)
	if err != nil {
		InternalError(w, "failed to create admin user")
		return
	}

	// Generate token and set cookie
	token, err := h.auth.GenerateToken(userID, req.Email, "owner", auth.RolePermissions["owner"])
	if err != nil {
		InternalError(w, "authentication failed")
		return
	}

	SetTokenCookie(w, token, int(h.cfg.JWTExpiration.Seconds()))
	Created(w, map[string]interface{}{
		"message": "system initialized",
		"user": map[string]string{
			"id":    userID,
			"email": req.Email,
			"name":  req.Name,
			"role":  "owner",
		},
	})
}
