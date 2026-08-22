package api

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/id"
)

// UploadHandler 提供通用分片上传（断点续传）：
//   - Init        POST   /v1/uploads            → upload_id / chunk_size / chunk_count
//   - PutChunk    PUT    /v1/uploads/{id}/chunks/{index}
//   - GetProgress GET    /v1/uploads/{id}       → received_chunks（断点续传依据）
//   - Complete    POST   /v1/uploads/{id}/complete → 合并并按 purpose 落库
//
// 分片存于 <storageRoot>/uploads/{upload_id}/chunk_{index}；小文件可走既有 multipart 直传。
type UploadHandler struct {
	authenticator *auth.Authenticator
	storageRoot   string
}

func NewUploadHandler(a *auth.Authenticator, storageRoot string) *UploadHandler {
	return &UploadHandler{authenticator: a, storageRoot: storageRoot}
}

const defaultChunkSize = 2 << 20 // 2MB

// maxKBDocSize 限制 kb_doc 文档大小（防 finalizeKBDoc 整文件读入内存导致 OOM）。
const maxKBDocSize int64 = 64 << 20 // 64MB

// validUploadNameRe 用于文件名净化：拒绝路径分隔符与目录穿越序列。
// 仅允许字母数字、中文等常规字符、点、下划线、连字符、空格。
var validUploadNameRe = regexp.MustCompile(`^[^\x00-\x1f/\\]+$`)

// sanitizeUploadName 净化上传文件名（P0-S3 路径穿越修复）：
// 拒绝包含路径分隔符或空白的名字；剥离潜在遍历；限制长度。
func sanitizeUploadName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 {
		return ""
	}
	if !validUploadNameRe.MatchString(name) {
		return ""
	}
	if name == "." || name == ".." {
		return ""
	}
	// 防御：仅保留文件名部分（杜绝任何残余分隔符场景）
	name = filepath.Base(filepath.Clean(name))
	if name == "." || name == ".." {
		return ""
	}
	return name
}

// chunkDir 返回 upload_id 的临时分片目录（自动创建）。
func (h *UploadHandler) chunkDir(uploadID string) (string, error) {
	dir := filepath.Join(h.storageRoot, "uploads", uploadID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func (h *UploadHandler) userID(r *http.Request) (string, bool) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		return "", false
	}
	return claims.UserID, true
}

// claimsOf 返回当前请求的 claims（含 tenant_id 与 user_id）。
func (h *UploadHandler) claimsOf(r *http.Request) (*auth.Claims, bool) {
	c := auth.GetClaims(r.Context())
	if c == nil || c.TenantID == "" {
		return nil, false
	}
	return c, true
}

// ── Init ────────────────────────────────────────────────────────────

func (h *UploadHandler) Init(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.claimsOf(r)
	if !ok {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	var body struct {
		Name      string `json:"name"`
		Size      int64  `json:"size"`
		MimeType  string `json:"mime_type"`
		Purpose   string `json:"purpose"`   // media / kb_doc / generic
		ParentID  string `json:"parent_id"` // media 文件夹 id；kb_doc 时传 kb_id
		Category  string `json:"category"`
		ChunkSize int    `json:"chunk_size"` // 可选，默认 2MB
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || body.Size <= 0 {
		BadRequest(w, "name and size are required")
		return
	}
	// P0-S3 路径穿越修复：文件名必须净化，拒绝 / \ .. 等
	body.Name = sanitizeUploadName(body.Name)
	if body.Name == "" {
		BadRequest(w, "invalid file name")
		return
	}
	// P0 存储型 XSS 防护（分片路径补齐）：可执行/脚本 MIME 与 html/xml 类扩展名一律拒绝
	// （直传路径已有 isExecutableMIME；此处覆盖分片上传，避免 .html 被按 text/html 同源输出）
	if isExecutableMIME(body.MimeType, body.Name) {
		BadRequest(w, "file type not allowed")
		return
	}
	if ext := strings.ToLower(filepath.Ext(body.Name)); ext == ".html" || ext == ".htm" ||
		ext == ".xml" || ext == ".xhtml" || ext == ".swf" {
		BadRequest(w, "file type not allowed: " + ext)
		return
	}
	if body.Purpose == "" {
		body.Purpose = "generic"
	}
	if body.Purpose != "media" && body.Purpose != "kb_doc" && body.Purpose != "generic" {
		BadRequest(w, "purpose must be media / kb_doc / generic")
		return
	}
	// P0-P3 防护：kb_doc 会整文件读入内存（知识文档 content 列），限制大小
	if body.Purpose == "kb_doc" && body.Size > maxKBDocSize {
		BadRequest(w, "kb_doc upload too large (max 64MB)")
		return
	}
	chunkSize := body.ChunkSize
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	if chunkSize > 64<<20 {
		chunkSize = 64 << 20
	}
	chunkCount := int((body.Size + int64(chunkSize) - 1) / int64(chunkSize))
	if chunkCount <= 0 {
		chunkCount = 1
	}

	uploadID, err := id.UUID()
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "generate id failed")
		return
	}
	now := time.Now()
	if _, err := db.Pool.Exec(r.Context(),
		`INSERT INTO uploads (id, tenant_id, user_id, name, size, mime_type, purpose, parent_id, category, chunk_size, chunk_count, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'uploading', $12, $12)`,
		uploadID, claims.TenantID, claims.UserID, body.Name, body.Size, truncateMIME(body.MimeType), body.Purpose,
		body.ParentID, body.Category, chunkSize, chunkCount, now); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "init upload failed")
		return
	}

	OK(w, map[string]interface{}{
		"upload_id":   uploadID,
		"chunk_size":  chunkSize,
		"chunk_count": chunkCount,
	})
}

