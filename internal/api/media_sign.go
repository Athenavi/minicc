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

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
)

// 濯掍綋绛惧悕 URL锛圥0 淇锛氬獟浣撲笉鍐嶈８鍏紑鍙寽娴嬶紱鏈湴鍚庣 HMAC锛孲3 鍚庣璧板師鐢熼绛惧悕锛夈€?// 绛惧悕 = HMAC-SHA256(JWTSecret, assetID|exp)锛岀煭鏃舵晥锛堥粯璁?15 鍒嗛挓锛夛紝涓庤祫浜х粦瀹氥€?
const mediaSignTTL = 15 * time.Minute

// signMediaURL 涓鸿祫浜х敓鎴愮鍚嶄笅杞?URL锛堟牎楠屽綊灞炲悗绛惧彂锛夈€?func signMediaURL(ctx context.Context, assetID, secret, tenantID, userID string) (string, error) {
	// 褰掑睘鏍￠獙
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

// SignMedia POST /v1/media/{id}/sign 鈥斺€?閴存潈鍚庣鍙戠鍚?URL銆?func (h *MediaHandler) SignMedia(w http.ResponseWriter, r *http.Request) {
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

// ServeSignedMedia GET /media/s/{assetID}?exp=&sig= 鈥斺€?鏍￠獙绛惧悕鍚庢祦寮忚繑鍥炴枃浠躲€?func (h *MediaHandler) ServeSignedMedia(w http.ResponseWriter, r *http.Request) {
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
	// 鍙栨枃浠惰矾寰?	var filePath string
	if err := db.ReadPool().QueryRow(r.Context(),
		`SELECT COALESCE(file_path, '') FROM media_assets WHERE id = $1`, assetID).Scan(&filePath); err != nil || filePath == "" {
		http.Error(w, "media not found", http.StatusNotFound)
		return
	}
	full := filepath.Join(h.mediaRoot(), filepath.FromSlash(filePath))
	// 闃插尽锛氱‘淇濊В鏋愬悗浠嶅湪濯掍綋鏍圭洰褰曞唴
	root := filepath.Clean(h.mediaRoot())
	clean := filepath.Clean(full)
	if clean != root && !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
		http.Error(w, "invalid path", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, clean)
}

// mediaRoot 杩斿洖濯掍綋瀛樺偍鏍癸紙涓庤矾鐢辨敞鍐屾椂 FileServer 鍚屾簮锛夈€?func (h *MediaHandler) mediaRoot() string {
	return h.root
}
