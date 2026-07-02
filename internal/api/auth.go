package api

import (
	"net/http"
	"time"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
)

type AuthHandler struct {
	auth  *auth.Authenticator
	cfg   *config.Config
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		auth: auth.NewAuthenticator(cfg.JWTSecret, cfg.JWTExpiration),
		cfg:  cfg,
	}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	User      struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
		Role  string `json:"role"`
	} `json:"user"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		BadRequest(w, "email and password required")
		return
	}

	// TODO: verify against PostgreSQL
	// For now, dev mode: accept any email with password "dev"
	if req.Password != "dev" {
		Unauthorized(w, "invalid credentials")
		return
	}

	token, err := h.auth.GenerateToken("dev-user", req.Email, "owner", auth.RolePermissions["owner"])
	if err != nil {
		InternalError(w, "token generation failed")
		return
	}

	OK(w, LoginResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(h.cfg.JWTExpiration).Format(time.RFC3339),
		User: struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Name  string `json:"name"`
			Role  string `json:"role"`
		}{
			ID:    "dev-user",
			Email: req.Email,
			Name:  "Developer",
			Role:  "owner",
		},
	})
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" || req.Name == "" {
		BadRequest(w, "email, password, and name required")
		return
	}

	// TODO: insert into PostgreSQL
	token, err := h.auth.GenerateToken("new-user", req.Email, "user", auth.RolePermissions["user"])
	if err != nil {
		InternalError(w, "token generation failed")
		return
	}

	Created(w, LoginResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(h.cfg.JWTExpiration).Format(time.RFC3339),
		User: struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Name  string `json:"name"`
			Role  string `json:"role"`
		}{
			ID:    "new-user",
			Email: req.Email,
			Name:  req.Name,
			Role:  "user",
		},
	})
}

type RefreshRequest struct {
	Token string `json:"token"`
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	newToken, err := h.auth.RefreshToken(req.Token)
	if err != nil {
		Unauthorized(w, "invalid or expired token")
		return
	}

	OK(w, map[string]string{
		"token":      newToken,
		"expires_at": time.Now().Add(h.cfg.JWTExpiration).Format(time.RFC3339),
	})
}

func (h *AuthHandler) Profile(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, "not authenticated")
		return
	}

	OK(w, map[string]interface{}{
		"user_id": claims.UserID,
		"email":   claims.Email,
		"role":    claims.Role,
		"perms":   claims.Perms,
	})
}