// ── PutChunk ────────────────────────────────────────────────────────

func (h *UploadHandler) PutChunk(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.claimsOf(r)
	if !ok {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	uploadID := r.PathValue("id")
	idxStr := r.PathValue("index")
	idx, err := strconv.Atoi(idxStr)
	if err != nil || idx < 0 {
		BadRequest(w, "invalid chunk index")
		return
	}

	// 归属校验（含 tenant_id，防跨租户读写分片）
	var owner string
	if err := db.Pool.QueryRow(r.Context(),
		`SELECT user_id FROM uploads WHERE id = $1 AND tenant_id = $2`, uploadID, claims.TenantID).Scan(&owner); err != nil || owner != claims.UserID {
		NotFound(w, "upload not found")
		return
	}

	// 限制单分片大小，防止恶意客户端发送超大 chunk 撑爆磁盘（P1-1）
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	dir, err := h.chunkDir(uploadID)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create chunk dir failed")
		return
	}
	dst := filepath.Join(dir, fmt.Sprintf("chunk_%d", idx))
	out, err := os.Create(dst)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create chunk failed")
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, r.Body); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "write chunk failed")
		return
	}

	if _, err := db.Pool.Exec(r.Context(),
		`UPDATE uploads SET chunks_received = array_append(chunks_received, $1), updated_at = NOW()
		 WHERE id = $2 AND tenant_id = $3 AND NOT ($1 = ANY(chunks_received))`, idxStr, uploadID, claims.TenantID); err != nil {
		slog.Warn("failed to record upload", "error", err)
	}
	OK(w, map[string]interface{}{"upload_id": uploadID, "index": idx, "received": true})
}

// ── GetProgress ─────────────────────────────────────────────────────

