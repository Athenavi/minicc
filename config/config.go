package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
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
	RedisMode        string   // "single", "cluster", "sentinel"
	RedisAddr        string
	RedisPassword    string
	RedisDB          int
	RedisAddrs       []string // for cluster mode
	RedisMasterName  string   // for sentinel mode
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

	// LLM
	LLMProvider    string
	LLMAPIKey      string
	LLMModel       string
	LLMBaseURL     string

	// Storage
	StorageBackend string // "local" or "s3"
	StorageRoot    string // local root path
	S3Endpoint     string
	S3Bucket       string
	S3AccessKey    string
	S3SecretKey    string
	S3UseSSL       bool   // S3/MinIO use SSL

	// Rate Limit
	RateLimitRPM        int  // requests per minute per user
	RateLimitFailClose  bool // P1-2: Redis 不可用时是否拒绝写操作（生产=true，开发=false）
	RateLimitGlobal int // global requests per minute

	// Log
	LogLevel string // debug / info / warn / error

	// Stripe
	StripeSecretKey    string
	StripeWebhookSecret string
	StripePriceID      string

	// 支付（支付宝/微信）
	PublicBaseURL     string // 公网可达的基础 URL，用于构造支付回调 notify_url
	FrontendURL       string // 前端地址（如 http://localhost:5173）；SSO 回调 302 目标，空 = 同源 "/"
	AlipayAppID       string
	AlipayPrivateKey  string // 应用私钥（PEM）
	AlipayPublicKey   string // 支付宝公钥（PEM）
	AlipayGateway     string // 默认生产网关；沙箱用 https://openapi-sandbox.dl.alipaydev.com/gateway.do
	WechatMchID       string
	WechatAppID       string
	WechatAPIv3Key    string // APIv3 密钥（32 位）
	WechatMchCertSerialNo string // 商户 API 证书序列号
	WechatMchPrivateKey   string // 商户 API 证书私钥（PEM）

	// Agent behavior
	AgentMaxTurns     int // max LLM-tool turns per run (default 10)
	AgentMaxTokens    int // max output tokens per LLM call (default 8192)
	AgentContextLimit int // max messages before pruning (default 20)

	// Python AI 引擎
	PythonEngineAddress string // HTTP 地址，如 "localhost:8000"
	PythonEngineTimeout time.Duration

	// LLM Gateway（Python 引擎内置）
	LLMGatewayURL  string // Python 引擎 LLM Gateway 地址，如 "http://localhost:8000"
	LLMGatewayKey  string // LLM Gateway API Key（可选）

	// Temporal
	TemporalAddress string // Temporal Server 地址，如 "localhost:7233"

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
		Port:            getEnv("PORT", "8080"),
		ReadTimeout:     getDuration("READ_TIMEOUT", 10*time.Second),
		WriteTimeout:    getDuration("WRITE_TIMEOUT", 60*time.Second),
		IdleTimeout:     getDuration("IDLE_TIMEOUT", 120*time.Second),
		// P2-4: 默认空，强制通过 env 提供；与 Python 端 config.py 保持一致
		PostgresDSN:      getEnv("POSTGRES_DSN", ""),
		PostgresMaxConn:  getInt("POSTGRES_MAX_CONN", 20),
		PostgresMinConn:  getInt("POSTGRES_MIN_CONN", 2),
		PostgresReadDSNs: getStringSlice("POSTGRES_READ_DSNS", []string{}),
		RedisMode:         getEnv("REDIS_MODE", "single"),
		RedisAddr:         getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     getEnv("REDIS_PASSWORD", ""),
		RedisDB:           getInt("REDIS_DB", 0),
		RedisAddrs:        getStringSlice("REDIS_ADDRS", []string{}),
		RedisMasterName:   getEnv("REDIS_MASTER_NAME", ""),
		RedisSentinelAddrs: getStringSlice("REDIS_SENTINEL_ADDRS", []string{}),
		RedisPoolSize:      getInt("REDIS_POOL_SIZE", 50),
		JWTSecret:       getEnv("JWT_SECRET", ""),
		JWTExpiration:   getDuration("JWT_EXPIRATION", 24*time.Hour),
		InternalToken:   getEnv("INTERNAL_TOKEN", ""),
		DisableRegistration: isTruthy(getEnv("DISABLE_REGISTRATION", "")),
		CookieSecure:    isTruthy(getEnv("COOKIE_SECURE", "")),
		CORSOrigins:     getEnv("CORS_ORIGINS", "http://localhost:3000,http://localhost:5173"),
		LLMProvider:     getEnv("LLM_PROVIDER", "openai"),
		LLMAPIKey:       getEnv("LLM_API_KEY", ""),
		LLMModel:        getEnv("LLM_MODEL", "gpt-4o"),
		LLMBaseURL:      getEnv("LLM_BASE_URL", ""),
		StorageBackend:  getEnv("STORAGE_BACKEND", "local"),
		StorageRoot:     getEnv("STORAGE_ROOT", "./workspace"),
		S3Endpoint:      getEnv("S3_ENDPOINT", ""),
		S3Bucket:        getEnv("S3_BUCKET", "minicc"),
		S3AccessKey:     getEnv("S3_ACCESS_KEY", ""),
		S3SecretKey:     getEnv("S3_SECRET_KEY", ""),
		S3UseSSL:        isTruthy(getEnv("S3_USE_SSL", "")),
		RateLimitRPM:       getInt("RATE_LIMIT_RPM", 100),
		RateLimitFailClose: isTruthy(getEnv("RATE_LIMIT_FAIL_CLOSE", "")),
		RateLimitGlobal: getInt("RATE_LIMIT_GLOBAL", 10000),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		StripeSecretKey:   getEnv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret: getEnv("STRIPE_WEBHOOK_SECRET", ""),
		StripePriceID:     getEnv("STRIPE_PRICE_ID", "price_1000_credits"),

		// 支付（支付宝/微信）
		PublicBaseURL:      getEnv("PUBLIC_BASE_URL", ""),
		FrontendURL:        getEnv("FRONTEND_URL", ""),
		AlipayAppID:        getEnv("ALIPAY_APP_ID", ""),
		AlipayPrivateKey:   getEnv("ALIPAY_PRIVATE_KEY", ""),
		AlipayPublicKey:    getEnv("ALIPAY_PUBLIC_KEY", ""),
		AlipayGateway:      getEnv("ALIPAY_GATEWAY", ""),
		WechatMchID:        getEnv("WXPAY_MCH_ID", ""),
		WechatAppID:        getEnv("WXPAY_APP_ID", ""),
		WechatAPIv3Key:     getEnv("WXPAY_API_V3_KEY", ""),
		WechatMchCertSerialNo: getEnv("WXPAY_MCH_CERT_SERIAL_NO", ""),
		WechatMchPrivateKey:   getEnv("WXPAY_MCH_PRIVATE_KEY", ""),
		AgentMaxTurns:     getInt("AGENT_MAX_TURNS", 10),
		AgentMaxTokens:    getInt("AGENT_MAX_TOKENS", 8192),
		AgentContextLimit: getInt("AGENT_CONTEXT_LIMIT", 20),

		// Python AI 引擎（连接池配置）

		// Python AI 引擎
		PythonEngineAddress: getEnv("PYTHON_ENGINE_ADDRESS", "localhost:8000"),
		PythonEngineTimeout: getDuration("PYTHON_ENGINE_TIMEOUT", 5*time.Minute),

		// LLM Gateway（Python 引擎内置）
		LLMGatewayURL: getEnv("LLM_GATEWAY_URL", getEnv("PYTHON_ENGINE_ADDRESS", "localhost:8000")),
		LLMGatewayKey: getEnv("LLM_GATEWAY_KEY", ""),

		// Temporal
		TemporalAddress: getEnv("TEMPORAL_ADDRESS", "localhost:7233"),

		PayPalClientID:    getEnv("PAYPAL_CLIENT_ID", ""),
		PayPalSecret:      getEnv("PAYPAL_SECRET", ""),
		PayPalSandbox:     isTruthy(getEnv("PAYPAL_SANDBOX", "")),

		PluginsConfigPath: getEnv("PLUGINS_CONFIG_PATH", "./plugins.json"),
		PluginDataDir:     getEnv("PLUGIN_DATA_DIR", "./data/plugins"),
	}

	// JWT_SECRET is required.
	if !ValidateJWTSecret(cfg.JWTSecret) {
		os.Stderr.WriteString("FATAL: JWT_SECRET environment variable must be set to a strong, unique value\n")
		os.Exit(1)
	}

	return cfg
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
