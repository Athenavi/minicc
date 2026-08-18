package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
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
	claims := auth.GetClaims(r.Context())

	parentID := r.URL.Query().Get("parent_id")
	category := r.URL.Query().Get("category")
	mediaType := r.URL.Query().Get("type")
	search := r.URL.Query().Get("search")
	tagsParam := r.URL.Query().Get("tags")
	page, pageSize := parsePagination(r.URL.Query())

	where := " WHERE tenant_id = $1 AND user_id = $2 AND parent_id = $3"
	args := []interface{}{DefaultTenantID, claims.UserID, parentID}
	argIdx := 4

	if category != "" {
		where += fmt.Sprintf(" AND category = $%d", argIdx)
		args = append(args, category)
		argIdx++
	}
	if mediaType != "" {
		where += fmt.Sprintf(" AND type = $%d", argIdx)
		args = append(args, mediaType)
		argIdx++
	}
	if search != "" {
		where += fmt.Sprintf(" AND name ILIKE $%d", argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}
	if tagsParam != "" {
		// 逗号分隔标签：匹配全部（tags @>）
		tagList := strings.Split(tagsParam, ",")
		clean := make([]string, 0, len(tagList))
		for _, t := range tagList {
			if t = strings.TrimSpace(t); t != "" {
				clean = append(clean, t)
			}
		}
		if len(clean) > 0 {
			where += fmt.Sprintf(" AND tags @> $%d::text[]", argIdx)
			args = append(args, clean)
			argIdx++
		}
	}

	var total int64
	if err := db.ReadPool().QueryRow(r.Context(), "SELECT COUNT(*) FROM media_assets"+where, args...).Scan(&total); err != nil {
		InternalError(w, "count media assets")
		return
	}

	query := `SELECT id, type, name, COALESCE(file_url, ''), COALESCE(mime_type, ''),
		COALESCE(thumbnail, ''), metadata::text, COALESCE(tags::text, ''), COALESCE(category, ''), size, created_at, updated_at
		FROM media_assets` + where +
		" ORDER BY (type = 'folder') DESC, name ASC LIMIT $%d OFFSET $%d"
	args = append(args, pageSize, (page-1)*pageSize)
	query = fmt.Sprintf(query, argIdx, argIdx+1)

	rows, err := db.ReadPool().Query(r.Context(), query, args...)
	if err != nil {
		InternalError(w, "query media assets")
		return
	}
	defer rows.Close()

	items := make([]MediaAsset, 0, pageSize)
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
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		InternalError(w, "iterate media assets")
		return
	}

	OK(w, map[string]interface{}{"items": items, "total": total, "page": page, "page_size": pageSize})
}

// ── Create (text/code content) ──

func (h *MediaHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, ErrAuthRequired)
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
		BadRequest(w, ErrInvalidReq)
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
			logAndRespond(w, err, http.StatusInternalServerError, "create asset failed")
			return
		}

		objectKey := fmt.Sprintf("media/%s/%s_%s", dir, shortAssetID(assetID), body.Name)
		if err := h.store.Write(r.Context(), objectKey, []byte(body.Content)); err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, "save file failed")
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
			logAndRespond(w, err, http.StatusInternalServerError, "create asset failed")
			return
		}
	}

	OK(w, map[string]string{"id": assetID, "name": body.Name, "type": body.Type, "file_url": fileURL})
}

// ── CreateFolder (virtual folder) ──

// CreateFolder creates a virtual folder (type='folder', no storage object).
// ListFolders 返回用户全部文件夹（含 parent_id），供移动层级树构建。
func (h *MediaHandler) ListFolders(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())

	rows, err := db.Pool.Query(r.Context(),
		`SELECT id, name, COALESCE(parent_id::text, '') FROM media_assets
		 WHERE tenant_id = $1 AND user_id = $2 AND type = 'folder'
		 ORDER BY name`, DefaultTenantID, claims.UserID)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "list folders failed")
		return
	}
	defer rows.Close()

	type folderNode struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	folders := make([]folderNode, 0, 16)
	for rows.Next() {
		var f folderNode
		if err := rows.Scan(&f.ID, &f.Name, &f.ParentID); err != nil {
			continue
		}
		folders = append(folders, f)
	}
	OK(w, folders)
}