func (h *UploadHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.claimsOf(r)
	if !ok {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	uploadID := r.PathValue("id")
	var received []string
	var status string
	err := db.Pool.QueryRow(r.Context(),
		`SELECT chunks_received, status FROM uploads WHERE id = $1 AND tenant_id = $2 AND user_id = $3`, uploadID, claims.TenantID, claims.UserID).
		Scan(&received, &status)
	if err != nil {
		NotFound(w, "upload not found")
		return
	}
	chunks := make([]int, 0, len(received))
	for _, s := range received {
		if n, err := strconv.Atoi(s); err == nil {
			chunks = append(chunks, n)
		}
	}
	sort.Ints(chunks)
	OK(w, map[string]interface{}{
		"upload_id":       uploadID,
		"received_chunks": chunks,
		"status":          status,
	})
}

// ── Complete ────────────────────────────────────────────────────────

func (h *UploadHandler) Complete(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.claimsOf(r)
	if !ok {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	uploadID := r.PathValue("id")

	var up struct {
		ID        string
		UserID    string
		Name      string
		Size      int64
		MimeType  string
		Purpose   string
		ParentID  string
		Category  string
		ChunkSize int
		ChunkCnt  int
		Received  []string
	}
	err := db.Pool.QueryRow(r.Context(),
		`SELECT id, user_id, name, size, mime_type, purpose, parent_id, category, chunk_size, chunk_count, chunks_received
		 FROM uploads WHERE id = $1 AND tenant_id = $2 AND user_id = $3`, uploadID, claims.TenantID, claims.UserID).
		Scan(&up.ID, &up.UserID, &up.Name, &up.Size, &up.MimeType, &up.Purpose,
			&up.ParentID, &up.Category, &up.ChunkSize, &up.ChunkCnt, &up.Received)
	if err != nil {
		NotFound(w, "upload not found")
		return
	}
	if len(up.Received) != up.ChunkCnt {
		BadRequest(w, fmt.Sprintf("incomplete upload: %d/%d chunks received", len(up.Received), up.ChunkCnt))
		return
	}

	// 按序合并分片
	dir, err := h.chunkDir(uploadID)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "read chunk dir failed")
		return
	}
	merged, err := h.mergeChunks(dir, up.ChunkCnt)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "merge chunks failed")
		return
	}
	defer merged.Close()

	// 校验总大小
	fi, err := merged.Stat()
	if err == nil && fi.Size() != up.Size {
		BadRequest(w, fmt.Sprintf("size mismatch: got %d want %d", fi.Size(), up.Size))
		return
	}

	var fileURL string
	switch up.Purpose {
	case "media":
		fileURL, err = h.finalizeMedia(r, claims.TenantID, up, merged)
	case "kb_doc":
		fileURL, err = h.finalizeKBDoc(r, claims.TenantID, up, merged)
	default:
		fileURL, err = h.finalizeGeneric(up, merged)
	}
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "finalize upload failed")
		return
	}

	if _, err := db.Pool.Exec(r.Context(),
		`UPDATE uploads SET status = 'completed', updated_at = NOW() WHERE id = $1 AND tenant_id = $2`, uploadID, claims.TenantID); err != nil {
		slog.Warn("failed to record upload", "error", err)
	}
	// 清理临时分片
	_ = os.RemoveAll(dir)

	OK(w, map[string]interface{}{
		"upload_id": uploadID, "file_url": fileURL,
		"purpose": up.Purpose, "name": up.Name, "size": up.Size,
	})
}

// mergeChunks 按 index 顺序拼接分片为单个临时文件。
func (h *UploadHandler) mergeChunks(dir string, count int) (*os.File, error) {
	merged, err := os.CreateTemp(h.storageRoot, "merged_*.tmp")
	if err != nil {
		return nil, err
	}
	for i := 0; i < count; i++ {
		part, err := os.Open(filepath.Join(dir, fmt.Sprintf("chunk_%d", i)))
		if err != nil {
			merged.Close()
			os.Remove(merged.Name())
			return nil, fmt.Errorf("open chunk %d: %w", i, err)
		}
		_, err = io.Copy(merged, part)
		part.Close()
		if err != nil {
			merged.Close()
			os.Remove(merged.Name())
			return nil, err
		}
	}
	if _, err := merged.Seek(0, io.SeekStart); err != nil {
		merged.Close()
		os.Remove(merged.Name())
		return nil, err
	}
	return merged, nil
}

// finalizeMedia 合并文件写入 media 存储区并落 media_assets（按当前租户）。
func (h *UploadHandler) finalizeMedia(r *http.Request, tenantID string, up struct {
	ID        string
	UserID    string
	Name      string
	Size      int64
	MimeType  string
	Purpose   string
	ParentID  string
	Category  string
	ChunkSize int
	ChunkCnt  int
	Received  []string
}, merged *os.File) (string, error) {
	dir := "u_" + up.UserID
	name := sanitizeUploadName(up.Name)
	if name == "" {
		return "", fmt.Errorf("invalid upload name")
	}
	objectKey := fmt.Sprintf("media/%s/%s_%s", dir, shortAssetID(up.ID), name)
	dest := filepath.Join(h.storageRoot, objectKey)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, merged); err != nil {
		return "", err
	}

	assetID, err := id.UUID()
	if err != nil {
		return "", err
	}
	assetType := detectType(up.MimeType)
	if _, err := db.Pool.Exec(r.Context(),
		`INSERT INTO media_assets (id, tenant_id, user_id, type, name, file_url, mime_type, category, size, parent_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())`,
		assetID, tenantID, up.UserID, assetType, up.Name, "/"+objectKey,
		truncateMIME(up.MimeType), nullableStr(up.Category), up.Size, up.ParentID); err != nil {
		return "", err
	}
	return "/" + objectKey, nil
}

