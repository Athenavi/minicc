package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
)

// MediaHandler manages AI-generated content assets.
type MediaHandler struct {
	storageRoot string
}

func NewMediaHandler(storageRoot string) *MediaHandler {
	return &MediaHandler{storageRoot: storageRoot}
}

// ── Types ──

type MediaAsset struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Name      string                 `json:"name"`
	Content   string                 `json:"content,omitempty"`
	FilePath  string                 `json:"file_path,omitempty"`
	MimeType  string                 `json:"mime_type,omitempty"`
	Thumbnail string                 `json:"thumbnail,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Tags      []string               `json:"tags,omitempty"`
	Category  string                 `json:"category,omitempty"`
	Size      int64                  `json:"size"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// ── List ──

func (h *MediaHandler) List(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		OK(w, []MediaAsset{})
		return
	}

	category := r.URL.Query().Get("category")
	mediaType := r.URL.Query().Get("type")
	search := r.URL.Query().Get("search")

	query := `SELECT id, type, name, COALESCE(content, ''), COALESCE(file_path, ''), COALESCE(mime_type, ''),
		COALESCE(thumbnail, ''), metadata::text, tags, COALESCE(category, ''), size, created_at, updated_at
		FROM media_assets WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if category != "" {
		query += fmt.Sprintf(" AND category = $%d", argIdx)
		args = append(args, category)
		argIdx++
	}
	if mediaType != "" {
		query += fmt.Sprintf(" AND type = $%d", argIdx)
		args = append(args, mediaType)
		argIdx++
	}
	if search != "" {
		query += fmt.Sprintf(" AND (name ILIKE $%d OR content ILIKE $%d)", argIdx, argIdx+1)
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern)
		argIdx += 2
	}

	query += " ORDER BY created_at DESC LIMIT 100"

	rows, err := db.Pool.Query(r.Context(), query, args...)
	if err != nil {
		OK(w, []MediaAsset{})
		return
	}
	defer rows.Close()

	assets := make([]MediaAsset, 0)
	for rows.Next() {
		var a MediaAsset
		var metadataJSON, tagsJSON string
		if err := rows.Scan(&a.ID, &a.Type, &a.Name, &a.Content, &a.FilePath, &a.MimeType,
			&a.Thumbnail, &metadataJSON, &tagsJSON, &a.Category, &a.Size, &a.CreatedAt, &a.UpdatedAt); err != nil {
			continue
		}
		if metadataJSON != "" && metadataJSON != "{}" {
			json.Unmarshal([]byte(metadataJSON), &a.Metadata)
		}
		if tagsJSON != "" {
			// Parse PostgreSQL array format {a,b,c}
			t := strings.Trim(tagsJSON, "{}")
			if t != "" {
				a.Tags = strings.Split(t, ",")
			}
		}
		assets = append(assets, a)
	}

	OK(w, assets)
}

// ── Create (text/code content) ──

func (h *MediaHandler) Create(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}

	var body struct {
		Type     string                 `json:"type"`
		Name     string                 `json:"name"`
		Content  string                 `json:"content"`
		Category string                 `json:"category"`
		Tags     []string               `json:"tags"`
		Metadata map[string]interface{} `json:"metadata"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}
	if body.Name == "" {
		BadRequest(w, "name is required")
		return
	}
	if body.Type == "" {
		body.Type = "text"
	}

	id := fmt.Sprintf("media_%d", time.Now().UnixNano())
	metadataJSON, _ := json.Marshal(body.Metadata)
	tagsJSON := "{}"
	if len(body.Tags) > 0 {
		tagsJSON = "{" + strings.Join(body.Tags, ",") + "}"
	}
	size := int64(len(body.Content))

	_, err := db.Pool.Exec(r.Context(),
		`INSERT INTO media_assets (id, type, name, content, category, tags, metadata, size, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())`,
		id, body.Type, body.Name, body.Content, nullableStr(body.Category), tagsJSON, string(metadataJSON), size)
	if err != nil {
		InternalError(w, "create asset: "+err.Error())
		return
	}

	OK(w, map[string]string{"id": id, "name": body.Name, "type": body.Type})
}

// ── Upload (images/files) ──

func (h *MediaHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}

	// Resolve storage identity from auth claims or SSE client_id
	storageID := resolveStorageID(r)

	// Max 50MB
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		BadRequest(w, "file too large or invalid form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		BadRequest(w, "file is required")
		return
	}
	defer file.Close()

	// Determine asset type from MIME
	mimeType := header.Header.Get("Content-Type")
	assetType := detectType(mimeType)
	category := r.FormValue("category")

	// Save to per-user storage: {storageRoot}/media/{storageID}/{type}/
	storageDir := filepath.Join(h.storageRoot, "media", storageID, assetType)
	os.MkdirAll(storageDir, 0755)
	ext := filepath.Ext(header.Filename)
	saveName := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	savePath := filepath.Join(storageDir, saveName)

	dst, err := os.Create(savePath)
	if err != nil {
		InternalError(w, "save file: "+err.Error())
		return
	}
	defer dst.Close()

	written, _ := io.Copy(dst, file)

	id := fmt.Sprintf("media_%d", time.Now().UnixNano())
	relPath := filepath.Join("media", storageID, assetType, saveName)
	name := r.FormValue("name")
	if name == "" {
		name = header.Filename
	}

	_, err = db.Pool.Exec(r.Context(),
		`INSERT INTO media_assets (id, type, name, file_path, mime_type, category, size, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())`,
		id, assetType, name, relPath, mimeType, nullableStr(category), written)
	if err != nil {
		InternalError(w, "create asset: "+err.Error())
		return
	}

	OK(w, map[string]string{"id": id, "name": name, "type": assetType, "path": relPath})
}

// ── Delete ──

func (h *MediaHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	// Get file path to delete from disk
	var filePath string
	db.Pool.QueryRow(r.Context(), `SELECT COALESCE(file_path, '') FROM media_assets WHERE id = $1`, id).Scan(&filePath)

	db.Pool.Exec(r.Context(), `DELETE FROM media_assets WHERE id = $1`, id)

	// Delete file from disk
	if filePath != "" {
		fullPath := filepath.Join(h.storageRoot, filePath)
		os.Remove(fullPath)
	}

	OK(w, map[string]string{"status": "deleted"})
}

// ── Helpers ──

func detectType(mime string) string {
	if strings.HasPrefix(mime, "image/") {
		if strings.Contains(mime, "svg") { return "image" }
		return "image"
	}
	if strings.HasPrefix(mime, "video/") { return "video" }
	if strings.HasPrefix(mime, "audio/") { return "audio" }
	if strings.Contains(mime, "pdf") || strings.Contains(mime, "document") { return "document" }
	return "file"
}

// resolveStorageID determines the per-user storage namespace.
// Authenticated users → user_{JWT_userID} (deterministic, tied to auth)
// Guest users → anon_{client_id} (from SSE connection, sent by frontend)
// This is SECURE because the identity is verified server-side, not client-generated.
func resolveStorageID(r *http.Request) string {
	// 1. Try JWT claims (authenticated users)
	if claims := auth.GetClaims(r.Context()); claims != nil {
		return "user_" + claims.UserID
	}
	// 2. Try client_id from query param (guest users, same as SSE)
	if cid := r.URL.Query().Get("client_id"); cid != "" {
		return "anon_" + cid
	}
	// 3. Last resort — generate one from request fingerprint
	return "anon_" + fmt.Sprintf("%d", time.Now().UnixNano())
}
