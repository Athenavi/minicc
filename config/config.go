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

	// Auth
	JWTSecret     string
	JWTExpiration time.Duration

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
	RateLimitRPM    int // requests per minute per user
	RateLimitGlobal int // global requests per minute

	// Log
	LogLevel string // debug / info / warn / error

	// Stripe
	StripeSecretKey    string
	StripeWebhookSecret string
	StripePriceID      string

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
}

func Load() *Config {
	loadDotEnv()     // .env file overrides config file
	loadConfigFile() // JSON config file (lowest priority)
	cfg := &Config{
		Port:            getEnv("PORT", "8080"),
		ReadTimeout:     getDuration("READ_TIMEOUT", 10*time.Second),
		WriteTimeout:    getDuration("WRITE_TIMEOUT", 60*time.Second),
		IdleTimeout:     getDuration("IDLE_TIMEOUT", 120*time.Second),
		PostgresDSN:      getEnv("POSTGRES_DSN", "postgres://minicc:minicc@localhost:5432/minicc?sslmode=disable"),
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
		JWTSecret:       getEnv("JWT_SECRET", ""),
		JWTExpiration:   getDuration("JWT_EXPIRATION", 24*time.Hour),
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
		RateLimitRPM:    getInt("RATE_LIMIT_RPM", 100),
		RateLimitGlobal: getInt("RATE_LIMIT_GLOBAL", 10000),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		StripeSecretKey:   getEnv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret: getEnv("STRIPE_WEBHOOK_SECRET", ""),
		StripePriceID:     getEnv("STRIPE_PRICE_ID", "price_1000_credits"),
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
	return secret != "" && secret != "dev-secret-change-in-production" && len(secret) >= 16
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