// finalizeKBDoc 合并文件内容落 knowledge_documents（content bytea 供 RAG 构建，按当前租户）。
func (h *UploadHandler) finalizeKBDoc(r *http.Request, tenantID string, up struct {
	ID        string
	UserID    string
	Name      string
	Size      int64
	MimeType  string
	Purpose   string
	ParentID  string
	Category  string
	ChunkSize int
	ChunkCnt  int
	Received  []string
}, merged *os.File) (string, error) {
	if up.ParentID == "" {
		return "", fmt.Errorf("kb_doc upload requires parent_id (kb_id)")
	}
	// 校验 KB 存在且归属当前租户当前用户
	var owner string
	if err := db.Pool.QueryRow(r.Context(),
		`SELECT user_id FROM knowledge_bases WHERE id = $1 AND tenant_id = $2`, up.ParentID, tenantID).Scan(&owner); err != nil || owner != up.UserID {
		return "", fmt.Errorf("knowledge base not found or not owned")
	}
	content, err := io.ReadAll(io.LimitReader(merged, maxKBDocSize+1))
	if err != nil {
		return "", err
	}
	if int64(len(content)) > maxKBDocSize {
		return "", fmt.Errorf("kb_doc content exceeds 64MB limit")
	}
	ext := strings.TrimPrefix(filepath.Ext(up.Name), ".")
	if ext == "" {
		ext = "txt"
	}
	docID, err := id.UUID()
	if err != nil {
		return "", err
	}
	_, err = db.Pool.Exec(r.Context(),
		`INSERT INTO knowledge_documents (id, tenant_id, knowledge_base_id, user_id, name, file_type, file_size_bytes, status, content, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', $8, NOW(), NOW())`,
		docID, tenantID, up.ParentID, up.UserID, up.Name, ext, up.Size, content)
	if err != nil {
		return "", err
	}
	// 重算 KB 统计（带 tenant_id 限制）
	_, _ = db.Pool.Exec(r.Context(),
		`UPDATE knowledge_bases
		 SET document_count = (SELECT COUNT(*) FROM knowledge_documents WHERE knowledge_base_id = $1 AND tenant_id = $2),
		     total_size_bytes = COALESCE((SELECT SUM(file_size_bytes) FROM knowledge_documents WHERE knowledge_base_id = $1 AND tenant_id = $2), 0),
		     updated_at = NOW() WHERE id = $1 AND tenant_id = $2`, up.ParentID, tenantID)
	return fmt.Sprintf("/kb/%s/documents/%s", up.ParentID, docID), nil
}

// finalizeGeneric 合并文件写入通用目录并返回可访问 URL。
func (h *UploadHandler) finalizeGeneric(up struct {
	ID        string
	UserID    string
	Name      string
	Size      int64
	MimeType  string
	Purpose   string
	ParentID  string
	Category  string
	ChunkSize int
	ChunkCnt  int
	Received  []string
}, merged *os.File) (string, error) {
	name := sanitizeUploadName(up.Name)
	if name == "" {
		return "", fmt.Errorf("invalid upload name")
	}
	objectKey := fmt.Sprintf("uploads/final/%s_%s", shortAssetID(up.ID), name)
	dest := filepath.Join(h.storageRoot, objectKey)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, merged); err != nil {
		return "", err
	}
	return "/" + objectKey, nil
}

// ── 路由注册 ───────────────────────────────────────────────────────

func (h *UploadHandler) RegisterRoutes(mux *http.ServeMux, authMW func(http.Handler) http.Handler, rlMW func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/uploads", authMW(rlMW(http.HandlerFunc(h.Init))))
	mux.Handle("PUT /v1/uploads/{id}/chunks/{index}", authMW(rlMW(http.HandlerFunc(h.PutChunk))))
	mux.Handle("GET /v1/uploads/{id}", authMW(rlMW(http.HandlerFunc(h.GetProgress))))
	mux.Handle("POST /v1/uploads/{id}/complete", authMW(rlMW(http.HandlerFunc(h.Complete))))
}
