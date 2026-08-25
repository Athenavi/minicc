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

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"golang.org/x/crypto/bcrypt"
)

// installLockPath：安装状态文件。位于运行时数据目录（与 data/media、data/skills 同级），
// 由安装流程写入；正常模式启动时读取其中的 DSN/Redis 配置覆盖引导连接（重启生效）。
const installLockPath = "./data/install.lock"

// ── 安装令牌（Jenkins 模式）───────────────────────────────────────────────
//
// 所有 /v1/install/* 端点必须携带安装令牌（X-Install-Token header 或 ?token= 查询参数）：
//   - APP_SECRET 已配置：HMAC-SHA256 确定性派生（重启后不变）；
//   - APP_SECRET 未配置：进程内随机生成，由 main 打印到启动日志，部署者凭日志令牌进入安装页。
//
// 安装完成后（install.lock 标记 completed）安装端点拒绝继续访问，令牌随之失效。
var installToken string

// InitInstallToken 初始化当前进程的安装令牌并返回（幂等：重复调用返回同一令牌）。
func InitInstallToken(cfg *config.Config) string {
	if installToken != "" {
		return installToken
	}
	if cfg != nil && cfg.ValidateAppSecret() {
		installToken = deriveInstallToken(cfg.AppSecret)
	} else {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			// 可预测令牌等于没有令牌：系统熵源不可用时必须显式失败（安全 fail-fast）
			panic("crypto/rand unavailable: cannot generate install token")
		}
		installToken = base64.RawURLEncoding.EncodeToString(buf)
	}
	return installToken
}

// InstallToken 返回当前进程的安装令牌（未初始化时为空串）。
func InstallToken() string { return installToken }

// InstallTokenIsSet 指示安装令牌是否已初始化（setup 模式）。
func InstallTokenIsSet() bool { return installToken != "" }

