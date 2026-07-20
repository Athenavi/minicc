package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/id"
	"github.com/athenavi/minicc/internal/storage"
)

// MediaHandler manages media assets stored in object storage (S3/MinIO).
// Database stores only metadata — the actual file content lives in S3.
type MediaHandler struct {
	store         storage.FileStore
	authenticator *auth.Authenticator
}

func NewMediaHandler(store storage.FileStore, authenticator *auth.Authenticator) *MediaHandler {
	return &MediaHandler{store: store, authenticator: authenticator}
}

// ── Types ──

type MediaAsset struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Name      string                 `json:"name"`
	FileURL   string                 `json:"file_url"`
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
	claims := getAuthClaims(r, h.authenticator)
	if claims == nil {
		Unauthorized(w, "authentication required")
		return
	}

	if db.Pool == nil {
		OK(w, []MediaAsset{})
		return
	}

	category := r.URL.Query().Get("category")
	mediaType := r.URL.Query().Get("type")
	search := r.URL.Query().Get("search")

	query := `SELECT id, type, name, COALESCE(file_url, ''), COALESCE(mime_type, ''),
		COALESCE(thumbnail, ''), metadata::text, COALESCE(tags::text, ''), COALESCE(category, ''), size, created_at, updated_at
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
		query += fmt.Sprintf(" AND name ILIKE $%d", argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}

	query += " ORDER BY created_at DESC LIMIT 100"

	rows, err := db.ReadPool().Query(r.Context(), query, args...)
	if err != nil {
		OK(w, []MediaAsset{})
		return
	}
	defer rows.Close()

	assets := make([]MediaAsset, 0)
	for rows.Next() {
		var a MediaAsset
		var metadataJSON, tagsJSON string
		if err := rows.Scan(&a.ID, &a.Type, &a.Name, &a.FileURL, &a.MimeType,
			&a.Thumbnail, &metadataJSON, &tagsJSON, &a.Category, &a.Size, &a.CreatedAt, &a.UpdatedAt); err != nil {
			continue
		}
		if metadataJSON != "" && metadataJSON != "{}" {
			json.Unmarshal([]byte(metadataJSON), &a.Metadata)
		}
		if tagsJSON != "" {
			t := strings.Trim(tagsJSON, "{}")
			if t != "" {
				a.Tags = strings.Split(t, ",")
			}
		}
		assets = append(assets, a)
	}
	if err := rows.Err(); err != nil {
		InternalError(w, "failed to iterate assets")
		return
	}

	OK(w, assets)
}

// ── Create (text/code content) ──

func (h *MediaHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := getAuthClaims(r, h.authenticator)
	if claims == nil {
		Unauthorized(w, "authentication required")
		return
	}

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

	// 使用 PostgreSQL 的 gen_random_uuid() 生成 UUID
	var assetID string
	dir := h.resolveDir(r)
	fileURL := ""

	if body.Content != "" {
		// 先插入数据库获取 UUID
		metadataJSON, _ := json.Marshal(body.Metadata)
		tagsJSON := "{}"
		if len(body.Tags) > 0 {
			tagsJSON = "{" + strings.Join(body.Tags, ",") + "}"
		}

		err := db.Pool.QueryRow(r.Context(),
			`INSERT INTO media_assets (id, tenant_id, user_id, type, name, file_url, category, tags, metadata, size, created_at, updated_at)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, '', $5, $6, $7, $8, NOW(), NOW())
			 RETURNING id`,
			DefaultTenantID, claims.UserID, body.Type, body.Name, nullableStr(body.Category), tagsJSON, string(metadataJSON), len(body.Content),
		).Scan(&assetID)
		if err != nil {
			InternalError(w, "create asset: "+err.Error())
			return
		}

		objectKey := fmt.Sprintf("media/%s/%s_%s", dir, assetID[:8], body.Name)
		if err := h.store.Write(r.Context(), objectKey, []byte(body.Content)); err != nil {
			InternalError(w, "save file: "+err.Error())
			return
		}
		fileURL = h.objectURL(objectKey)

		// 更新 file_url
		_, err = db.Pool.Exec(r.Context(),
			`UPDATE media_assets SET file_url = $1 WHERE id = $2`,
			fileURL, assetID)
		if err != nil {
			slog.Warn("update file_url", "error", err)
		}
	} else {
		metadataJSON, _ := json.Marshal(body.Metadata)
		tagsJSON := "{}"
		if len(body.Tags) > 0 {
			tagsJSON = "{" + strings.Join(body.Tags, ",") + "}"
		}

		err := db.Pool.QueryRow(r.Context(),
			`INSERT INTO media_assets (id, tenant_id, user_id, type, name, file_url, category, tags, metadata, size, created_at, updated_at)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, '', $5, $6, $7, $8, NOW(), NOW())
			 RETURNING id`,
			DefaultTenantID, claims.UserID, body.Type, body.Name, nullableStr(body.Category), tagsJSON, string(metadataJSON), 0,
		).Scan(&assetID)
		if err != nil {
			InternalError(w, "create asset: "+err.Error())
			return
		}
	}

	OK(w, map[string]string{"id": assetID, "name": body.Name, "type": body.Type, "file_url": fileURL})
}

// ── Upload (multipart file) ──

func (h *MediaHandler) Upload(w http.ResponseWriter, r *http.Request) {
	claims := getAuthClaims(r, h.authenticator)
	if claims == nil {
		Unauthorized(w, "authentication required")
		return
	}

	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}

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

	fileData, err := io.ReadAll(file)
	if err != nil {
		InternalError(w, "read file: "+err.Error())
		return
	}
	fileSize := int64(len(fileData))

	mimeType := truncateMIME(header.Header.Get("Content-Type"))
	assetType := detectType(mimeType)
	category := r.FormValue("category")

	dir := h.resolveDir(r)
	name := r.FormValue("name")
	if name == "" {
		name = header.Filename
	}

	// 先插入数据库获取 UUID
	var assetID string
	err = db.Pool.QueryRow(r.Context(),
		`INSERT INTO media_assets (id, tenant_id, user_id, type, name, file_url, mime_type, category, size, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, '', $5, $6, $7, NOW(), NOW())
		 RETURNING id`,
		DefaultTenantID, claims.UserID, assetType, name, mimeType, nullableStr(category), fileSize,
	).Scan(&assetID)
	if err != nil {
		InternalError(w, "create asset: "+err.Error())
		return
	}

	objectKey := fmt.Sprintf("media/%s/%s_%s", dir, assetID[:8], name)
	if err := h.store.Write(r.Context(), objectKey, fileData); err != nil {
		InternalError(w, "save file: "+err.Error())
		return
	}
	fileURL := h.objectURL(objectKey)

	// 更新 file_url
	_, err = db.Pool.Exec(r.Context(),
		`UPDATE media_assets SET file_url = $1 WHERE id = $2`,
		fileURL, assetID)
	if err != nil {
		slog.Warn("update file_url", "error", err)
	}

	OK(w, map[string]string{
		"id": assetID, "name": name, "type": assetType,
		"file_url": fileURL, "size": fmt.Sprintf("%d", fileSize),
	})
}

// ── PresignUpload — returns a presigned URL for client-side direct upload ──

func (h *MediaHandler) PresignUpload(w http.ResponseWriter, r *http.Request) {
	claims := getAuthClaims(r, h.authenticator)
	if claims == nil {
		Unauthorized(w, "authentication required")
		return
	}

	var body struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Category string `json:"category"`
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
		body.Type = "file"
	}
	if body.Category == "" {
		body.Category = "upload"
	}

	// Type-assert to S3Store for presigned URL support (unwrap AtomicStore first)
	inner := h.store
	if atomic, ok := h.store.(*storage.AtomicStore); ok {
		inner = atomic.LoadRaw()
	}
	s3store, ok := inner.(*storage.S3Store)
	if !ok {
		InternalError(w, "presigned upload requires S3 storage backend")
		return
	}

	assetID := id.NextID()
	dir := h.resolveDir(r)
	objectKey := fmt.Sprintf("media/%s/%s_%s", dir, assetID[:8], body.Name)
	fileURL := s3store.ObjectURL(objectKey)

	presignedURL, err := s3store.PresignedPutURL(r.Context(), objectKey, 15*time.Minute)
	if err != nil {
		InternalError(w, "generate presigned url: "+err.Error())
		return
	}

	OK(w, map[string]string{
		"id":            assetID,
		"name":          body.Name,
		"type":          body.Type,
		"category":      body.Category,
		"file_url":      fileURL,
		"presigned_url": presignedURL,
		"expires_in":    "900",
	})
}

// ── CompleteUpload — called by client after presigned upload is done ──

func (h *MediaHandler) CompleteUpload(w http.ResponseWriter, r *http.Request) {
	claims := getAuthClaims(r, h.authenticator)
	if claims == nil {
		Unauthorized(w, "authentication required")
		return
	}

	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}

	var body struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Type     string `json:"type"`
		FileURL  string `json:"file_url"`
		Category string `json:"category"`
		Size     int64  `json:"size"`
		MimeType string `json:"mime_type"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}
	if body.ID == "" || body.FileURL == "" {
		BadRequest(w, "id and file_url are required")
		return
	}
	if body.Type == "" {
		body.Type = "file"
	}

	_, err := db.Pool.Exec(r.Context(),
		`INSERT INTO media_assets (id, tenant_id, user_id, type, name, file_url, mime_type, category, size, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())`,
		body.ID, DefaultTenantID, claims.UserID, body.Type, body.Name, body.FileURL, truncateMIME(body.MimeType), nullableStr(body.Category), body.Size)
	if err != nil {
		InternalError(w, "create asset: "+err.Error())
		return
	}

	OK(w, map[string]string{"id": body.ID, "name": body.Name, "file_url": body.FileURL})
}

