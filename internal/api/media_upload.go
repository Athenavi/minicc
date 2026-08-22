package api

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/id"
	"github.com/athenavi/minicc/internal/storage"
)

// ── Upload (multipart file) ──

func (h *MediaHandler) Upload(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	tenantID := claims.TenantID

	r.Body = http.MaxBytesReader(w, r.Body, 200<<20) // 200MB 上传上限
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		// 暴露真实原因（请求体超限 / boundary 缺失 / 格式损坏），便于定位
		BadRequest(w, "file too large or invalid form: "+err.Error())
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
		logAndRespond(w, err, http.StatusInternalServerError, "read file failed")
		return
	}
	fileSize := int64(len(fileData))

	// P1-6: 用 magic bytes 检测真实 MIME，避免客户端伪造 Content-Type 上传可执行文件
	declaredMIME := truncateMIME(header.Header.Get("Content-Type"))
	detectedMIME := truncateMIME(http.DetectContentType(fileData))
	// 优先采用检测到的 MIME；声明与检测不一致时以检测为准（更安全）
	mimeType := detectedMIME
	if declaredMIME != "" && declaredMIME == detectedMIME {
		// 声明与检测一致，保留声明的（可能更精确，如 image/png vs image/jpeg）
		mimeType = declaredMIME
	}
	// 拒绝可执行/脚本类 MIME（即使客户端伪装为图片）
	// P1-6: 传入文件名以便扩展名兜底（PE/ELF magic bytes 只检测为 octet-stream）
	if isExecutableMIME(mimeType, header.Filename) {
		BadRequest(w, "file type not allowed: "+mimeType)
		return
	}
	assetType := detectType(mimeType)
	category := r.FormValue("category")
	parentID := r.FormValue("parent_id") // 当前目录；空 = 根目录

	dir := h.resolveDir(r)
	name := r.FormValue("name")
	if name == "" {
		name = header.Filename
	}

	// 先插入数据库获取 UUID（含 parent_id，子文件夹上传不落到根目录）
	var assetID string
	err = db.Pool.QueryRow(r.Context(),
		`INSERT INTO media_assets (id, tenant_id, user_id, type, name, file_url, mime_type, category, size, parent_id, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, '', $5, $6, $7, $8, NOW(), NOW())
		 RETURNING id`,
		tenantID, claims.UserID, assetType, name, mimeType, nullableStr(category), fileSize, parentID,
	).Scan(&assetID)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create asset failed")
		return
	}

	objectKey := fmt.Sprintf("media/%s/%s_%s", dir, shortAssetID(assetID), name)
	if err := h.store.Write(r.Context(), objectKey, fileData); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "save file failed")
		return
	}
	fileURL := h.objectURL(objectKey)

	// 更新 file_url
	_, err = db.Pool.Exec(r.Context(),
		`UPDATE media_assets SET file_url = $1 WHERE id = $2 AND tenant_id = $3`,
		fileURL, assetID, tenantID)
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
	var body struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Category string `json:"category"`
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
	objectKey := fmt.Sprintf("media/%s/%s_%s", dir, shortAssetID(assetID), body.Name)
	fileURL := s3store.ObjectURL(objectKey)

	presignedURL, err := s3store.PresignedPutURL(r.Context(), objectKey, 15*time.Minute)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "generate presigned url failed")
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
	claims := auth.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	tenantID := claims.TenantID

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
		BadRequest(w, ErrInvalidReq)
		return
	}
	if body.FileURL == "" {
		BadRequest(w, "file_url is required")
		return
	}
	if body.Type == "" {
		body.Type = "file"
	}

	// 校验 file_url 指向配置的存储后端（防止客户端伪造任意 URL 入库）
	inner := h.store
	if atomic, ok := h.store.(*storage.AtomicStore); ok {
		inner = atomic.LoadRaw()
	}
	switch s := inner.(type) {
	case *storage.S3Store:
		if !strings.HasPrefix(body.FileURL, s.ObjectURL("")) {
			BadRequest(w, "file_url does not match configured storage backend")
			return
		}
	case *storage.LocalStore:
		// Reject protocol-relative URLs and require /media/ prefix
		if !strings.HasPrefix(body.FileURL, "/media/") || strings.HasPrefix(body.FileURL, "//") {
			BadRequest(w, "file_url does not match configured storage backend")
			return
		}
	default:
		BadRequest(w, "unsupported storage backend")
		return
	}

	// ID 由服务端生成，不信任客户端传入的 body.ID
	assetID, err := id.UUID()
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "generate id failed")
		return
	}
	_, err = db.Pool.Exec(r.Context(),
		`INSERT INTO media_assets (id, tenant_id, user_id, type, name, file_url, mime_type, category, size, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())`,
		assetID, tenantID, claims.UserID, body.Type, body.Name, body.FileURL, truncateMIME(body.MimeType), nullableStr(body.Category), body.Size)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create asset failed")
		return
	}

	OK(w, map[string]string{"id": assetID, "name": body.Name, "file_url": body.FileURL})
}

// ── Upload helpers ──

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

// truncateMIME ensures MIME type fits in the VARCHAR(64) column.
func truncateMIME(mime string) string {
	if len(mime) > 64 {
		return mime[:64]
	}
	return mime
}

// isExecutableMIME 拦截可执行/脚本类 MIME，防止伪装为图片上传恶意文件。
// P1-6: 即使 magic bytes 检测出真实类型，仍需拒绝危险类型落地存储。
// fileName 用于扩展名兜底（net/http 的 DetectContentType 对 PE/ELF/Mach-O
// 通常返回 application/octet-stream，无法区分可执行文件与普通二进制）。
func isExecutableMIME(mime string, fileName string) bool {
	// 归一化小写并去掉参数
	m := strings.ToLower(strings.TrimSpace(mime))
	if i := strings.Index(m, ";"); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}
	switch m {
	case "application/x-msdownload",
		"application/x-msdos-program",
		"application/x-msi",
		"application/x-sh",
		"application/x-shar",
		"application/x-csh",
		"application/x-bat",
		"application/x-batch",
		"application/vnd.microsoft.portable-executable",
		"application/x-elf",
		"application/x-executable",
		"application/x-mach-o-executable",
		"application/x-mach-binary":
		return true
	}
	// 扩展名兜底：octet-stream（PE/ELF/Mach-O magic bytes 通用回退）或 text/plain
	// （shell 脚本检测为 text/plain）+ 危险扩展名 → 拒绝
	if m == "application/octet-stream" || m == "" || m == "text/plain" || m == "text/html" {
		ext := strings.ToLower(filepath.Ext(fileName))
		switch ext {
		case ".exe", ".dll", ".msi", ".sh", ".bat", ".cmd", ".com", ".scr",
			".so", ".dylib", ".app", ".jar", ".class", ".py", ".rb", ".pl",
			".ps1", ".vbs", ".wsf", ".htm", ".html":
			return true
		}
	}
	// text/plain 可能是任何脚本，但 magic bytes 只能识别文本；
	// 这里只拦截明确的可执行二进制类型，text/plain 由业务层判断扩展名
	return false
}
