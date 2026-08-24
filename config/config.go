package config

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// AppSecret：部署级主密钥（≥32 字符）。用于：
	//  - 派生 JWT_SECRET / INTERNAL_TOKEN（当对应环境变量未显式设置时）
	//  - 加密 system_settings 中敏感配置（LLM/S3/支付密钥、redis/pg 密码）
	AppSecret string

	// Server
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

	// Database
	PostgresDSN      string
	PostgresMaxConn  int
	PostgresMinConn  int
	PostgresReadDSNs []string // read-replica DSNs (comma-separated)

	// Redis
	RedisMode          string // "single", "cluster", "sentinel"
	RedisAddr          string
	RedisPassword      string
	RedisDB            int
	RedisAddrs         []string // for cluster mode
	RedisMasterName    string   // for sentinel mode
	RedisSentinelAddrs []string // for sentinel mode
	RedisPoolSize      int

	// Auth
	JWTSecret     string
	JWTExpiration time.Duration
	// InternalToken：Go 网关 ↔ Python 引擎共享密钥。
	// Go 转发请求时注入 X-Internal-Token header，Python 据此校验 ?tenant_id= 透传身份。
	// 未配置时 Python 侧 fail-close 拒绝 query 透传身份（强制走 JWT/API Key）。
	InternalToken string

	// Registration
	DisableRegistration bool // 生产单租户可关闭公开注册（S 安全加固）

	// Cookie
	CookieSecure bool // 生产 HTTPS 下设置 Secure 标志，防止 JWT cookie 明文传输

	// CORS
	CORSOrigins string

	// LLM 调用已转发 Python 引擎（config 中 LLM 配置为遗留死配置，已移除）

	// Storage
	StorageBackend string // "local" or "s3"
	StorageRoot    string // local root path
	S3Endpoint     string
	S3Bucket       string
	S3AccessKey    string
	S3SecretKey    string
	S3UseSSL       bool // S3/MinIO use SSL

	// Rate Limit
	RateLimitRPM       int  // requests per minute per user
	RateLimitFailClose bool // P1-2: Redis 不可用时是否拒绝写操作（生产=true，开发=false）
	RateLimitGlobal    int  // global requests per minute

	// TrustedProxyCIDRs trusted reverse-proxy CIDRs (comma separated).
	// X-Forwarded-For / X-Real-IP are only honored when the direct peer
	// matches one of these CIDRs; otherwise clients could spoof IP-based limits.
	TrustedProxyCIDRs []string

	// MetricsToken shared bearer token for Prometheus to scrape /metrics.
	// When empty, /metrics still requires JWT admin permission.
	MetricsToken string

	// Log
	LogLevel string // debug / info / warn / error

	// 支付（支付宝/微信；Stripe 为遗留死配置已移除）
	PublicBaseURL         string // 公网可达的基础 URL，用于构造支付回调 notify_url
	FrontendURL           string // 前端地址（如 http://localhost:5173）；SSO 回调 302 目标，空 = 同源 "/"
	AlipayAppID           string
	AlipayPrivateKey      string // 应用私钥（PEM）
	AlipayPublicKey       string // 支付宝公钥（PEM）
	AlipayGateway         string // 默认生产网关；沙箱用 https://openapi-sandbox.dl.alipaydev.com/gateway.do
	WechatMchID           string
	WechatAppID           string
	WechatAPIv3Key        string // APIv3 密钥（32 位）
	WechatMchCertSerialNo string // 商户 API 证书序列号
	WechatMchPrivateKey   string // 商户 API 证书私钥（PEM）

	// Agent behavior
	AgentMaxTurns     int // max LLM-tool turns per run (default 10)
	AgentMaxTokens    int // max output tokens per LLM call (default 8192)
	AgentContextLimit int // max messages before pruning (default 20)
	AgentMaxConcurrency int // max concurrent agent runs (default 20)

	// Python AI 引擎
	PythonEngineAddress string // HTTP 地址，如 "localhost:8000"
	PythonEngineTimeout time.Duration

	// Temporal / LLMGateway 为遗留死配置（未使用），已移除

	// PayPal
	PayPalClientID string
	PayPalSecret   string
	PayPalSandbox  bool

	// Plugins
	PluginsConfigPath string // path to plugins.json (MCP server config)
	PluginDataDir     string // per-user plugin config root: {PluginDataDir}/{user_id}/plugins.json
}