func deriveInstallToken(appSecret string) string {
	h := hmac.New(sha256.New, []byte(appSecret))
	h.Write([]byte("minicc-install-token"))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// installMW 校验安装令牌：X-Install-Token header 优先，其次 ?token= 查询参数；常量时间比较。
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

// ── install.lock 状态文件 ─────────────────────────────────────────────────

// InstallLock 记录安装流程状态。DSN / Redis 密码等敏感字段以 AES-256-GCM 加密后落盘，
// 密钥由 APP_SECRET 派生（域分离）；仅当 APP_SECRET 有效时才允许写入（Step 2 前由 Step 1 把关）。
type InstallLock struct {
	Completed     bool      `json:"completed"`
	Step1Done     bool      `json:"step1_done"`
	Step2Done     bool      `json:"step2_done"`
	Step3Done     bool      `json:"step3_done"`
	AppSecretSet  bool      `json:"app_secret_set"`
	AppSecretPlain string   `json:"app_secret_plain,omitempty"` // 安装向导中用户提交的 APP_SECRET（仅内存使用，不落盘）
	DSN           string    `json:"dsn,omitempty"`            // AES-256-GCM 加密
	RedisAddr     string    `json:"redis_addr,omitempty"`     // AES-256-GCM 加密
	RedisPassword string    `json:"redis_password,omitempty"` // AES-256-GCM 加密
	RedisDB       int       `json:"redis_db,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	CompletedAt   time.Time `json:"completed_at,omitempty"`
}

// LoadInstallLock 读取安装状态；文件不存在时返回空状态（未安装）。
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

// SaveInstallLock 原子写入安装状态：随机临时文件 + rename。
// Windows 的 os.Rename 不覆盖已存在目标，写入前先移除旧文件（本地数据文件，可接受短暂窗口）。
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
	defer os.Remove(tmpName) // 失败路径清理；成功 rename 后无残留
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

// lockEncryptKey 由 APP_SECRET 派生 install.lock 的 AES-256-GCM 密钥（域分离）。
func lockEncryptKey(appSecret string) []byte {
	h := hmac.New(sha256.New, []byte(appSecret))
	h.Write([]byte("minicc-install-lock-key"))
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

// dataDirWritable 探测安装状态文件所在目录是否可写（Step 1 环境检测项）。
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

// ApplyInstallLockConfig 由 main 在启动时调用（仅 APP_SECRET 有效时）：
// 读取已完成的 install.lock，用其中加密保存的 DSN/Redis 配置覆盖引导连接值（重启生效）。
// lock 不存在、未完成或解密失败（APP_SECRET 变更）时为空操作。
// 优先级：POSTGRES_DSN 以 env 显式设置为准（lock 仅在 env 未设置时兜底）；
// Redis 配置以 lock 为准（安装向导最近一次确认的值），后台 system_settings 可再覆盖。
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
	// RedisDB 仅在 Redis 配置整体可解密时应用，避免半套配置生效（一致性）
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

	// 依赖探测明细（初始化页面展示各就绪项）
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

	// 依赖 1：PostgreSQL 连通性（真实 ping）
	dbOK := db.Pool != nil && db.Pool.Ping(ctx) == nil
	status.DB = dbOK
	status.Deps = append(status.Deps, InstallDep{
		Name:    "postgres",
		OK:      dbOK,
		Message: map[bool]string{true: "PostgreSQL 连接正常", false: "PostgreSQL 不可用：请检查 POSTGRES_DSN"}[dbOK],
	})

	// 依赖 2：Redis 连通性（真实 ping）
	redisOK := db.Redis != nil && db.Redis.Ping(ctx).Err() == nil
	status.Redis = redisOK
	status.Deps = append(status.Deps, InstallDep{
		Name:    "redis",
		OK:      redisOK,
		Message: map[bool]string{true: "Redis 连接正常", false: "Redis 不可用：请检查 REDIS_ADDR / 密码"}[redisOK],
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

// ── 安装流程（setup 模式）─────────────────────────────────────────────────

// Step1Request 无请求体。
// Step1 环境检测：APP_SECRET 是否已配置（非弱值/占位符）、安装状态目录是否可写、当前安装进度。
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
			"message":        "系统已完成安装，安装入口已关闭",
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

// Step2 保存数据库配置：验证 PG 连接（成功即建立全局连接池供 Step 3 使用）→
// Redis 可选（填写则验证连通性）→ 敏感字段 AES-256-GCM 加密后写入 install.lock。
// 配置在重启服务后全面生效（与现有「重启生效」的架构一致）。
// 当 APP_SECRET 未在环境变量中配置时，允许通过请求体提交 app_secret，
// 用于加密落盘并供 Step 3 创建管理员使用（重启后仍需要在 .env 中配置）。
// POST /v1/install/step2
func (h *InstallHandler) Step2(w http.ResponseWriter, r *http.Request) {
	// 确定用于加密的 APP_SECRET：环境变量优先，其次请求体提交
	appSecret := h.cfg.AppSecret
	if !h.cfg.ValidateAppSecret() {
		// APP_SECRET 未配置，允许从请求体中提交
		// 但此时无法解密之前的 lock，因此暂不处理已有 lock 的情况
		// 先解析请求体获取 app_secret
		lk, lkErr := LoadInstallLock()
		if lkErr != nil {
			slog.Error("install step2: read install lock", "error", lkErr)
			InternalError(w, "failed to read install state")
			return
		}
		if lk.Completed {
			BadRequest(w, "系统已完成安装")
			return
		}

		// 先解码请求体（不提前校验 app_secret，让后续逻辑处理）
		var req Step2Request
		if err := DecodeJSON(w, r, &req); err != nil {
			BadRequest(w, ErrInvalidReq)
			return
		}
		req.PostgresDSN = strings.TrimSpace(req.PostgresDSN)
		if req.PostgresDSN == "" {
			BadRequest(w, "postgres_dsn 必填")
			return
		}
		appSecret = strings.TrimSpace(req.AppSecret)
		if appSecret == "" {
			BadRequest(w, "APP_SECRET 未配置：请在表单中填写部署主密钥（APP_SECRET），或先在 .env 配置后重启服务")
			return
		}
		// 临时校验：用提交的 app_secret 验证强度
		if !config.ValidateJWTSecret(appSecret) {
			BadRequest(w, "APP_SECRET 强度不足：请使用 32 位以上的随机字符串")
			return
		}
		// 保存到 lock 中供后续步骤使用
		lk.AppSecretPlain = appSecret
		lk.AppSecretSet = true
		if lk.CreatedAt.IsZero() {
			lk.CreatedAt = time.Now()
		}
		lk.Step1Done = true
		// 暂不保存 lock（Step 3 完成时一起保存）

		// 1) 验证 PostgreSQL 连接
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		if err := db.ConnectPostgres(ctx, req.PostgresDSN, h.cfg.PostgresMaxConn, h.cfg.PostgresMinConn); err != nil {
			slog.Warn("install step2: postgres connect failed", "error", err)
			BadRequest(w, "PostgreSQL 连接失败：请检查 DSN 地址、端口、账号密码与网络连通性")
			return
		}

		// 2) Redis 可选
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
				BadRequest(w, "Redis 连接失败：请检查地址、端口与密码")
				return
			}
			pingCtx, cancelPing := context.WithTimeout(r.Context(), 5*time.Second)
			perr := rc.Ping(pingCtx).Err()
			cancelPing()
			_ = rc.Close()
			if perr != nil {
				slog.Warn("install step2: redis ping failed", "error", perr)
				BadRequest(w, "Redis 连接失败：请检查地址、端口与密码")
				return
			}
		}

		// 3) 加密落盘（密钥使用提交的 app_secret）
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
		// 不保存 app_secret_plain 到磁盘
		clearAppSecret := lk.AppSecretPlain
		lk.AppSecretPlain = ""
		if err := SaveInstallLock(lk); err != nil {
			slog.Error("install step2: save install lock", "error", err)
			InternalError(w, "failed to save install.lock")
			return
		}
		lk.AppSecretPlain = clearAppSecret // 恢复内存中供后续使用

		OK(w, map[string]interface{}{
			"step2_done": true,
			"message":    "数据库配置已保存并验证通过；请继续创建管理员账户",
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
		BadRequest(w, "系统已完成安装")
		return
	}

	var req Step2Request
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	req.PostgresDSN = strings.TrimSpace(req.PostgresDSN)
	if req.PostgresDSN == "" {
		BadRequest(w, "postgres_dsn 必填")
		return
	}

	// 1) 验证 PostgreSQL 连接；成功后 db.Pool 已就绪（Step 3 创建管理员依赖）
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	if err := db.ConnectPostgres(ctx, req.PostgresDSN, h.cfg.PostgresMaxConn, h.cfg.PostgresMinConn); err != nil {
		// 连接错误细节（host/port/user/DSN）仅记录服务端日志，客户端只给通用提示
		slog.Warn("install step2: postgres connect failed", "error", err)
		BadRequest(w, "PostgreSQL 连接失败：请检查 DSN 地址、端口、账号密码与网络连通性")
		return
	}

	// 2) Redis 可选：填写则验证连通性（留空 = 不保存 Redis 配置，重启后按 env 默认并降级运行）
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
			BadRequest(w, "Redis 连接失败：请检查地址、端口与密码")
			return
		}
		pingCtx, cancelPing := context.WithTimeout(r.Context(), 5*time.Second)
		perr := rc.Ping(pingCtx).Err()
		cancelPing()
		_ = rc.Close()
		if perr != nil {
			slog.Warn("install step2: redis ping failed", "error", perr)
			BadRequest(w, "Redis 连接失败：请检查地址、端口与密码")
			return
		}
	}

	// 3) 加密落盘（密钥派生自 APP_SECRET，本步之前已校验其有效性）
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
		"message":    "数据库配置已保存并验证通过；请继续创建管理员账户",
	})
}

type Step3Request struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// Step3 创建首个 owner 管理员并标记安装完成。完成后安装入口关闭；
// 由于 Step 2 保存的 DSN/Redis 配置需重启后全面生效，前端提示重启服务。
// POST /v1/install/step3
func (h *InstallHandler) Step3(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		BadRequest(w, "数据库尚未配置：请先完成数据库配置步骤")
		return
	}
	lk, err := LoadInstallLock()
	if err != nil {
		slog.Error("install step3: read install lock", "error", err)
		InternalError(w, "failed to read install state")
		return
	}
	if lk.Completed {
		BadRequest(w, "系统已完成安装")
		return
	}
	if !lk.Step2Done {
		BadRequest(w, "请先完成数据库配置步骤")
		return
	}

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

	// 标记安装完成（幂等：重复请求在第一步即被 lock.Completed 拦截）
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
		"message":   "安装完成，请重启服务使全部功能生效",
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

// ErrAlreadyInitialized 表示系统已存在 owner 账户，禁止重复初始化。
var ErrAlreadyInitialized = errors.New("system already initialized")

// createOwnerAccount 原子化创建首个 owner 账户（事务 + 咨询锁保证并发/读副本滞后下只初始化一次）。
// 已存在 owner 时返回 ErrAlreadyInitialized。
func createOwnerAccount(ctx context.Context, email, name, password string) (string, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('minicc_install'))`); err != nil {
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
