package api

import (
	"net/http"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
)

// handleKBVisibility 鐭ヨ瘑搴撳叡浜彲瑙佹€э紙owner-only锛夛細PUT /v1/kb/{id}/visibility
func handleKBVisibility(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		Unauthorized(w, "missing tenant context")
		return
	}
	var body struct {
		Visibility string `json:"visibility"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if body.Visibility != "private" && body.Visibility != "tenant" && body.Visibility != "public" {
		BadRequest(w, "visibility must be private, tenant, or public")
		return
	}
	kbID := r.PathValue("id")
	tag, err := db.Pool.Exec(r.Context(),
		`UPDATE knowledge_bases SET visibility = $1 WHERE id = $2 AND tenant_id = $3 AND user_id = $4`,
		body.Visibility, kbID, claims.TenantID, claims.UserID)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "update visibility failed")
		return
	}
	if tag.RowsAffected() == 0 {
		Forbidden(w, "knowledge base not found or not owned by you")
		return
	}
	OK(w, map[string]interface{}{"id": kbID, "visibility": body.Visibility})
}
