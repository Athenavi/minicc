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
	// AppSecret锛氶儴缃茬骇涓诲瘑閽ワ紙鈮?2 瀛楃锛夈€傜敤浜庯細
	//  - 娲剧敓 JWT_SECRET / INTERNAL_TOKEN锛堝綋瀵瑰簲鐜鍙橀噺鏈樉寮忚缃椂锛?
	//  - 鍔犲瘑 system_settings 涓晱鎰熼厤缃紙LLM/S3/鏀粯瀵嗛挜銆乺edis/pg 瀵嗙爜锛?
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
	// InternalToken锛欸o 缃戝叧 鈫?Python 寮曟搸鍏变韩瀵嗛挜銆?
	// Go 杞彂璇锋眰鏃舵敞鍏?X-Internal-Token header锛孭ython 鎹鏍￠獙 ?tenant_id= 閫忎紶韬唤銆?
	// 鏈厤缃椂 Python 渚?fail-close 鎷掔粷 query 閫忎紶韬唤锛堝己鍒惰蛋 JWT/API Key锛夈€?
	InternalToken string

	// Registration
	DisableRegistration bool // 鐢熶骇鍗曠鎴峰彲鍏抽棴鍏紑娉ㄥ唽锛圫 瀹夊叏鍔犲浐锛?

	// Cookie
	CookieSecure bool // 鐢熶骇 HTTPS 涓嬭缃?Secure 鏍囧織锛岄槻姝?JWT cookie 鏄庢枃浼犺緭

	// CORS
	CORSOrigins string

	// LLM 璋冪敤宸茶浆鍙?Python 寮曟搸锛坈onfig 涓?LLM 閰嶇疆涓洪仐鐣欐閰嶇疆锛屽凡绉婚櫎锛?

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
	RateLimitFailClose bool // P1-2: Redis 涓嶅彲鐢ㄦ椂鏄惁鎷掔粷鍐欐搷浣滐紙鐢熶骇=true锛屽紑鍙?false锛?
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

	// 鏀粯锛堟敮浠樺疂/寰俊锛汼tripe 涓洪仐鐣欐閰嶇疆宸茬Щ闄わ級
	PublicBaseURL         string // 鍏綉鍙揪鐨勫熀纭€ URL锛岀敤浜庢瀯閫犳敮浠樺洖璋?notify_url
	FrontendURL           string // 鍓嶇鍦板潃锛堝 http://localhost:5173锛夛紱SSO 鍥炶皟 302 鐩爣锛岀┖ = 鍚屾簮 "/"
	AlipayAppID           string
	AlipayPrivateKey      string // 搴旂敤绉侀挜锛圥EM锛?
	AlipayPublicKey       string // 鏀粯瀹濆叕閽ワ紙PEM锛?
	AlipayGateway         string // 榛樿鐢熶骇缃戝叧锛涙矙绠辩敤 https://openapi-sandbox.dl.alipaydev.com/gateway.do
	WechatMchID           string
	WechatAppID           string
	WechatAPIv3Key        string // APIv3 瀵嗛挜锛?2 浣嶏級
	WechatMchCertSerialNo string // 鍟嗘埛 API 璇佷功搴忓垪鍙?
	WechatMchPrivateKey   string // 鍟嗘埛 API 璇佷功绉侀挜锛圥EM锛?

	// Agent behavior
	AgentMaxTurns     int // max LLM-tool turns per run (default 10)
	AgentMaxTokens    int // max output tokens per LLM call (default 8192)
	AgentContextLimit int // max messages before pruning (default 20)
	AgentMaxConcurrency int // max concurrent agent runs (default 20)

	// Python AI 寮曟搸
	PythonEngineAddress string // HTTP 鍦板潃锛屽 "localhost:8000"
	PythonEngineTimeout time.Duration

	// Temporal / LLMGateway 涓洪仐鐣欐閰嶇疆锛堟湭浣跨敤锛夛紝宸茬Щ闄?

	// PayPal
	PayPalClientID string
	PayPalSecret   string
	PayPalSandbox  bool

	// Plugins
	PluginsConfigPath string // path to plugins.json (MCP server config)
	PluginDataDir     string // per-user plugin config root: {PluginDataDir}/{user_id}/plugins.json
}

func Load() *Config {
	cfg := loadConfig()

	// APP_SECRET is required锛堥儴缃茬骇涓诲瘑閽ワ級銆?
	if !cfg.ValidateAppSecret() {
		os.Stderr.WriteString("FATAL: APP_SECRET environment variable must be set to a strong, unique value (32+ chars)\n")
		os.Exit(1)
	}

	// 娲剧敓瀵嗛挜锛氬綋 JWT_SECRET / INTERNAL_TOKEN 鏈樉寮忚缃椂锛岀敱 APP_SECRET 鍩熷垎绂绘淳鐢熴€?
	// 瀹夊叏妯″瀷鍙栬垗锛氶儴缃蹭富瀵嗛挜涓哄敮涓€蹇呭～绉樺瘑锛涜嫢闇€鏇村己鐨勫瘑閽ラ殧绂伙紝浠嶅彲鏄惧紡璁剧疆 JWT_SECRET / INTERNAL_TOKEN 瑕嗙洊娲剧敓鍊笺€?
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = deriveSubsecret(cfg.AppSecret, "chiron-jwt")
	}
	if cfg.InternalToken == "" {
		cfg.InternalToken = deriveSubsecret(cfg.AppSecret, "chiron-internal")
	}

	// JWT_SECRET is required (derived or explicit).
	if !ValidateJWTSecret(cfg.JWTSecret) {
		os.Stderr.WriteString("FATAL: JWT_SECRET (or its source APP_SECRET) must be set to a strong, unique value\n")
		os.Exit(1)
	}

	return cfg
}

