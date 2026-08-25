package api

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/athenavi/chiron/config"
	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
	"golang.org/x/crypto/bcrypt"
)

// installLockPath锛氬畨瑁呯姸鎬佹枃浠躲€備綅浜庤繍琛屾椂鏁版嵁鐩綍锛堜笌 data/media銆乨ata/skills 鍚岀骇锛夛紝
// 鐢卞畨瑁呮祦绋嬪啓鍏ワ紱姝ｅ父妯″紡鍚姩鏃惰鍙栧叾涓殑 DSN/Redis 閰嶇疆瑕嗙洊寮曞杩炴帴锛堥噸鍚敓鏁堬級銆?
const installLockPath = "./data/install.lock"

// 鈹€鈹€ 瀹夎浠ょ墝锛圝enkins 妯″紡锛夆攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
//
// 鎵€鏈?/v1/install/* 绔偣蹇呴』鎼哄甫瀹夎浠ょ墝锛圶-Install-Token header 鎴??token= 鏌ヨ鍙傛暟锛夛細
//   - APP_SECRET 宸查厤缃細HMAC-SHA256 纭畾鎬ф淳鐢燂紙閲嶅惎鍚庝笉鍙橈級锛?
//   - APP_SECRET 鏈厤缃細杩涚▼鍐呴殢鏈虹敓鎴愶紝鐢?main 鎵撳嵃鍒板惎鍔ㄦ棩蹇楋紝閮ㄧ讲鑰呭嚟鏃ュ織浠ょ墝杩涘叆瀹夎椤点€?
//
// 瀹夎瀹屾垚鍚庯紙install.lock 鏍囪 completed锛夊畨瑁呯鐐规嫆缁濈户缁闂紝浠ょ墝闅忎箣澶辨晥銆?
var installToken string

// InitInstallToken 鍒濆鍖栧綋鍓嶈繘绋嬬殑瀹夎浠ょ墝骞惰繑鍥烇紙骞傜瓑锛氶噸澶嶈皟鐢ㄨ繑鍥炲悓涓€浠ょ墝锛夈€?
func InitInstallToken(cfg *config.Config) string {
	if installToken != "" {
		return installToken
	}
	if cfg != nil && cfg.ValidateAppSecret() {
		installToken = deriveInstallToken(cfg.AppSecret)
	} else {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			// 鍙娴嬩护鐗岀瓑浜庢病鏈変护鐗岋細绯荤粺鐔垫簮涓嶅彲鐢ㄦ椂蹇呴』鏄惧紡澶辫触锛堝畨鍏?fail-fast锛?
			panic("crypto/rand unavailable: cannot generate install token")
		}
		installToken = base64.RawURLEncoding.EncodeToString(buf)
	}
	return installToken
}

// InstallToken 杩斿洖褰撳墠杩涚▼鐨勫畨瑁呬护鐗岋紙鏈垵濮嬪寲鏃朵负绌轰覆锛夈€?
func InstallToken() string { return installToken }

// InstallTokenIsSet 鎸囩ず瀹夎浠ょ墝鏄惁宸插垵濮嬪寲锛坰etup 妯″紡锛夈€?
func InstallTokenIsSet() bool { return installToken != "" }