// ── Delete ──

func (h *MediaHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := getAuthClaims(r, h.authenticator)
	if claims == nil {
		Unauthorized(w, "authentication required")
		return
	}

	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}

	ctx := r.Context()

	id := r.PathValue("id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	// 查询资产并校验所有权
	var fileName, assetUserID string
	if err := db.ReadPool().QueryRow(ctx,
		`SELECT COALESCE(name, ''), COALESCE(user_id, '') FROM media_assets WHERE id = $1 AND tenant_id = $2 AND user_id = $3`,
		id, DefaultTenantID, claims.UserID,
	).Scan(&fileName, &assetUserID); err != nil {
		NotFound(w, "media asset not found")
		return
	}

	if _, err := db.Pool.Exec(ctx, `DELETE FROM media_assets WHERE id = $1 AND tenant_id = $2 AND user_id = $3`, id, DefaultTenantID, claims.UserID); err != nil {
		InternalError(w, "delete media asset: "+err.Error())
		return
	}

	userID := assetUserID

	// Delete from object store — reconstruct key from stored data
	dir := "anonymous"
	if userID != "" {
		dir = "u_" + userID
	}
	objectKey := fmt.Sprintf("media/%s/%s_%s", dir, id[:8], fileName)
	if err := h.store.Delete(ctx, objectKey); err != nil {
		slog.Warn("failed to delete media file from store", "key", objectKey, "error", err)
	}

	OK(w, map[string]string{"status": "deleted"})
}

// ── Helpers ──

func detectType(mime string) string {
	if strings.HasPrefix(mime, "image/") {
		return "image"
	}
	if strings.HasPrefix(mime, "video/") {
		return "video"
	}
	if strings.HasPrefix(mime, "audio/") {
		return "audio"
	}
	if strings.Contains(mime, "pdf") || strings.Contains(mime, "document") {
		return "document"
	}
	return "file"
}

// objectURL constructs the public URL for an object.
func (h *MediaHandler) objectURL(objectKey string) string {
	inner := h.store
	if atomic, ok := h.store.(*storage.AtomicStore); ok {
		inner = atomic.LoadRaw()
	}
	if s3store, ok := inner.(*storage.S3Store); ok {
		return s3store.ObjectURL(objectKey)
	}
	return "/" + objectKey
}

// resolveDir returns a stable per-user directory name for object storage.
func (h *MediaHandler) resolveDir(r *http.Request) string {
	if claims := getAuthClaims(r, h.authenticator); claims != nil {
		return "u_" + claims.UserID
	}
	return "anonymous"
}

// truncateMIME ensures MIME type fits in the VARCHAR(64) column.
func truncateMIME(mime string) string {
	if len(mime) > 64 {
		return mime[:64]
	}
	return mime
}
