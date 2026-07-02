package config

import (
	"bufio"
	"os"
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
	PostgresDSN     string
	PostgresMaxConn int
	PostgresMinConn int

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

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

	// Rate Limit
	RateLimitRPM    int // requests per minute per user
	RateLimitGlobal int // global requests per minute

	// Log
	LogLevel string // debug / info / warn / error
}

func Load() *Config {
	loadDotEnv()
	cfg := &Config{
		Port:            getEnv("PORT", "8080"),
		ReadTimeout:     getDuration("READ_TIMEOUT", 10*time.Second),
		WriteTimeout:    getDuration("WRITE_TIMEOUT", 60*time.Second),
		IdleTimeout:     getDuration("IDLE_TIMEOUT", 120*time.Second),
		PostgresDSN:     getEnv("POSTGRES_DSN", "postgres://minicc:minicc@localhost:5432/minicc?sslmode=disable"),
		PostgresMaxConn: getInt("POSTGRES_MAX_CONN", 50),
		PostgresMinConn: getInt("POSTGRES_MIN_CONN", 10),
		RedisAddr:       getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:   getEnv("REDIS_PASSWORD", ""),
		RedisDB:         getInt("REDIS_DB", 0),
		JWTSecret:       getEnv("JWT_SECRET", ""),
		JWTExpiration:   getDuration("JWT_EXPIRATION", 24*time.Hour),
		CORSOrigins:     getEnv("CORS_ORIGINS", "http://localhost:3000"),
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
		RateLimitRPM:    getInt("RATE_LIMIT_RPM", 100),
		RateLimitGlobal: getInt("RATE_LIMIT_GLOBAL", 10000),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
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

// loadDotEnv reads .env file and sets environment variables if not already set.
func loadDotEnv() {
	data, err := os.ReadFile(".env")
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
