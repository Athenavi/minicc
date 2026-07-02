package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/monitor"
	"github.com/go-chi/chi/v5"
)

// AdminHandler provides admin-only management endpoints.
type AdminHandler struct {
	authenticator *auth.Authenticator
}

func NewAdminHandler(a *auth.Authenticator) *AdminHandler {
	return &AdminHandler{authenticator: a}
}

// ── Routes ────────────────────────────────────────────────────────────────

// RegisterRoutes adds admin endpoints to the given router under /v1/admin.
// Caller is responsible for auth middleware.
func (h *AdminHandler) RegisterRoutes(r chi.Router) {
	r.Get("/metrics", h.Metrics)
	r.Get("/users", h.ListUsers)
	r.Get("/users/{id}", h.GetUser)
	r.Put("/users/{id}", h.UpdateUser)
	r.Delete("/users/{id}", h.DeleteUser)
	r.Get("/system", h.SystemInfo)
	r.Post("/maintenance", h.TriggerMaintenance)
}

// ── Metrics ──────────────────────────────────────────────────────────────

func (h *AdminHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	OK(w, monitor.Snapshot())
}

// ── User Management ───────────────────────────────────────────────────────

type AdminUser struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		OK(w, []AdminUser{})
		return
	}

	rows, err := db.Pool.Query(r.Context(),
		`SELECT id, email, name, role, created_at, updated_at
		 FROM users ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		InternalError(w, "query users: "+err.Error())
		return
	}
	defer rows.Close()

	users := make([]AdminUser, 0)
	for rows.Next() {
		var u AdminUser
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &createdAt, &updatedAt); err != nil {
			continue
		}
		u.CreatedAt = createdAt.Format(time.RFC3339)
		u.UpdatedAt = updatedAt.Format(time.RFC3339)
		users = append(users, u)
	}

	OK(w, users)
}

func (h *AdminHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	var u AdminUser
	var createdAt, updatedAt time.Time
	err := db.Pool.QueryRow(r.Context(),
		`SELECT id, email, name, role, created_at, updated_at
		 FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Email, &u.Name, &u.Role, &createdAt, &updatedAt)
	if err != nil {
		NotFound(w, "user not found")
		return
	}
	u.CreatedAt = createdAt.Format(time.RFC3339)
	u.UpdatedAt = updatedAt.Format(time.RFC3339)

	OK(w, u)
}

func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	var body struct {
		Email string `json:"email"`
		Name  string `json:"name"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	// Validate role
	if body.Role != "" && body.Role != "owner" && body.Role != "admin" && body.Role != "user" {
		BadRequest(w, "invalid role: must be owner, admin, or user")
		return
	}

	// Build dynamic UPDATE
	setClauses := ""
	args := []interface{}{}
	argIdx := 1

	if body.Email != "" {
		setClauses += fmt.Sprintf("email = $%d, ", argIdx)
		args = append(args, body.Email)
		argIdx++
	}
	if body.Name != "" {
		setClauses += fmt.Sprintf("name = $%d, ", argIdx)
		args = append(args, body.Name)
		argIdx++
	}
	if body.Role != "" {
		setClauses += fmt.Sprintf("role = $%d, ", argIdx)
		args = append(args, body.Role)
		argIdx++
	}

	if setClauses == "" {
		BadRequest(w, "no fields to update")
		return
	}

	setClauses += fmt.Sprintf("updated_at = NOW()")
	args = append(args, id)

	query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d", setClauses, argIdx)
	result, err := db.Pool.Exec(r.Context(), query, args...)
	if err != nil {
		InternalError(w, "update user: "+err.Error())
		return
	}
	if result.RowsAffected() == 0 {
		NotFound(w, "user not found")
		return
	}

	OK(w, map[string]string{"status": "updated"})
}

func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	// Prevent deleting yourself
	claims := auth.GetClaims(r.Context())
	if claims != nil && claims.UserID == id {
		BadRequest(w, "cannot delete your own account")
		return
	}

	_, err := db.Pool.Exec(r.Context(), `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		InternalError(w, "delete user: "+err.Error())
		return
	}

	OK(w, map[string]string{"status": "deleted"})
}

// ── System Management ─────────────────────────────────────────────────────

func (h *AdminHandler) SystemInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"version":   "2.0.0",
		"uptime":    time.Since(monitor.Global.StartTime).String(),
		"db":        map[string]interface{}{
			"postgres": db.Pool != nil,
			"redis":    db.Redis != nil,
		},
	}
	OK(w, info)
}

func (h *AdminHandler) TriggerMaintenance(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"` // vacuum | reindex | analyze | flush_cache
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Action == "" {
		BadRequest(w, "action is required (vacuum, reindex, analyze, flush_cache)")
		return
	}

	switch body.Action {
	case "vacuum":
		if db.Pool != nil {
			db.Pool.Exec(r.Context(), "VACUUM ANALYZE")
		}
	case "reindex":
		if db.Pool != nil {
			db.Pool.Exec(r.Context(), "REINDEX DATABASE minicc")
		}
	case "analyze":
		if db.Pool != nil {
			db.Pool.Exec(r.Context(), "ANALYZE")
		}
	case "flush_cache":
		if db.Redis != nil {
			db.Redis.FlushDB(r.Context())
		}
	default:
		BadRequest(w, fmt.Sprintf("unknown action: %s", body.Action))
		return
	}

	monitor.NewTracer(nil)

	OK(w, map[string]string{
		"status": "completed",
		"action": body.Action,
	})
}
