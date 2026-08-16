package config

import (
	"encoding/json"
	"os"
	"strconv"
)

// ConfigFile is the JSON-serializable subset of Config for config file.
type ConfigFile struct {
	Port         string  `json:"port,omitempty"`
	ReadTimeout  string  `json:"read_timeout,omitempty"`
	WriteTimeout string  `json:"write_timeout,omitempty"`
	IdleTimeout  string  `json:"idle_timeout,omitempty"`

	PostgresDSN     string `json:"postgres_dsn,omitempty"`
	PostgresMaxConn *int   `json:"postgres_max_conn,omitempty"`
	PostgresMinConn *int   `json:"postgres_min_conn,omitempty"`

	RedisAddr     string `json:"redis_addr,omitempty"`
	RedisPassword string `json:"redis_password,omitempty"`
	RedisDB       *int   `json:"redis_db,omitempty"`

	JWTSecret     string `json:"jwt_secret,omitempty"`
	JWTExpiration string `json:"jwt_expiration,omitempty"`
	CORSOrigins   string `json:"cors_origins,omitempty"`

	LLMProvider string `json:"llm_provider,omitempty"`
	LLMAPIKey   string `json:"llm_api_key,omitempty"`
	LLMModel    string `json:"llm_model,omitempty"`
	LLMBaseURL  string `json:"llm_base_url,omitempty"`

	StorageBackend string `json:"storage_backend,omitempty"`
	StorageRoot    string `json:"storage_root,omitempty"`
	S3Endpoint     string `json:"s3_endpoint,omitempty"`
	S3Bucket       string `json:"s3_bucket,omitempty"`
	S3AccessKey    string `json:"s3_access_key,omitempty"`
	S3SecretKey    string `json:"s3_secret_key,omitempty"`

	RateLimitRPM    *int   `json:"rate_limit_rpm,omitempty"`
	RateLimitGlobal *int   `json:"rate_limit_global,omitempty"`
	LogLevel        string `json:"log_level,omitempty"`
}

// configPath returns the config file path (default: config/config.json).
func configPath() string {
	if p := os.Getenv("CONFIG_FILE"); p != "" {
		return p
	}
	return "config/config.json"
}

// loadConfigFile reads a JSON config file and applies its values to env vars
// so they are picked up by Load(). Env vars and .env still take precedence.
func loadConfigFile() {
	path := configPath()
	// Search up the directory tree if the config file is not found at the relative path.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		found := findFileUpward(path)
		if found != path {
			path = found
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return // file not found, skip
	}

	var cf ConfigFile
	if err := json.Unmarshal(data, &cf); err != nil {
		os.Stderr.WriteString("WARN: config file " + path + " parse error: " + err.Error() + "\n")
		return
	}

	// Apply each non-zero field as if it were an env var (env takes precedence)
	setIfNot("PORT", cf.Port)
	setIfNot("READ_TIMEOUT", cf.ReadTimeout)
	setIfNot("WRITE_TIMEOUT", cf.WriteTimeout)
	setIfNot("IDLE_TIMEOUT", cf.IdleTimeout)
	setIfNot("POSTGRES_DSN", cf.PostgresDSN)
	setIfNotInt("POSTGRES_MAX_CONN", cf.PostgresMaxConn)
	setIfNotInt("POSTGRES_MIN_CONN", cf.PostgresMinConn)
	setIfNot("REDIS_ADDR", cf.RedisAddr)
	setIfNot("REDIS_PASSWORD", cf.RedisPassword)
	setIfNotInt("REDIS_DB", cf.RedisDB)
	setIfNot("JWT_SECRET", cf.JWTSecret)
	setIfNot("JWT_EXPIRATION", cf.JWTExpiration)
	setIfNot("CORS_ORIGINS", cf.CORSOrigins)
	setIfNot("LLM_PROVIDER", cf.LLMProvider)
	setIfNot("LLM_API_KEY", cf.LLMAPIKey)
	setIfNot("LLM_MODEL", cf.LLMModel)
	setIfNot("LLM_BASE_URL", cf.LLMBaseURL)
	setIfNot("STORAGE_BACKEND", cf.StorageBackend)
	setIfNot("STORAGE_ROOT", cf.StorageRoot)
	setIfNot("S3_ENDPOINT", cf.S3Endpoint)
	setIfNot("S3_BUCKET", cf.S3Bucket)
	setIfNot("S3_ACCESS_KEY", cf.S3AccessKey)
	setIfNot("S3_SECRET_KEY", cf.S3SecretKey)
	setIfNotInt("RATE_LIMIT_RPM", cf.RateLimitRPM)
	setIfNotInt("RATE_LIMIT_GLOBAL", cf.RateLimitGlobal)
	setIfNot("LOG_LEVEL", cf.LogLevel)
}

func setIfNot(key, val string) {
	if val == "" {
		return
	}
	if os.Getenv(key) == "" {
		os.Setenv(key, val)
	}
}

func setIfNotInt(key string, val *int) {
	if val == nil {
		return
	}
	if os.Getenv(key) == "" {
		os.Setenv(key, strconv.Itoa(*val))
	}
}