func deriveInstallToken(appSecret string) string {
	h := hmac.New(sha256.New, []byte(appSecret))
	h.Write([]byte("chiron-install-token"))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// installMW 鏍￠獙瀹夎浠ょ墝锛歑-Install-Token header 浼樺厛锛屽叾娆??token= 鏌ヨ鍙傛暟锛涘父閲忔椂闂存瘮杈冦€?
func installMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Install-Token")
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		if installToken == "" || subtle.ConstantTimeCompare([]byte(token), []byte(installToken)) != 1 {
			Unauthorized(w, "install token required or invalid")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// 鈹€鈹€ install.lock 鐘舵€佹枃浠?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// InstallLock 璁板綍瀹夎娴佺▼鐘舵€併€侱SN / Redis 瀵嗙爜绛夋晱鎰熷瓧娈典互 AES-256-GCM 鍔犲瘑鍚庤惤鐩橈紝
// 瀵嗛挜鐢?APP_SECRET 娲剧敓锛堝煙鍒嗙锛夛紱浠呭綋 APP_SECRET 鏈夋晥鏃舵墠鍏佽鍐欏叆锛圫tep 2 鍓嶇敱 Step 1 鎶婂叧锛夈€?
type InstallLock struct {
	Completed     bool      `json:"completed"`
	Step1Done     bool      `json:"step1_done"`
	Step2Done     bool      `json:"step2_done"`
	Step3Done     bool      `json:"step3_done"`
	AppSecretSet  bool      `json:"app_secret_set"`
	AppSecretPlain string   `json:"app_secret_plain,omitempty"` // 瀹夎鍚戝涓敤鎴锋彁浜ょ殑 APP_SECRET锛堜粎鍐呭瓨浣跨敤锛屼笉钀界洏锛?
	DSN           string    `json:"dsn,omitempty"`            // AES-256-GCM 鍔犲瘑
	RedisAddr     string    `json:"redis_addr,omitempty"`     // AES-256-GCM 鍔犲瘑
	RedisPassword string    `json:"redis_password,omitempty"` // AES-256-GCM 鍔犲瘑
	RedisDB       int       `json:"redis_db,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	CompletedAt   time.Time `json:"completed_at,omitempty"`
}

// LoadInstallLock 璇诲彇瀹夎鐘舵€侊紱鏂囦欢涓嶅瓨鍦ㄦ椂杩斿洖绌虹姸鎬侊紙鏈畨瑁咃級銆?
func LoadInstallLock() (*InstallLock, error) {
	data, err := os.ReadFile(installLockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &InstallLock{}, nil
		}
		return nil, err
	}
	var lk InstallLock
	if err := json.Unmarshal(data, &lk); err != nil {
		return nil, fmt.Errorf("parse install.lock: %w", err)
	}
	return &lk, nil
}

// SaveInstallLock 鍘熷瓙鍐欏叆瀹夎鐘舵€侊細闅忔満涓存椂鏂囦欢 + rename銆?
// Windows 鐨?os.Rename 涓嶈鐩栧凡瀛樺湪鐩爣锛屽啓鍏ュ墠鍏堢Щ闄ゆ棫鏂囦欢锛堟湰鍦版暟鎹枃浠讹紝鍙帴鍙楃煭鏆傜獥鍙ｏ級銆?
func SaveInstallLock(lk *InstallLock) error {
	dir := filepath.Dir(installLockPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(lk, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".install.lock-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // 澶辫触璺緞娓呯悊锛涙垚鍔?rename 鍚庢棤娈嬬暀
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if _, err := os.Stat(installLockPath); err == nil {
		if err := os.Remove(installLockPath); err != nil {
			return err
		}
	}
	return os.Rename(tmpName, installLockPath)
}

// lockEncryptKey 鐢?APP_SECRET 娲剧敓 install.lock 鐨?AES-256-GCM 瀵嗛挜锛堝煙鍒嗙锛夈€?
func lockEncryptKey(appSecret string) []byte {
	h := hmac.New(sha256.New, []byte(appSecret))
	h.Write([]byte("chiron-install-lock-key"))
	return h.Sum(nil)
}

func encryptSecret(appSecret, plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	block, err := aes.NewCipher(lockEncryptKey(appSecret))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}

func decryptSecret(appSecret, enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	data, err := base64.RawStdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(lockEncryptKey(appSecret))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("install lock: ciphertext too short")
	}
	nonce, sealed := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("install lock: decrypt: %w", err)
	}
	return string(plain), nil
}

// dataDirWritable 鎺㈡祴瀹夎鐘舵€佹枃浠舵墍鍦ㄧ洰褰曟槸鍚﹀彲鍐欙紙Step 1 鐜妫€娴嬮」锛夈€?
func dataDirWritable() bool {
	dir := filepath.Dir(installLockPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".wtest-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// ApplyInstallLockConfig 鐢?main 鍦ㄥ惎鍔ㄦ椂璋冪敤锛堜粎 APP_SECRET 鏈夋晥鏃讹級锛?
// 璇诲彇宸插畬鎴愮殑 install.lock锛岀敤鍏朵腑鍔犲瘑淇濆瓨鐨?DSN/Redis 閰嶇疆瑕嗙洊寮曞杩炴帴鍊硷紙閲嶅惎鐢熸晥锛夈€?
// lock 涓嶅瓨鍦ㄣ€佹湭瀹屾垚鎴栬В瀵嗗け璐ワ紙APP_SECRET 鍙樻洿锛夋椂涓虹┖鎿嶄綔銆?
// 浼樺厛绾э細POSTGRES_DSN 浠?env 鏄惧紡璁剧疆涓哄噯锛坙ock 浠呭湪 env 鏈缃椂鍏滃簳锛夛紱
// Redis 閰嶇疆浠?lock 涓哄噯锛堝畨瑁呭悜瀵兼渶杩戜竴娆＄‘璁ょ殑鍊硷級锛屽悗鍙?system_settings 鍙啀瑕嗙洊銆?
func ApplyInstallLockConfig(cfg *config.Config) {
	if cfg == nil || !cfg.ValidateAppSecret() {
		return
	}
	lk, err := LoadInstallLock()
	if err != nil {
		slog.Warn("install lock: read failed, ignoring", "error", err)
		return
	}
	if !lk.Completed {
		return
	}
	dsn, err := decryptSecret(cfg.AppSecret, lk.DSN)
	if err != nil {
		slog.Warn("install lock: decrypt dsn failed (APP_SECRET changed since install?)", "error", err)
		return
	}
	if dsn != "" && cfg.PostgresDSN == "" {
		cfg.PostgresDSN = dsn
	}
	redisOK := false
	if addr, err := decryptSecret(cfg.AppSecret, lk.RedisAddr); err == nil && addr != "" {
		cfg.RedisAddr = addr
		redisOK = true
	}
	if pwd, err := decryptSecret(cfg.AppSecret, lk.RedisPassword); err == nil {
		cfg.RedisPassword = pwd
	}
	// RedisDB 浠呭湪 Redis 閰嶇疆鏁翠綋鍙В瀵嗘椂搴旂敤锛岄伩鍏嶅崐濂楅厤缃敓鏁堬紙涓€鑷存€э級
	if redisOK && lk.RedisDB != 0 {
		cfg.RedisDB = lk.RedisDB
	}
	slog.Info("applied database config from install.lock", "postgres_set", dsn != "", "redis_set", cfg.RedisAddr != "")
}

type InstallHandler struct {
	cfg  *config.Config
	auth *auth.Authenticator
}

func NewInstallHandler(cfg *config.Config) *InstallHandler {
	return &InstallHandler{
		cfg:  cfg,
		auth: auth.NewAuthenticator(cfg.JWTSecret, cfg.JWTExpiration),
	}
}

type InstallStatus struct {
	Needed bool   `json:"needed"`
	Reason string `json:"reason,omitempty"`
	DB     bool   `json:"db"`
	Redis  bool   `json:"redis"`

	// 渚濊禆鎺㈡祴鏄庣粏锛堝垵濮嬪寲椤甸潰灞曠ず鍚勫氨缁」锛?
	Deps []InstallDep `json:"deps,omitempty"`
}

type InstallDep struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// Status checks if the system needs initialization.
// GET /v1/install/status
func (h *InstallHandler) Status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var status InstallStatus
	status.Deps = make([]InstallDep, 0, 2)

	// 渚濊禆 1锛歅ostgreSQL 杩為€氭€э紙鐪熷疄 ping锛?
	dbOK := db.Pool != nil && db.Pool.Ping(ctx) == nil
	status.DB = dbOK
	status.Deps = append(status.Deps, InstallDep{
		Name:    "postgres",
		OK:      dbOK,
		Message: map[bool]string{true: "PostgreSQL 杩炴帴姝ｅ父", false: "PostgreSQL 涓嶅彲鐢細璇锋鏌?POSTGRES_DSN"}[dbOK],
	})

	// 渚濊禆 2锛歊edis 杩為€氭€э紙鐪熷疄 ping锛?
	redisOK := db.Redis != nil && db.Redis.Ping(ctx).Err() == nil
	status.Redis = redisOK
	status.Deps = append(status.Deps, InstallDep{
		Name:    "redis",
		OK:      redisOK,
		Message: map[bool]string{true: "Redis 杩炴帴姝ｅ父", false: "Redis 涓嶅彲鐢細璇锋鏌?REDIS_ADDR / 瀵嗙爜"}[redisOK],
	})

	// If at least one user with role 'owner' exists, system is initialized
	if dbOK {
		var count int
		err := db.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM users WHERE role = 'owner'`).Scan(&count)
		if err != nil || count == 0 {
			status.Needed = true
			status.Reason = "no admin user configured"
		}
	} else {
		status.Needed = true
		status.Reason = "postgres unavailable"
	}

	OK(w, status)
}

