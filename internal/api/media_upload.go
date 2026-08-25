package api

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/id"
	"github.com/athenavi/chiron/internal/storage"
)

// 鈹€鈹€ Upload (multipart file) 鈹€鈹€

func (h *MediaHandler) Upload(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	tenantID := claims.TenantID

	r.Body = http.MaxBytesReader(w, r.Body, 200<<20) // 200MB 涓婁紶涓婇檺
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		// 鏆撮湶鐪熷疄鍘熷洜锛堣姹備綋瓒呴檺 / boundary 缂哄け / 鏍煎紡鎹熷潖锛夛紝渚夸簬瀹氫綅
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

	// P1-6: 鐢?magic bytes 妫€娴嬬湡瀹?MIME锛岄伩鍏嶅鎴风浼€?Content-Type 涓婁紶鍙墽琛屾枃浠?
	declaredMIME := truncateMIME(header.Header.Get("Content-Type"))
	detectedMIME := truncateMIME(http.DetectContentType(fileData))
	// 浼樺厛閲囩敤妫€娴嬪埌鐨?MIME锛涘０鏄庝笌妫€娴嬩笉涓€鑷存椂浠ユ娴嬩负鍑嗭紙鏇村畨鍏級
	mimeType := detectedMIME
	if declaredMIME != "" && declaredMIME == detectedMIME {
		// 澹版槑涓庢娴嬩竴鑷达紝淇濈暀澹版槑鐨勶紙鍙兘鏇寸簿纭紝濡?image/png vs image/jpeg锛?
		mimeType = declaredMIME
	}
	// 鎷掔粷鍙墽琛?鑴氭湰绫?MIME锛堝嵆浣垮鎴风浼涓哄浘鐗囷級
	// P1-6: 浼犲叆鏂囦欢鍚嶄互渚挎墿灞曞悕鍏滃簳锛圥E/ELF magic bytes 鍙娴嬩负 octet-stream锛?
	if isExecutableMIME(mimeType, header.Filename) {
		BadRequest(w, "file type not allowed: "+mimeType)
		return
	}
	assetType := detectType(mimeType)
	category := r.FormValue("category")
	parentID := r.FormValue("parent_id") // 褰撳墠鐩綍锛涚┖ = 鏍圭洰褰?

	dir := h.resolveDir(r)
	name := r.FormValue("name")
	if name == "" {
		name = header.Filename
	}

	// 鍏堟彃鍏ユ暟鎹簱鑾峰彇 UUID锛堝惈 parent_id锛屽瓙鏂囦欢澶逛笂浼犱笉钀藉埌鏍圭洰褰曪級
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

	// 鏇存柊 file_url
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

// 鈹€鈹€ PresignUpload 鈥?returns a presigned URL for client-side direct upload 鈹€鈹€

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

// 鈹€鈹€ CompleteUpload 鈥?called by client after presigned upload is done 鈹€鈹€

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

	// 鏍￠獙 file_url 鎸囧悜閰嶇疆鐨勫瓨鍌ㄥ悗绔紙闃叉瀹㈡埛绔吉閫犱换鎰?URL 鍏ュ簱锛?
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

	// ID 鐢辨湇鍔＄鐢熸垚锛屼笉淇′换瀹㈡埛绔紶鍏ョ殑 body.ID
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

// 鈹€鈹€ Upload helpers 鈹€鈹€

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

// isExecutableMIME 鎷︽埅鍙墽琛?鑴氭湰绫?MIME锛岄槻姝吉瑁呬负鍥剧墖涓婁紶鎭舵剰鏂囦欢銆?
// P1-6: 鍗充娇 magic bytes 妫€娴嬪嚭鐪熷疄绫诲瀷锛屼粛闇€鎷掔粷鍗遍櫓绫诲瀷钀藉湴瀛樺偍銆?
// fileName 鐢ㄤ簬鎵╁睍鍚嶅厹搴曪紙net/http 鐨?DetectContentType 瀵?PE/ELF/Mach-O
// 閫氬父杩斿洖 application/octet-stream锛屾棤娉曞尯鍒嗗彲鎵ц鏂囦欢涓庢櫘閫氫簩杩涘埗锛夈€?
func isExecutableMIME(mime string, fileName string) bool {
	// 褰掍竴鍖栧皬鍐欏苟鍘绘帀鍙傛暟
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
	// 鎵╁睍鍚嶅厹搴曪細octet-stream锛圥E/ELF/Mach-O magic bytes 閫氱敤鍥為€€锛夋垨 text/plain
	// 锛坰hell 鑴氭湰妫€娴嬩负 text/plain锛? 鍗遍櫓鎵╁睍鍚?鈫?鎷掔粷
	if m == "application/octet-stream" || m == "" || m == "text/plain" || m == "text/html" {
		ext := strings.ToLower(filepath.Ext(fileName))
		switch ext {
		case ".exe", ".dll", ".msi", ".sh", ".bat", ".cmd", ".com", ".scr",
			".so", ".dylib", ".app", ".jar", ".class", ".py", ".rb", ".pl",
			".ps1", ".vbs", ".wsf", ".htm", ".html":
			return true
		}
	}
	// text/plain 鍙兘鏄换浣曡剼鏈紝浣?magic bytes 鍙兘璇嗗埆鏂囨湰锛?
	// 杩欓噷鍙嫤鎴槑纭殑鍙墽琛屼簩杩涘埗绫诲瀷锛宼ext/plain 鐢变笟鍔″眰鍒ゆ柇鎵╁睍鍚?
	return false
}
