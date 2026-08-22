package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
)

// 媒体签名 URL（P0 修复：媒体不再裸公开可猜测；本地后端 HMAC，S3 后端走原生预签名）。
// 签名 = HMAC-SHA256(JWTSecret, assetID|exp)，短时效（默认 15 分钟），与资产绑定。

const mediaSignTTL = 15 * time.Minute

// signMediaURL 为资产生成签名下载 URL（校验归属后签发）。
func signMediaURL(ctx context.Context, assetID, secret, tenantID, userID string) (string, error) {
	// 归属校验
	var filePath string
	if err := db.ReadPool().QueryRow(ctx,
		`SELECT COALESCE(file_path, '') FROM media_assets WHERE id = $1 AND tenant_id = $2 AND user_id = $3`,
		assetID, tenantID, userID).Scan(&filePath); err != nil {
		return "", err
	}
	exp := time.Now().Add(mediaSignTTL).Unix()
	sig := mediaHMAC(secret, assetID, exp)
	return "/media/s/" + assetID + "?exp=" + strconv.FormatInt(exp, 10) + "&sig=" + sig, nil
}

func mediaHMAC(secret, assetID string, exp int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(assetID + "|" + strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

// SignMedia POST /v1/media/{id}/sign —— 鉴权后签发签名 URL。
func (h *MediaHandler) SignMedia(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		Unauthorized(w, "missing tenant context")
		return
	}
	assetID := r.PathValue("id")
	if assetID == "" {
		BadRequest(w, "id is required")
		return
	}
	url, err := signMediaURL(r.Context(), assetID, string(h.authenticator.SigningSecret()), claims.TenantID, claims.UserID)
	if err != nil {
		NotFound(w, "media asset not found")
		return
	}
	OK(w, map[string]interface{}{"url": url})
}

// ServeSignedMedia GET /media/s/{assetID}?exp=&sig= —— 校验签名后流式返回文件。
func (h *MediaHandler) ServeSignedMedia(w http.ResponseWriter, r *http.Request) {
	assetID := r.PathValue("assetID")
	expStr := r.URL.Query().Get("exp")
	sig := r.URL.Query().Get("sig")
	if assetID == "" || expStr == "" || sig == "" {
		http.Error(w, "missing signature params", http.StatusBadRequest)
		return
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		http.Error(w, "link expired", http.StatusForbidden)
		return
	}
	secret := string(h.authenticator.SigningSecret())
	if !hmac.Equal([]byte(sig), []byte(mediaHMAC(secret, assetID, exp))) {
		http.Error(w, "invalid signature", http.StatusForbidden)
		return
	}
	// 取文件路径
	var filePath string
	if err := db.ReadPool().QueryRow(r.Context(),
		`SELECT COALESCE(file_path, '') FROM media_assets WHERE id = $1`, assetID).Scan(&filePath); err != nil || filePath == "" {
		http.Error(w, "media not found", http.StatusNotFound)
		return
	}
	full := filepath.Join(h.mediaRoot(), filepath.FromSlash(filePath))
	// 防御：确保解析后仍在媒体根目录内
	root := filepath.Clean(h.mediaRoot())
	clean := filepath.Clean(full)
	if clean != root && !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
		http.Error(w, "invalid path", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, clean)
}

// mediaRoot 返回媒体存储根（与路由注册时 FileServer 同源）。
func (h *MediaHandler) mediaRoot() string {
	return h.root
}