// CreateFolder creates a folder asset.
func (h *MediaHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())

	var body struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		BadRequest(w, "name is required")
		return
	}

	// 同级重名检查
	var exists bool
	if err := db.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM media_assets WHERE tenant_id=$1 AND user_id=$2 AND parent_id=$3 AND name=$4)`,
		DefaultTenantID, claims.UserID, body.ParentID, body.Name).Scan(&exists); err != nil {
		InternalError(w, "check folder name")
		return
	}
	if exists {
		BadRequest(w, "a folder or file with this name already exists")
		return
	}

	var id string
	if err := db.Pool.QueryRow(r.Context(),
		`INSERT INTO media_assets (id, tenant_id, user_id, type, name, parent_id, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, 'folder', $3, $4, NOW(), NOW()) RETURNING id`,
		DefaultTenantID, claims.UserID, body.Name, body.ParentID).Scan(&id); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create folder failed")
		return
	}
	OK(w, map[string]string{"id": id, "name": body.Name, "type": "folder", "parent_id": body.ParentID})
}

// ── Update (rename / move) ──

// Update renames or moves a media asset (folder or file).
func (h *MediaHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())

	id := r.PathValue("id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	var body struct {
		Name     *string   `json:"name"`
		ParentID *string   `json:"parent_id"`
		Tags     *[]string `json:"tags"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if body.Name == nil && body.ParentID == nil && body.Tags == nil {
		BadRequest(w, "nothing to update")
		return
	}

	// 所有权 + 当前值
	var curName, curParent string
	if err := db.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(name,''), COALESCE(parent_id,'') FROM media_assets WHERE id=$1 AND tenant_id=$2 AND user_id=$3`,
		id, DefaultTenantID, claims.UserID).Scan(&curName, &curParent); err != nil {
		NotFound(w, "media asset not found")
		return
	}

	newName := curName
	if body.Name != nil {
		newName = strings.TrimSpace(*body.Name)
		if newName == "" {
			BadRequest(w, "name cannot be empty")
			return
		}
	}
	newParent := curParent
	if body.ParentID != nil {
		newParent = *body.ParentID
	}

	// 移动防环
	if body.ParentID != nil {
		cycle, err := wouldCreateCycle(func(pid string) (string, error) {
			var p string
			err := db.Pool.QueryRow(r.Context(),
				`SELECT COALESCE(parent_id,'') FROM media_assets WHERE id=$1 AND tenant_id=$2 AND user_id=$3`,
				pid, DefaultTenantID, claims.UserID).Scan(&p)
			if err != nil {
				return "", err
			}
			return p, nil
		}, id, newParent)
		if err != nil {
			InternalError(w, "check move target")
			return
		}
		if cycle {
			BadRequest(w, "cannot move a folder into itself or its own descendant")
			return
		}
	}

	// 重名检查（排除自身）
	if newName != curName || newParent != curParent {
		var exists bool
		if err := db.Pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM media_assets WHERE tenant_id=$1 AND user_id=$2 AND parent_id=$3 AND name=$4 AND id<>$5)`,
			DefaultTenantID, claims.UserID, newParent, newName, id).Scan(&exists); err != nil {
			InternalError(w, "check name conflict")
			return
		}
		if exists {
			BadRequest(w, "a folder or file with this name already exists")
			return
		}
	}

	if _, err := db.Pool.Exec(r.Context(),
		`UPDATE media_assets SET name=$1, parent_id=$2, updated_at=NOW() WHERE id=$3 AND tenant_id=$4 AND user_id=$5`,
		newName, newParent, id, DefaultTenantID, claims.UserID); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "update media asset failed")
		return
	}
	// 标签独立更新（不影响名称/父目录检查）
	if body.Tags != nil {
		if _, err := db.Pool.Exec(r.Context(),
			`UPDATE media_assets SET tags=$1, updated_at=NOW() WHERE id=$2 AND tenant_id=$3 AND user_id=$4`,
			*body.Tags, id, DefaultTenantID, claims.UserID); err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, "update media tags failed")
			return
		}
	}
	OK(w, map[string]string{"id": id, "name": newName, "parent_id": newParent})
}