// LoadAllowUnconfigured 涓?Load 鐩稿悓锛屼絾鍏佽 APP_SECRET / JWT_SECRET 缂哄け鎴栧急鍊艰€屼笉閫€鍑猴細
// 渚?cmd/chiron 鍦ㄣ€屽畨瑁呮ā寮忋€嶄笅鍚姩锛堥儴缃插皻鏈厤缃敮涓€涓诲瘑閽ワ紝鏃犳硶娲剧敓 JWT / 鍔犲瘑瀵嗛挜锛夈€?
// APP_SECRET 鏈夋晥鏃朵粛鎵ц娲剧敓锛涜皟鐢ㄦ柟椤荤敤 cfg.ValidateAppSecret() 鍒ゆ柇鏄惁杩涘叆瀹夎妯″紡銆?
func LoadAllowUnconfigured() *Config {
	cfg := loadConfig()

	if cfg.JWTSecret == "" {
		cfg.JWTSecret = deriveSubsecret(cfg.AppSecret, "chiron-jwt")
	}
	if cfg.InternalToken == "" {
		cfg.InternalToken = deriveSubsecret(cfg.AppSecret, "chiron-internal")
	}
	return cfg
}

func loadConfig() *Config {
	loadDotEnv()     // .env file overrides config file
	loadConfigFile() // JSON config file (lowest priority)
	cfg := &Config{
		AppSecret:       getEnv("APP_SECRET", ""),
		Port:            getEnv("PORT", "8080"),
		ReadTimeout:     getDuration("READ_TIMEOUT", 10*time.Second),
		WriteTimeout:    getDuration("WRITE_TIMEOUT", 60*time.Second),
		IdleTimeout:     getDuration("IDLE_TIMEOUT", 120*time.Second),
		// 寮曞杩炴帴锛氫粎褰撴湭鏄惧紡璁剧疆 POSTGRES_DSN 鏃朵娇鐢ㄩ粯璁わ紝渚涖€屽彧鐣?app_secret銆嶅垵濮嬪寲鍚庝粠
		// system_settings 璇诲彇鏁版嵁搴撻泦缇ら厤缃鐩栵紙鍒囨崲闆嗙兢閲嶅惎鐢熸晥锛夈€?
		PostgresDSN:         getEnv("POSTGRES_DSN", ""),
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
		S3Bucket:            getEnv("S3_BUCKET", "chiron"),
		S3AccessKey:         getEnv("S3_ACCESS_KEY", ""),
		S3SecretKey:         getEnv("S3_SECRET_KEY", ""),
		S3UseSSL:            isTruthy(getEnv("S3_USE_SSL", "")),
		RateLimitRPM:        getInt("RATE_LIMIT_RPM", 100),
		RateLimitFailClose:  isTruthy(getEnv("RATE_LIMIT_FAIL_CLOSE", "")),
		RateLimitGlobal:     getInt("RATE_LIMIT_GLOBAL", 10000),
		TrustedProxyCIDRs:   getStringSlice("TRUSTED_PROXY_CIDRS", []string{}),
		MetricsToken:        getEnv("METRICS_TOKEN", ""),
		LogLevel:            getEnv("LOG_LEVEL", "info"),

		// 鏀粯锛堟敮浠樺疂/寰俊锛?
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

		// Python AI 寮曟搸锛堣繛鎺ユ睜閰嶇疆锛?

		// Python AI 寮曟搸
		PythonEngineAddress: getEnv("PYTHON_ENGINE_ADDRESS", "localhost:8000"),
		PythonEngineTimeout: getDuration("PYTHON_ENGINE_TIMEOUT", 5*time.Minute),

		PayPalClientID: getEnv("PAYPAL_CLIENT_ID", ""),
		PayPalSecret:   getEnv("PAYPAL_SECRET", ""),
		PayPalSandbox:  isTruthy(getEnv("PAYPAL_SANDBOX", "")),

		PluginsConfigPath: getEnv("PLUGINS_CONFIG_PATH", "./plugins.json"),
		PluginDataDir:     getEnv("PLUGIN_DATA_DIR", "./data/plugins"),
	}

	return cfg
}

// ValidateAppSecret 鏍￠獙閮ㄧ讲涓诲瘑閽ュ己搴︼紙鈮?2 瀛楃锛岄潪寮卞€硷級銆?
func (c *Config) ValidateAppSecret() bool {
	return ValidateJWTSecret(c.AppSecret)
}

// deriveSubsecret 浠庝富瀵嗛挜娲剧敓纭畾鎬у瓙瀵嗛挜锛圚MAC-SHA256锛屽煙鍒嗙锛夈€?
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