// 鈹€鈹€ 瀹夎娴佺▼锛坰etup 妯″紡锛夆攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// Step1Request 鏃犺姹備綋銆?
// Step1 鐜妫€娴嬶細APP_SECRET 鏄惁宸查厤缃紙闈炲急鍊?鍗犱綅绗︼級銆佸畨瑁呯姸鎬佺洰褰曟槸鍚﹀彲鍐欍€佸綋鍓嶅畨瑁呰繘搴︺€?
// GET /v1/install/step1
func (h *InstallHandler) Step1(w http.ResponseWriter, r *http.Request) {
	lk, err := LoadInstallLock()
	if err != nil {
		slog.Error("install step1: read install lock", "error", err)
		InternalError(w, "failed to read install state")
		return
	}
	if lk.Completed {
		OK(w, map[string]interface{}{
			"completed":      true,
			"message":        "绯荤粺宸插畬鎴愬畨瑁咃紝瀹夎鍏ュ彛宸插叧闂?,
			"app_secret_set": h.cfg.ValidateAppSecret(),
		})
		return
	}
	OK(w, map[string]interface{}{
		"completed":      false,
		"app_secret_set": h.cfg.ValidateAppSecret(),
		"data_writable":  dataDirWritable(),
		"step2_done":     lk.Step2Done,
		"step3_done":     lk.Step3Done,
	})
}

type Step2Request struct {
	AppSecret     string `json:"app_secret,omitempty"`
	PostgresDSN   string `json:"postgres_dsn"`
	RedisAddr     string `json:"redis_addr,omitempty"`
	RedisPassword string `json:"redis_password,omitempty"`
	RedisDB       int    `json:"redis_db,omitempty"`
}