// assetObjectKey resolves the storage object key for an asset, preferring
// the persisted file_path column; falls back to legacy name-based key.
func (h *MediaHandler) assetObjectKey(r *http.Request, id string) (string, error) {
	var fileName, filePath, userID string
	err := db.ReadPool().QueryRow(r.Context(),
		`SELECT COALESCE(name,''), COALESCE(file_path,''), COALESCE(user_id,'') FROM media_assets WHERE id=$1`,
		id).Scan(&fileName, &filePath, &userID)
	if err != nil {
		return "", err
	}
	if filePath != "" {
		return filePath, nil
	}
	dir := "anonymous"
	if userID != "" {
		dir = "u_" + userID
	}
	return fmt.Sprintf("media/%s/%s_%s", dir, shortAssetID(id), fileName), nil
}

// ── Delete (recursive for folders) ──

func (h *MediaHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}

	ctx := r.Context()

	id := r.PathValue("id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	// 查询资产并校验所有权
	if err := db.ReadPool().QueryRow(ctx,
		`SELECT 1 FROM media_assets WHERE id = $1 AND tenant_id = $2 AND user_id = $3`,
		id, DefaultTenantID, claims.UserID,
	).Scan(new(int)); err != nil {
		NotFound(w, "media asset not found")
		return
	}

	ids, err := collectFolderIDs(func(parent string) ([]string, error) {
		return h.getChildren(ctx, claims.UserID, parent)
	}, id)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "collect folder tree failed")
		return
	}

	// 删除存储对象（DB 为准，失败仅记日志）
	for _, aid := range ids {
		if key, err := h.assetObjectKey(r, aid); err == nil && key != "" {
			if err := h.store.Delete(ctx, key); err != nil {
				slog.Warn("failed to delete media object", "key", key, "error", err)
			}
		}
	}

	if _, err := db.Pool.Exec(ctx,
		`DELETE FROM media_assets WHERE id = ANY($1) AND tenant_id=$2 AND user_id=$3`,
		ids, DefaultTenantID, claims.UserID); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "delete media asset failed")
		return
	}

	OK(w, map[string]interface{}{"status": "deleted", "deleted": len(ids)})
}

// ── BatchDelete ──

// BatchDelete deletes multiple assets, folders recursively.
func (h *MediaHandler) BatchDelete(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if len(body.IDs) == 0 {
		BadRequest(w, "ids is required")
		return
	}

	allIDs := make([]string, 0, len(body.IDs))
	for _, id := range body.IDs {
		sub, err := collectFolderIDs(func(parent string) ([]string, error) {
			return h.getChildren(r.Context(), claims.UserID, parent)
		}, id)
		if err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, "collect folder tree failed")
			return
		}
		allIDs = append(allIDs, sub...)
	}

	// 删除存储对象（DB 为准，失败仅记日志）
	for _, id := range allIDs {
		if key, err := h.assetObjectKey(r, id); err == nil && key != "" {
			if err := h.store.Delete(r.Context(), key); err != nil {
				slog.Warn("failed to delete media object", "key", key, "error", err)
			}
		}
	}

	if _, err := db.Pool.Exec(r.Context(),
		`DELETE FROM media_assets WHERE id = ANY($1) AND tenant_id=$2 AND user_id=$3`,
		allIDs, DefaultTenantID, claims.UserID); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "batch delete media assets failed")
		return
	}
	OK(w, map[string]interface{}{"deleted": len(allIDs)})
}

// ── Shared helpers ──

// getChildren returns the child asset IDs for a given parent folder.
func (h *MediaHandler) getChildren(ctx context.Context, userID, parentID string) ([]string, error) {
	rows, err := db.ReadPool().Query(ctx,
		`SELECT id FROM media_assets WHERE tenant_id=$1 AND user_id=$2 AND parent_id=$3`,
		DefaultTenantID, userID, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err != nil {
			return nil, err
		}
		ids = append(ids, cid)
	}
	return ids, rows.Err()
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
	if claims := auth.GetClaims(r.Context()); claims != nil {
		return "u_" + claims.UserID
	}
	return "anonymous"
}

// shortAssetID returns the stable object-key prefix for an asset ID.
// IDs shorter than 8 chars (e.g. snowflake IDs produced during clock
// regression, or client-supplied IDs via CompleteUpload) are returned
// in full to avoid a slice-bounds panic.
func shortAssetID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
