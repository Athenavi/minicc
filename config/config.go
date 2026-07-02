package config

import (
	"os"
	"strconv"
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
	return &Config{
		Port:            getEnv("PORT", "8080"),
		ReadTimeout:     getDuration("READ_TIMEOUT", 10*time.Second),
		WriteTimeout:    getDuration("WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:     getDuration("IDLE_TIMEOUT", 60*time.Second),
		PostgresDSN:     getEnv("POSTGRES_DSN", "postgres://minicc:minicc@localhost:5432/minicc?sslmode=disable"),
		PostgresMaxConn: getInt("POSTGRES_MAX_CONN", 50),
		PostgresMinConn: getInt("POSTGRES_MIN_CONN", 10),
		RedisAddr:       getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:   getEnv("REDIS_PASSWORD", ""),
		RedisDB:         getInt("REDIS_DB", 0),
		JWTSecret:       getEnv("JWT_SECRET", "dev-secret-change-in-production"),
		JWTExpiration:   getDuration("JWT_EXPIRATION", 24*time.Hour),
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