func Load() *Config {
	loadDotEnv()     // .env file overrides config file
	loadConfigFile() // JSON config file (lowest priority)
	cfg := &Config{
		AppSecret:       getEnv("APP_SECRET", ""),
		Port:            getEnv("PORT", "8080"),
		ReadTimeout:     getDuration("READ_TIMEOUT", 10*time.Second),
		WriteTimeout:    getDuration("WRITE_TIMEOUT", 60*time.Second),
		IdleTimeout:     getDuration("IDLE_TIMEOUT", 120*time.Second),
		// 引导连接：仅当未显式设置 POSTGRES_DSN 时使用默认，供「只留 app_secret」初始化后从
		// system_settings 读取数据库集群配置覆盖（切换集群重启生效）。
		PostgresDSN:         getEnv("POSTGRES_DSN", "postgres://postgres@localhost:5432/minicc?sslmode=disable"),
		PostgresMaxConn:     getInt("POSTGRES_MAX_CONN", 20),
		PostgresMinConn:     getInt("POSTGRES_MIN_CONN", 2),
		PostgresReadDSNs:    getStringSlice("POSTGRES_READ_DSNS", []string{}),
		RedisMode:           getEnv("REDIS_MODE", "single"),
		RedisAddr:           getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:       getEnv("REDIS_PASSWORD", ""),
		RedisDB:             getInt("REDIS_DB", 0),
		RedisAddrs:          getStringSlice("REDIS_ADDRS", []string{}),
		RedisMasterName:     getEnv("REDIS_MASTER_NAME", ""),
		RedisSentinelAddrs:  getStringSlice("REDIS_SENTINEL_ADDRS", []string{}),
		RedisPoolSize:       getInt("REDIS_POOL_SIZE", 50),
		JWTSecret:           getEnv("JWT_SECRET", ""),
		JWTExpiration:       getDuration("JWT_EXPIRATION", 24*time.Hour),
		InternalToken:       getEnv("INTERNAL_TOKEN", ""),
		DisableRegistration: isTruthy(getEnv("DISABLE_REGISTRATION", "")),
		CookieSecure:        isTruthy(getEnv("COOKIE_SECURE", "")),
		CORSOrigins:         getEnv("CORS_ORIGINS", "http://localhost:3000,http://localhost:5173"),
		StorageBackend:      getEnv("STORAGE_BACKEND", "local"),
		StorageRoot:         getEnv("STORAGE_ROOT", "./workspace"),
		S3Endpoint:          getEnv("S3_ENDPOINT", ""),
		S3Bucket:            getEnv("S3_BUCKET", "minicc"),
		S3AccessKey:         getEnv("S3_ACCESS_KEY", ""),
		S3SecretKey:         getEnv("S3_SECRET_KEY", ""),
		S3UseSSL:            isTruthy(getEnv("S3_USE_SSL", "")),
		RateLimitRPM:        getInt("RATE_LIMIT_RPM", 100),
		RateLimitFailClose:  isTruthy(getEnv("RATE_LIMIT_FAIL_CLOSE", "")),
		RateLimitGlobal:     getInt("RATE_LIMIT_GLOBAL", 10000),
		TrustedProxyCIDRs:   getStringSlice("TRUSTED_PROXY_CIDRS", []string{}),
		MetricsToken:        getEnv("METRICS_TOKEN", ""),
		LogLevel:            getEnv("LOG_LEVEL", "info"),

		// 支付（支付宝/微信）
		PublicBaseURL:         getEnv("PUBLIC_BASE_URL", ""),
		FrontendURL:           getEnv("FRONTEND_URL", ""),
		AlipayAppID:           getEnv("ALIPAY_APP_ID", ""),
		AlipayPrivateKey:      getEnv("ALIPAY_PRIVATE_KEY", ""),
		AlipayPublicKey:       getEnv("ALIPAY_PUBLIC_KEY", ""),
		AlipayGateway:         getEnv("ALIPAY_GATEWAY", ""),
		WechatMchID:           getEnv("WXPAY_MCH_ID", ""),
		WechatAppID:           getEnv("WXPAY_APP_ID", ""),
		WechatAPIv3Key:        getEnv("WXPAY_API_V3_KEY", ""),
		WechatMchCertSerialNo: getEnv("WXPAY_MCH_CERT_SERIAL_NO", ""),
		WechatMchPrivateKey:   getEnv("WXPAY_MCH_PRIVATE_KEY", ""),
		AgentMaxTurns:         getInt("AGENT_MAX_TURNS", 10),
		AgentMaxTokens:        getInt("AGENT_MAX_TOKENS", 8192),
		AgentContextLimit:     getInt("AGENT_CONTEXT_LIMIT", 20),
		AgentMaxConcurrency:   getInt("AGENT_MAX_CONCURRENCY", 20),

		// Python AI 引擎（连接池配置）

		// Python AI 引擎
		PythonEngineAddress: getEnv("PYTHON_ENGINE_ADDRESS", "localhost:8000"),
		PythonEngineTimeout: getDuration("PYTHON_ENGINE_TIMEOUT", 5*time.Minute),

		PayPalClientID: getEnv("PAYPAL_CLIENT_ID", ""),
		PayPalSecret:   getEnv("PAYPAL_SECRET", ""),
		PayPalSandbox:  isTruthy(getEnv("PAYPAL_SANDBOX", "")),

		PluginsConfigPath: getEnv("PLUGINS_CONFIG_PATH", "./plugins.json"),
		PluginDataDir:     getEnv("PLUGIN_DATA_DIR", "./data/plugins"),
	}

	// APP_SECRET is required（部署级主密钥）。
	if !cfg.ValidateAppSecret() {
		os.Stderr.WriteString("FATAL: APP_SECRET environment variable must be set to a strong, unique value (32+ chars)\n")
		os.Exit(1)
	}

	// 派生密钥：当 JWT_SECRET / INTERNAL_TOKEN 未显式设置时，由 APP_SECRET 域分离派生。
	// 安全模型取舍：部署主密钥为唯一必填秘密；若需更强的密钥隔离，仍可显式设置 JWT_SECRET / INTERNAL_TOKEN 覆盖派生值。
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = deriveSubsecret(cfg.AppSecret, "minicc-jwt")
	}
	if cfg.InternalToken == "" {
		cfg.InternalToken = deriveSubsecret(cfg.AppSecret, "minicc-internal")
	}

	// JWT_SECRET is required (derived or explicit).
	if !ValidateJWTSecret(cfg.JWTSecret) {
		os.Stderr.WriteString("FATAL: JWT_SECRET (or its source APP_SECRET) must be set to a strong, unique value\n")
		os.Exit(1)
	}

	return cfg
}

