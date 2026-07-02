package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"golang.org/x/crypto/bcrypt"
)

const tokenCookieName = "minicc_token"

type AuthHandler struct {
	auth *auth.Authenticator
	cfg  *config.Config
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

type UserResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

// SetTokenCookie sets the JWT as an HTTP-only secure cookie.
func SetTokenCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     tokenCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
	})
}

func ClearTokenCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     tokenCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		BadRequest(w, "email and password are required")
		return
	}

	// No dev bypass — always validate against DB
	if db.Pool == nil {
		InternalError(w, "database not available")
		return
	}

	var user UserResponse
	var passwordHash string
	err := db.Pool.QueryRow(r.Context(),
		`SELECT id, email, name, role, password_hash FROM users WHERE email = $1`, req.Email,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &passwordHash)
	if err != nil {
		Unauthorized(w, "invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		Unauthorized(w, "invalid email or password")
		return
	}

	token, err := h.auth.GenerateToken(user.ID, user.Email, user.Role, auth.RolePermissions[user.Role])
	if err != nil {
		InternalError(w, "authentication failed")
		return
	}

	SetTokenCookie(w, token, int(h.cfg.JWTExpiration.Seconds()))
	OK(w, map[string]interface{}{
		"user": user,
	})
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" || req.Name == "" {
		BadRequest(w, "email, password, and name are required")
		return
	}
	if len(req.Password) < 8 {
		BadRequest(w, "password must be at least 8 characters")
		return
	}
	if len(req.Email) > 255 || len(req.Name) > 128 {
		BadRequest(w, "email or name too long")
		return
	}

	if db.Pool == nil {
		InternalError(w, "database not available")
		return
	}

	// Check for existing user
	var exists int
	db.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM users WHERE email = $1`, req.Email).Scan(&exists)
	if exists > 0 {
		BadRequest(w, "email already registered")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		InternalError(w, "registration failed")
		return
	}

	userID := generateID()
	_, err = db.Pool.Exec(r.Context(),
		`INSERT INTO users (id, email, name, password_hash, role, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 'user', NOW(), NOW())`,
		userID, req.Email, req.Name, string(hash),
	)
	if err != nil {
		InternalError(w, "registration failed")
		return
	}

	token, err := h.auth.GenerateToken(userID, req.Email, "user", auth.RolePermissions["user"])
	if err != nil {
		InternalError(w, "authentication failed")
		return
	}

	SetTokenCookie(w, token, int(h.cfg.JWTExpiration.Seconds()))
	Created(w, map[string]interface{}{
		"user": UserResponse{ID: userID, Email: req.Email, Name: req.Name, Role: "user"},
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ClearTokenCookie(w)
	OK(w, map[string]string{"message": "logged out"})
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

type RefreshRequest struct {
	Token string `json:"token"`
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	// Read token from cookie
	cookie, err := r.Cookie(tokenCookieName)
	if err != nil {
		Unauthorized(w, "not authenticated")
		return
	}

	newToken, err := h.auth.RefreshToken(cookie.Value)
	if err != nil {
		Unauthorized(w, "session expired")
		return
	}

	SetTokenCookie(w, newToken, int(h.cfg.JWTExpiration.Seconds()))
	OK(w, map[string]string{"message": "token refreshed"})
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
