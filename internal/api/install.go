package api

import (
	"context"
	"net/http"
	"time"

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
	Redis  bool   `json:"redis"`

	// 依赖探测明细（初始化页面展示各就绪项）
	Deps []InstallDep `json:"deps,omitempty"`
}

type InstallDep struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// Status checks if the system needs initialization.
// GET /v1/install/status
func (h *InstallHandler) Status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var status InstallStatus
	status.Deps = make([]InstallDep, 0, 2)

	// 依赖 1：PostgreSQL 连通性（真实 ping）
	dbOK := db.Pool != nil && db.Pool.Ping(ctx) == nil
	status.DB = dbOK
	status.Deps = append(status.Deps, InstallDep{
		Name:    "postgres",
		OK:      dbOK,
		Message: map[bool]string{true: "PostgreSQL 连接正常", false: "PostgreSQL 不可用：请检查 POSTGRES_DSN"}[dbOK],
	})

	// 依赖 2：Redis 连通性（真实 ping）
	redisOK := db.Redis != nil && db.Redis.Ping(ctx).Err() == nil
	status.Redis = redisOK
	status.Deps = append(status.Deps, InstallDep{
		Name:    "redis",
		OK:      redisOK,
		Message: map[bool]string{true: "Redis 连接正常", false: "Redis 不可用：请检查 REDIS_ADDR / 密码"}[redisOK],
	})

	// If at least one user with role 'owner' exists, system is initialized
	if dbOK {
		var count int
		err := db.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM users WHERE role = 'owner'`).Scan(&count)
		if err != nil || count == 0 {
			status.Needed = true
			status.Reason = "no admin user configured"
		}
	} else {
		status.Needed = true
		status.Reason = "postgres unavailable"
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
	var req SetupRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, ErrInvalidReq)
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

	// P1 修复：用事务 + 咨询锁保证初始化原子性，避免并发/读副本滞后重复初始化
	tx, err := db.Pool.Begin(r.Context())
	if err != nil {
		InternalError(w, "setup failed")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtext('minicc_install'))`); err != nil {
		InternalError(w, "setup failed")
		return
	}
	var count int
	if err := tx.QueryRow(r.Context(), `SELECT COUNT(*) FROM users WHERE role = 'owner'`).Scan(&count); err != nil {
		InternalError(w, "setup failed")
		return
	}
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
	err = tx.QueryRow(r.Context(),
		`INSERT INTO users (id, tenant_id, email, name, password_hash, role, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, 'owner', NOW(), NOW())
		 RETURNING id`,
		DefaultTenantID, req.Email, req.Name, string(hash),
	).Scan(&userID)
	if err != nil {
		InternalError(w, "failed to create admin user")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		InternalError(w, "setup failed")
		return
	}

	// Generate token and set cookie
	token, err := h.auth.GenerateToken(userID, req.Email, "owner", DefaultTenantID, auth.RolePermissions["owner"])
	if err != nil {
		InternalError(w, "authentication failed")
		return
	}

	SetTokenCookie(w, token, int(h.cfg.JWTExpiration.Seconds()), h.cfg.CookieSecure)
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