// ValidateAppSecret 校验部署主密钥强度（≥32 字符，非弱值）。
func (c *Config) ValidateAppSecret() bool {
	return ValidateJWTSecret(c.AppSecret)
}

// deriveSubsecret 从主密钥派生确定性子密钥（HMAC-SHA256，域分离）。
func deriveSubsecret(secret, domain string) string {
	if secret == "" {
		return ""
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(domain))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// ValidateJWTSecret returns true if the secret is valid for production use.
func ValidateJWTSecret(secret string) bool {
	if secret == "" {
		return false
	}
	// Reject weak/known secrets
	weakSecrets := []string{
		"dev-secret-change-in-production",
		"dev-secret-change-in-production-12345678",
		"secret",
		"test-secret",
		"change-me",
		"changeme",
	}
	for _, ws := range weakSecrets {
		if secret == ws {
			return false
		}
	}
	// Require minimum length for security (at least 32 chars for strong encryption)
	if len(secret) < 32 {
		return false
	}
	return true
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func getStringSlice(key string, fallback []string) []string {
	if v := os.Getenv(key); v != "" {
		// Split by comma and trim whitespace
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return fallback
}

// isTruthy returns true if s is "true", "1", "yes", or "on" (case-insensitive).
func isTruthy(s string) bool {
	switch s {
	case "true", "1", "yes", "on", "TRUE", "YES", "ON":
		return true
	}
	return false
}

// loadDotEnv reads .env file and sets environment variables if not already set.
// findFileUpward searches for a file starting from the current directory
// and walking up to the filesystem root. Returns the first match.
func findFileUpward(name string) string {
	dir, _ := os.Getwd()
	for {
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached filesystem root
		}
		dir = parent
	}
	return name // fall back to original relative path (will fail with useful error)
}

func loadDotEnv() {
	path := findFileUpward(".env")
	data, err := os.ReadFile(path)
	if err != nil {
		return // .env file not found, skip
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// Strip quotes if present
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		// Only set if not already set (env vars take precedence)
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}