// Step2 淇濆瓨鏁版嵁搴撻厤缃細楠岃瘉 PG 杩炴帴锛堟垚鍔熷嵆寤虹珛鍏ㄥ眬杩炴帴姹犱緵 Step 3 浣跨敤锛夆啋
// Redis 鍙€夛紙濉啓鍒欓獙璇佽繛閫氭€э級鈫?鏁忔劅瀛楁 AES-256-GCM 鍔犲瘑鍚庡啓鍏?install.lock銆?
// 閰嶇疆鍦ㄩ噸鍚湇鍔″悗鍏ㄩ潰鐢熸晥锛堜笌鐜版湁銆岄噸鍚敓鏁堛€嶇殑鏋舵瀯涓€鑷达級銆?
// 褰?APP_SECRET 鏈湪鐜鍙橀噺涓厤缃椂锛屽厑璁搁€氳繃璇锋眰浣撴彁浜?app_secret锛?
// 鐢ㄤ簬鍔犲瘑钀界洏骞朵緵 Step 3 鍒涘缓绠＄悊鍛樹娇鐢紙閲嶅惎鍚庝粛闇€瑕佸湪 .env 涓厤缃級銆?
// POST /v1/install/step2
func (h *InstallHandler) Step2(w http.ResponseWriter, r *http.Request) {
	// 纭畾鐢ㄤ簬鍔犲瘑鐨?APP_SECRET锛氱幆澧冨彉閲忎紭鍏堬紝鍏舵璇锋眰浣撴彁浜?
	appSecret := h.cfg.AppSecret
	if !h.cfg.ValidateAppSecret() {
		// APP_SECRET 鏈厤缃紝鍏佽浠庤姹備綋涓彁浜?
		// 浣嗘鏃舵棤娉曡В瀵嗕箣鍓嶇殑 lock锛屽洜姝ゆ殏涓嶅鐞嗗凡鏈?lock 鐨勬儏鍐?
		// 鍏堣В鏋愯姹備綋鑾峰彇 app_secret
		lk, lkErr := LoadInstallLock()
		if lkErr != nil {
			slog.Error("install step2: read install lock", "error", lkErr)
			InternalError(w, "failed to read install state")
			return
		}
		if lk.Completed {
			BadRequest(w, "绯荤粺宸插畬鎴愬畨瑁?)
			return
		}

		// 鍏堣В鐮佽姹備綋锛堜笉鎻愬墠鏍￠獙 app_secret锛岃鍚庣画閫昏緫澶勭悊锛?
		var req Step2Request
		if err := DecodeJSON(w, r, &req); err != nil {
			BadRequest(w, ErrInvalidReq)
			return
		}
		req.PostgresDSN = strings.TrimSpace(req.PostgresDSN)
		if req.PostgresDSN == "" {
			BadRequest(w, "postgres_dsn 蹇呭～")
			return
		}
		appSecret = strings.TrimSpace(req.AppSecret)
		if appSecret == "" {
			BadRequest(w, "APP_SECRET 鏈厤缃細璇峰湪琛ㄥ崟涓～鍐欓儴缃蹭富瀵嗛挜锛圓PP_SECRET锛夛紝鎴栧厛鍦?.env 閰嶇疆鍚庨噸鍚湇鍔?)
			return
		}
		// 涓存椂鏍￠獙锛氱敤鎻愪氦鐨?app_secret 楠岃瘉寮哄害
		if !config.ValidateJWTSecret(appSecret) {
			BadRequest(w, "APP_SECRET 寮哄害涓嶈冻锛氳浣跨敤 32 浣嶄互涓婄殑闅忔満瀛楃涓?)
			return
		}
		// 淇濆瓨鍒?lock 涓緵鍚庣画姝ラ浣跨敤
		lk.AppSecretPlain = appSecret
		lk.AppSecretSet = true
		if lk.CreatedAt.IsZero() {
			lk.CreatedAt = time.Now()
		}
		lk.Step1Done = true
		// 鏆備笉淇濆瓨 lock锛圫tep 3 瀹屾垚鏃朵竴璧蜂繚瀛橈級

		// 1) 楠岃瘉 PostgreSQL 杩炴帴
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		if err := db.ConnectPostgres(ctx, req.PostgresDSN, h.cfg.PostgresMaxConn, h.cfg.PostgresMinConn); err != nil {
			slog.Warn("install step2: postgres connect failed", "error", err)
			BadRequest(w, "PostgreSQL 杩炴帴澶辫触锛氳妫€鏌?DSN 鍦板潃銆佺鍙ｃ€佽处鍙峰瘑鐮佷笌缃戠粶杩為€氭€?)
			return
		}

		// 2) Redis 鍙€?
		redisAddr := strings.TrimSpace(req.RedisAddr)
		redisSet := redisAddr != ""
		if redisSet {
			rcfg := db.RedisConfig{
				Mode:     "single",
				Addr:     redisAddr,
				Password: req.RedisPassword,
				DB:       req.RedisDB,
				PoolSize: h.cfg.RedisPoolSize,
			}
			rc, rerr := db.NewRedisClient(rcfg)
			if rerr != nil {
				slog.Warn("install step2: redis init failed", "error", rerr)
				BadRequest(w, "Redis 杩炴帴澶辫触锛氳妫€鏌ュ湴鍧€銆佺鍙ｄ笌瀵嗙爜")
				return
			}
			pingCtx, cancelPing := context.WithTimeout(r.Context(), 5*time.Second)
			perr := rc.Ping(pingCtx).Err()
			cancelPing()
			_ = rc.Close()
			if perr != nil {
				slog.Warn("install step2: redis ping failed", "error", perr)
				BadRequest(w, "Redis 杩炴帴澶辫触锛氳妫€鏌ュ湴鍧€銆佺鍙ｄ笌瀵嗙爜")
				return
			}
		}

		// 3) 鍔犲瘑钀界洏锛堝瘑閽ヤ娇鐢ㄦ彁浜ょ殑 app_secret锛?
		dsnEnc, err := encryptSecret(appSecret, req.PostgresDSN)
		if err != nil {
			slog.Error("install step2: encrypt dsn", "error", err)
			InternalError(w, "failed to encrypt dsn")
			return
		}
		redisAddrEnc, _ := encryptSecret(appSecret, redisAddr)
		redisPwdEnc, _ := encryptSecret(appSecret, req.RedisPassword)

		lk.Step2Done = true
		lk.DSN = dsnEnc
		lk.RedisAddr = redisAddrEnc
		lk.RedisPassword = redisPwdEnc
		lk.RedisDB = req.RedisDB
		// 涓嶄繚瀛?app_secret_plain 鍒扮鐩?
		clearAppSecret := lk.AppSecretPlain
		lk.AppSecretPlain = ""
		if err := SaveInstallLock(lk); err != nil {
			slog.Error("install step2: save install lock", "error", err)
			InternalError(w, "failed to save install.lock")
			return
		}
		lk.AppSecretPlain = clearAppSecret // 鎭㈠鍐呭瓨涓緵鍚庣画浣跨敤

		OK(w, map[string]interface{}{
			"step2_done": true,
			"message":    "鏁版嵁搴撻厤缃凡淇濆瓨骞堕獙璇侀€氳繃锛涜缁х画鍒涘缓绠＄悊鍛樿处鎴?,
		})
		return
	}
	lk, err := LoadInstallLock()
	if err != nil {
		slog.Error("install step2: read install lock", "error", err)
		InternalError(w, "failed to read install state")
		return
	}
	if lk.Completed {
		BadRequest(w, "绯荤粺宸插畬鎴愬畨瑁?)
		return
	}

	var req Step2Request
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	req.PostgresDSN = strings.TrimSpace(req.PostgresDSN)
	if req.PostgresDSN == "" {
		BadRequest(w, "postgres_dsn 蹇呭～")
		return
	}

	// 1) 楠岃瘉 PostgreSQL 杩炴帴锛涙垚鍔熷悗 db.Pool 宸插氨缁紙Step 3 鍒涘缓绠＄悊鍛樹緷璧栵級
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	if err := db.ConnectPostgres(ctx, req.PostgresDSN, h.cfg.PostgresMaxConn, h.cfg.PostgresMinConn); err != nil {
		// 杩炴帴閿欒缁嗚妭锛坔ost/port/user/DSN锛変粎璁板綍鏈嶅姟绔棩蹇楋紝瀹㈡埛绔彧缁欓€氱敤鎻愮ず
		slog.Warn("install step2: postgres connect failed", "error", err)
		BadRequest(w, "PostgreSQL 杩炴帴澶辫触锛氳妫€鏌?DSN 鍦板潃銆佺鍙ｃ€佽处鍙峰瘑鐮佷笌缃戠粶杩為€氭€?)
		return
	}

	// 2) Redis 鍙€夛細濉啓鍒欓獙璇佽繛閫氭€э紙鐣欑┖ = 涓嶄繚瀛?Redis 閰嶇疆锛岄噸鍚悗鎸?env 榛樿骞堕檷绾ц繍琛岋級
	redisAddr := strings.TrimSpace(req.RedisAddr)
	redisSet := redisAddr != ""
	if redisSet {
		rcfg := db.RedisConfig{
			Mode:     "single",
			Addr:     redisAddr,
			Password: req.RedisPassword,
			DB:       req.RedisDB,
			PoolSize: h.cfg.RedisPoolSize,
		}
		rc, rerr := db.NewRedisClient(rcfg)
		if rerr != nil {
			slog.Warn("install step2: redis init failed", "error", rerr)
			BadRequest(w, "Redis 杩炴帴澶辫触锛氳妫€鏌ュ湴鍧€銆佺鍙ｄ笌瀵嗙爜")
			return
		}
		pingCtx, cancelPing := context.WithTimeout(r.Context(), 5*time.Second)
		perr := rc.Ping(pingCtx).Err()
		cancelPing()
		_ = rc.Close()
		if perr != nil {
			slog.Warn("install step2: redis ping failed", "error", perr)
			BadRequest(w, "Redis 杩炴帴澶辫触锛氳妫€鏌ュ湴鍧€銆佺鍙ｄ笌瀵嗙爜")
			return
		}
	}

	// 3) 鍔犲瘑钀界洏锛堝瘑閽ユ淳鐢熻嚜 APP_SECRET锛屾湰姝ヤ箣鍓嶅凡鏍￠獙鍏舵湁鏁堟€э級
	dsnEnc, err := encryptSecret(h.cfg.AppSecret, req.PostgresDSN)
	if err != nil {
		slog.Error("install step2: encrypt dsn", "error", err)
		InternalError(w, "failed to encrypt dsn")
		return
	}
	redisAddrEnc, _ := encryptSecret(h.cfg.AppSecret, redisAddr)
	redisPwdEnc, _ := encryptSecret(h.cfg.AppSecret, req.RedisPassword)

	if lk.CreatedAt.IsZero() {
		lk.CreatedAt = time.Now()
	}
	lk.Step1Done = true
	lk.Step2Done = true
	lk.AppSecretSet = true
	lk.DSN = dsnEnc
	lk.RedisAddr = redisAddrEnc
	lk.RedisPassword = redisPwdEnc
	lk.RedisDB = req.RedisDB
	if err := SaveInstallLock(lk); err != nil {
		slog.Error("install step2: save install lock", "error", err)
		InternalError(w, "failed to save install.lock")
		return
	}

	OK(w, map[string]interface{}{
		"step2_done": true,
		"message":    "鏁版嵁搴撻厤缃凡淇濆瓨骞堕獙璇侀€氳繃锛涜缁х画鍒涘缓绠＄悊鍛樿处鎴?,
	})
}

// rebuildDBFromLock 浠?install.lock 涓В瀵?DSN 骞堕噸寤?db.Pool锛堜腑鏂仮澶嶇敤锛夈€?
// 闇€瑕?APP_SECRET 鏈夋晥锛屽惁鍒欐棤娉曡В瀵嗐€?
func (h *InstallHandler) rebuildDBFromLock(lk *InstallLock, r *http.Request) error {
	appSecret := h.cfg.AppSecret
	if !h.cfg.ValidateAppSecret() {
		// APP_SECRET 鏈湪鐜鍙橀噺閰嶇疆锛屼絾 lock 涓彲鑳戒繚瀛樹簡 AppSecretPlain
		if lk.AppSecretPlain != "" {
			appSecret = lk.AppSecretPlain
		} else {
			return fmt.Errorf("APP_SECRET not configured and no plain secret in lock")
		}
	}
	dsn, err := decryptSecret(appSecret, lk.DSN)
	if err != nil {
		return fmt.Errorf("decrypt dsn: %w", err)
	}
	if dsn == "" {
		return fmt.Errorf("empty dsn in lock")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	return db.ConnectPostgres(ctx, dsn, h.cfg.PostgresMaxConn, h.cfg.PostgresMinConn)
}

type Step3Request struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	// AppSecret 鍦ㄦ湇鍔￠噸鍚悗 db.Pool 涓?nil 鏃朵娇鐢細浠?lock 涓В瀵?DSN 閲嶅缓杩炴帴姹?
	AppSecret string `json:"app_secret,omitempty"`
}

// Step3 鎵ц鏁版嵁搴撹縼绉汇€佸垱寤洪涓?owner 绠＄悊鍛樺苟鏍囪瀹夎瀹屾垚銆傚畬鎴愬悗瀹夎鍏ュ彛鍏抽棴锛?
// 鐢变簬 Step 2 淇濆瓨鐨?DSN/Redis 閰嶇疆闇€閲嶅惎鍚庡叏闈㈢敓鏁堬紝鍓嶇鎻愮ず閲嶅惎鏈嶅姟銆?
// 鏀寔涓柇鎭㈠锛氬綋 db.Pool == nil 浣?Step2Done == true 鏃跺皾璇曚粠 lock 瑙ｅ瘑 DSN 閲嶅缓杩炴帴姹犮€?
// POST /v1/install/step3
func (h *InstallHandler) Step3(w http.ResponseWriter, r *http.Request) {
	lk, err := LoadInstallLock()
	if err != nil {
		slog.Error("install step3: read install lock", "error", err)
		InternalError(w, "failed to read install state")
		return
	}
	if lk.Completed {
		BadRequest(w, "绯荤粺宸插畬鎴愬畨瑁?)
		return
	}
	if !lk.Step2Done {
		BadRequest(w, "璇峰厛瀹屾垚鏁版嵁搴撻厤缃楠?)
		return
	}
	// 涓柇鎭㈠锛歋tep2Done == true 浣?db.Pool == nil锛堟湇鍔￠噸鍚悗锛夛紝浠?lock 瑙ｅ瘑 DSN 閲嶅缓杩炴帴姹?
	if db.Pool == nil {
		// 鍏堣В鐮佽姹備綋锛岃幏鍙栧彲鑳芥惡甯︾殑 app_secret 鐢ㄤ簬瑙ｅ瘑 DSN
		var req Step3Request
		if err := DecodeJSON(w, r, &req); err != nil {
			BadRequest(w, ErrInvalidReq)
			return
		}
		if req.Email == "" || req.Password == "" || req.Name == "" {
			BadRequest(w, "email, password, and name are required")
			return
		}
		if len(req.Password) < 8 {
			BadRequest(w, "password must be at least 8 characters")
			return
		}
		// 濡傛灉 AppSecretPlain 涓虹┖锛屽皾璇曠敤璇锋眰浣撲腑鐨?app_secret 鍏滃簳
		if req.AppSecret != "" {
			if !config.ValidateJWTSecret(req.AppSecret) {
				BadRequest(w, "APP_SECRET 寮哄害涓嶈冻")
				return
			}
			lk.AppSecretPlain = req.AppSecret
		}
		if err := h.rebuildDBFromLock(lk, r); err != nil {
			slog.Error("install step3: rebuild pool from lock", "error", err)
			BadRequest(w, "鏁版嵁搴撹繛鎺ュ凡澶辨晥锛岃閲嶆柊瀹屾垚鏁版嵁搴撻厤缃楠?)
			return
		}
		// 鎵ц鏁版嵁搴撹縼绉伙紙骞傜瓑锛氬凡搴旂敤鐨勮縼绉昏嚜鍔ㄨ烦杩囷級
		migrateCtx, migrateCancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer migrateCancel()
		if err := db.RunAtlasMigrations(migrateCtx, db.Pool, "migrations"); err != nil {
			slog.Error("install step3: run migrations", "error", err)
			InternalError(w, "鏁版嵁搴撳垵濮嬪寲澶辫触锛岃妫€鏌ユ棩蹇楃‘璁よ縼绉婚敊璇?)
			return
		}
		// 骞傜瓑 seed 榛樿绉熸埛
		if err := db.EnsureDefaultTenant(migrateCtx, db.Pool); err != nil {
			slog.Error("install step3: ensure default tenant", "error", err)
			InternalError(w, "榛樿绉熸埛鍒濆鍖栧け璐?)
			return
		}

		userID, err := createOwnerAccount(r.Context(), req.Email, req.Name, req.Password)
		if err != nil {
			if errors.Is(err, ErrAlreadyInitialized) {
				BadRequest(w, "system already initialized")
				return
			}
			slog.Error("install step3: create owner", "error", err)
			InternalError(w, "failed to create admin user")
			return
		}

		// 鏍囪瀹夎瀹屾垚
		lk.Step3Done = true
		lk.Completed = true
		lk.CompletedAt = time.Now()
		if err := SaveInstallLock(lk); err != nil {
			slog.Error("install step3: save install lock", "error", err)
			InternalError(w, "failed to save install.lock")
			return
		}

		token, err := h.auth.GenerateToken(userID, req.Email, "owner", DefaultTenantID, auth.RolePermissions["owner"])
		if err != nil {
			InternalError(w, "authentication failed")
			return
		}

		SetTokenCookie(w, token, int(h.cfg.JWTExpiration.Seconds()), h.cfg.CookieSecure)
		Created(w, map[string]interface{}{
			"message":   "瀹夎瀹屾垚锛岃閲嶅惎鏈嶅姟浣垮叏閮ㄥ姛鑳界敓鏁?,
			"completed": true,
			"restart":   true,
			"user": map[string]string{
				"id":    userID,
				"email": req.Email,
				"name":  req.Name,
				"role":  "owner",
			},
		})
		return
	}

	// 闈炰腑鏂仮澶嶈矾寰勶細db.Pool 宸插氨缁紝鐩存帴瑙ｇ爜璇锋眰浣?
	var req Step3Request
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if req.Email == "" || req.Password == "" || req.Name == "" {
		BadRequest(w, "email, password, and name are required")
		return
	}
	if len(req.Password) < 8 {
		BadRequest(w, "password must be at least 8 characters")
		return
	}

	// 鎵ц鏁版嵁搴撹縼绉伙紙骞傜瓑锛氬凡搴旂敤鐨勮縼绉昏嚜鍔ㄨ烦杩囷級
	migrateCtx, migrateCancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer migrateCancel()
	if err := db.RunAtlasMigrations(migrateCtx, db.Pool, "migrations"); err != nil {
		slog.Error("install step3: run migrations", "error", err)
		InternalError(w, "鏁版嵁搴撳垵濮嬪寲澶辫触锛岃妫€鏌ユ棩蹇楃‘璁よ縼绉婚敊璇?)
		return
	}
	// 骞傜瓑 seed 榛樿绉熸埛
	if err := db.EnsureDefaultTenant(migrateCtx, db.Pool); err != nil {
		slog.Error("install step3: ensure default tenant", "error", err)
		InternalError(w, "榛樿绉熸埛鍒濆鍖栧け璐?)
		return
	}

	userID, err := createOwnerAccount(r.Context(), req.Email, req.Name, req.Password)
	if err != nil {
		if errors.Is(err, ErrAlreadyInitialized) {
			BadRequest(w, "system already initialized")
			return
		}
		slog.Error("install step3: create owner", "error", err)
		InternalError(w, "failed to create admin user")
		return
	}

	// 鏍囪瀹夎瀹屾垚锛堝箓绛夛細閲嶅璇锋眰鍦ㄧ涓€姝ュ嵆琚?lock.Completed 鎷︽埅锛?
	lk.Step3Done = true
	lk.Completed = true
	lk.CompletedAt = time.Now()
	if err := SaveInstallLock(lk); err != nil {
		slog.Error("install step3: save install lock", "error", err)
		InternalError(w, "failed to save install.lock")
		return
	}

	// Generate token and set cookie
	token, err := h.auth.GenerateToken(userID, req.Email, "owner", DefaultTenantID, auth.RolePermissions["owner"])
	if err != nil {
		InternalError(w, "authentication failed")
		return
	}

	SetTokenCookie(w, token, int(h.cfg.JWTExpiration.Seconds()), h.cfg.CookieSecure)
	Created(w, map[string]interface{}{
		"message":   "瀹夎瀹屾垚锛岃閲嶅惎鏈嶅姟浣垮叏閮ㄥ姛鑳界敓鏁?,
		"completed": true,
		"restart":   true,
		"user": map[string]string{
			"id":    userID,
			"email": req.Email,
			"name":  req.Name,
			"role":  "owner",
		},
	})
}

// ErrAlreadyInitialized 琛ㄧず绯荤粺宸插瓨鍦?owner 璐︽埛锛岀姝㈤噸澶嶅垵濮嬪寲銆?
var ErrAlreadyInitialized = errors.New("system already initialized")

// createOwnerAccount 鍘熷瓙鍖栧垱寤洪涓?owner 璐︽埛锛堜簨鍔?+ 鍜ㄨ閿佷繚璇佸苟鍙?璇诲壇鏈粸鍚庝笅鍙垵濮嬪寲涓€娆★級銆?
// 宸插瓨鍦?owner 鏃惰繑鍥?ErrAlreadyInitialized銆?
func createOwnerAccount(ctx context.Context, email, name, password string) (string, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('chiron_install'))`); err != nil {
		return "", err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE role = 'owner'`).Scan(&count); err != nil {
		return "", err
	}
	if count > 0 {
		return "", ErrAlreadyInitialized
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	// Create owner user using PostgreSQL's gen_random_uuid()
	var userID string
	err = tx.QueryRow(ctx,
		`INSERT INTO users (id, tenant_id, email, name, password_hash, role, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, 'owner', NOW(), NOW())
		 RETURNING id`,
		DefaultTenantID, email, name, string(hash),
	).Scan(&userID)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return userID, nil
}

type SetupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	// AppSecret 鍦ㄦ湇鍔￠噸鍚悗 db.Pool 涓?nil 鏃朵娇鐢細浠?lock 涓В瀵?DSN 閲嶅缓杩炴帴姹?
	AppSecret string `json:"app_secret,omitempty"`
}

// Setup initializes the system with the first admin user.
// POST /v1/install/setup
func (h *InstallHandler) Setup(w http.ResponseWriter, r *http.Request) {
	var req SetupRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}

	// Validate
	if req.Email == "" || req.Password == "" || req.Name == "" {
		BadRequest(w, "email, password, and name are required")
		return
	}
	if len(req.Password) < 8 {
		BadRequest(w, "password must be at least 8 characters")
		return
	}

	// 涓柇鎭㈠锛歞b.Pool == nil锛堟湇鍔￠噸鍚悗锛夛紝浠?lock 閲嶅缓杩炴帴姹?
	if db.Pool == nil {
		lk, lkErr := LoadInstallLock()
		if lkErr != nil {
			slog.Error("install setup: read install lock", "error", lkErr)
			InternalError(w, "failed to read install state")
			return
		}
		if !lk.Step2Done {
			InternalError(w, "鏁版嵁搴撴湭閰嶇疆锛岃鍏堝畬鎴愭暟鎹簱閰嶇疆姝ラ")
			return
		}
		// 濡傛灉 AppSecretPlain 涓虹┖锛屽皾璇曠敤璇锋眰浣撲腑鐨?app_secret 鍏滃簳
		if req.AppSecret != "" {
			if !config.ValidateJWTSecret(req.AppSecret) {
				BadRequest(w, "APP_SECRET 寮哄害涓嶈冻")
				return
			}
			lk.AppSecretPlain = req.AppSecret
		}
		if err := h.rebuildDBFromLock(lk, r); err != nil {
			slog.Error("install setup: rebuild pool from lock", "error", err)
			InternalError(w, "鏁版嵁搴撹繛鎺ュ凡澶辨晥锛岃閲嶆柊瀹屾垚鏁版嵁搴撻厤缃楠?)
			return
		}
	}

	// 鎵ц鏁版嵁搴撹縼绉伙紙骞傜瓑锛氬凡搴旂敤鐨勮縼绉昏嚜鍔ㄨ烦杩囷級
	migrateCtx, migrateCancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer migrateCancel()
	if db.Pool != nil {
		if err := db.RunAtlasMigrations(migrateCtx, db.Pool, "migrations"); err != nil {
			slog.Error("install setup: run migrations", "error", err)
			InternalError(w, "鏁版嵁搴撳垵濮嬪寲澶辫触锛岃妫€鏌ユ棩蹇楃‘璁よ縼绉婚敊璇?)
			return
		}
		// 骞傜瓑 seed 榛樿绉熸埛
		if err := db.EnsureDefaultTenant(migrateCtx, db.Pool); err != nil {
			slog.Error("install setup: ensure default tenant", "error", err)
			InternalError(w, "榛樿绉熸埛鍒濆鍖栧け璐?)
			return
		}
	} else {
		InternalError(w, "鏁版嵁搴撹繛鎺ヤ笉鍙敤锛岃妫€鏌ユ暟鎹簱閰嶇疆")
		return
	}

	userID, err := createOwnerAccount(r.Context(), req.Email, req.Name, req.Password)
	if err != nil {
		if errors.Is(err, ErrAlreadyInitialized) {
			BadRequest(w, "system already initialized")
			return
		}
		InternalError(w, "setup failed")
		return
	}

	// Generate token and set cookie
	token, err := h.auth.GenerateToken(userID, req.Email, "owner", DefaultTenantID, auth.RolePermissions["owner"])
	if err != nil {
		InternalError(w, "authentication failed")
		return
	}

	SetTokenCookie(w, token, int(h.cfg.JWTExpiration.Seconds()), h.cfg.CookieSecure)
	Created(w, map[string]interface{}{
		"message": "system initialized",
		"user": map[string]string{
			"id":    userID,
			"email": req.Email,
			"name":  req.Name,
			"role":  "owner",
		},
	})
}
