package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/athenavi/minicc/internal/api"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/broadcast"
	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/engine"
	"github.com/athenavi/minicc/internal/monitor"
	"github.com/athenavi/minicc/internal/session"
	"github.com/athenavi/minicc/internal/settings"
	"github.com/athenavi/minicc/internal/storage"
)

func main() {
	// 宽松加载：APP_SECRET 缺失/弱值时不再退出（安装模式需要先配置主密钥）。
	cfg := config.LoadAllowUnconfigured()

	// Logger
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.LogLevel),
	})))

	slog.Info("starting minicc gateway", "version", "3.0.0", "port", cfg.Port)

	// Use defer+os.Exit pattern so deferred cleanups always run
	exitCode := 0
	defer func() { os.Exit(exitCode) }()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 安装模式（setup mode）：APP_SECRET 未配置（无法派生 JWT / 加密密钥，必须先配置主密钥）
	// 或 PostgreSQL 不可达（需在安装向导中配置 DSN 后重启生效）时进入；
	// 该模式下仅提供安装向导端点，业务路由返回 503。
	setupMode := !cfg.ValidateAppSecret()

	// ── install.lock 重启生效：安装完成后，用向导加密保存的 DSN/Redis 配置覆盖引导连接值 ──
	if !setupMode {
		api.ApplyInstallLockConfig(cfg)
	}

	// ── PostgreSQL ──
	pgConnected := false
	if !setupMode && len(cfg.PostgresReadDSNs) > 0 {
		// Read replicas configured — use DatabaseRouter for read/write splitting
		poolCfg := db.PoolConfig{
			MaxConns:          cfg.PostgresMaxConn,
			MinConns:          cfg.PostgresMinConn,
			MaxConnLifetime:   30 * time.Minute,
			MaxConnIdleTime:   5 * time.Minute,
			HealthCheckPeriod: 30 * time.Second,
		}
		router, err := db.NewDatabaseRouter(ctx, cfg.PostgresDSN, cfg.PostgresReadDSNs, poolCfg)
		if err != nil {
			slog.Warn("database router init failed, falling back to single pool", "error", err)
		} else {
			db.Router = router
			db.Pool = router.Write() // backward compatibility alias
			pgConnected = true
			defer router.Close()
			if err := db.RunAtlasMigrations(ctx, router.Write(), "migrations"); err != nil {
				slog.Warn("migrations failed", "error", err)
			}
			// 幂等 seed 默认租户（不依赖迁移状态；缺失时注册会违反外键 23503）
			if err := db.EnsureDefaultTenant(ctx, router.Write()); err != nil {
				slog.Warn("ensure default tenant failed", "error", err)
			}
			slog.Info("database router enabled", "read_replicas", len(cfg.PostgresReadDSNs))
		}
	}

	if !setupMode && !pgConnected {
		// No read replicas or router failed — fall back to single pool
		if err := db.ConnectPostgres(ctx, cfg.PostgresDSN, cfg.PostgresMaxConn, cfg.PostgresMinConn); err != nil {
			// 无数据库降级：进入安装模式，由安装向导配置 DSN（保存后重启生效）
			slog.Warn("failed to connect to PostgreSQL — entering setup mode; configure database via install wizard", "error", err)
		} else {
			pgConnected = true
			defer db.ClosePostgres()
			if err := db.RunAtlasMigrations(ctx, db.Pool, "migrations"); err != nil {
				slog.Warn("migrations failed", "error", err)
			}
			// 幂等 seed 默认租户（不依赖迁移状态；缺失时注册会违反外键 23503）
			if err := db.EnsureDefaultTenant(ctx, db.Pool); err != nil {
				slog.Warn("ensure default tenant failed", "error", err)
			}
		}
	}

	if !pgConnected {
		setupMode = true
		slog.Warn("SETUP MODE: PostgreSQL unavailable — only install wizard will be served")
	}

	// 幂等播种市场目录示例（技能/Agent/MCP；目录非空则跳过）
	if pgConnected {
		if err := db.SeedMarketCatalog(ctx, db.Pool); err != nil {
			slog.Warn("seed market catalog failed", "error", err)
		}
	}

	// 引导：连上数据库后，读取后台已持久化的基础设施/业务配置覆盖 cfg。
	// 使后续 Redis/存储/路由初始化使用 DB 值——支持仅凭 APP_SECRET 切换 Redis 集群等，重启生效。
	if pgConnected {
		applyDBSettingsAfterConnect(cfg)
	}

	// ── Redis ──
	// 产品决策(2026-08-22)「Redis 必需、无降级」已修订：Redis 不可用时降级运行
	// （内存限流、无会话热缓存/广播/审计流）；数据库缺失时安装模式完全不需要 Redis。
	var atomicRedis *db.AtomicRedis
	redisCfg := db.RedisConfig{
		Mode:          cfg.RedisMode,
		Addr:          cfg.RedisAddr,
		Password:      cfg.RedisPassword,
		DB:            cfg.RedisDB,
		Addrs:         cfg.RedisAddrs,
		MasterName:    cfg.RedisMasterName,
		SentinelAddrs: cfg.RedisSentinelAddrs,
		PoolSize:      cfg.RedisPoolSize,
	}
	redisClient, redisErr := db.NewRedisClient(redisCfg)
	if redisErr != nil {
		slog.Warn("Redis unavailable — degraded mode (no distributed rate limit / session cache / broadcast / audit stream)", "error", redisErr)
	} else {
		atomicRedis = db.NewAtomicRedis(redisClient)
		db.Redis = atomicRedis
		defer atomicRedis.Close()
		slog.Info("redis initialized", "mode", cfg.RedisMode)
	}

	// ── Audit Consumer: Redis Stream audit:events → PG audit_logs 批量落库 ──
	if db.Redis != nil {
		auditSink := db.NewDefaultAuditSink()
		defer auditSink.Close()
		auditCtx, auditCancel := context.WithCancel(context.Background())
		defer auditCancel()
		go func() { _ = db.NewAuditConsumer(db.Redis, auditSink.Handle).Start(auditCtx) }()
		slog.Info("audit consumer started", "stream", "audit:events")
	}

	// ── Monitor ──
	monitor.Init()

	// ── Auth: Initialize JWT authenticator ──
	// 安装模式下 APP_SECRET 未配置、JWT 密钥不可派生，跳过认证初始化；
	// 安装完成（Step 3 要求 APP_SECRET 有效）重启后按正常模式初始化。
	if setupMode {
		slog.Warn("auth skipped: setup mode (APP_SECRET 未配置，安装完成后重启生效)")
	} else {
		auth.InitJWTAuth()
		if !config.ValidateJWTSecret(cfg.JWTSecret) {
			slog.Error("FATAL: JWT_SECRET is weak or not set. Generate a strong secret (32+ chars) and set JWT_SECRET env var")
			exitCode = 1
			return
		}
		slog.Info("auth initialized", "jwt_secret_set", cfg.JWTSecret != "")
	}

	// ── Rate Limiter: initialized per-router in GatewayRouter ──
	slog.Info("rate limiter configured", "default_rpm", cfg.RateLimitRPM)

	// ── Event Hub ──
	var eventHub *broadcast.Hub
	if !setupMode {
		if db.Redis != nil {
			eventHub = broadcast.NewHub(db.Redis)
		} else {
			eventHub = broadcast.NewHub(nil)
		}
		defer eventHub.Close()
	}

	// ── Python AI Engine Client ──
	// 安装模式跳过：INTERNAL_TOKEN 派生自 APP_SECRET，未配置时无法建立可信通道。
	var pythonClient *engine.PythonClient
	if !setupMode && cfg.PythonEngineAddress != "" {
		// Support comma-separated addresses for multi-instance deployment
		var addrs []string
		for _, a := range strings.Split(cfg.PythonEngineAddress, ",") {
			a = strings.TrimSpace(a)
			if a != "" {
				if !strings.HasPrefix(a, "http://") && !strings.HasPrefix(a, "https://") {
					a = "http://" + a
				}
				addrs = append(addrs, a)
			}
		}
		if len(addrs) > 0 {
			pythonClient = engine.NewPythonClient(addrs...)
			pythonClient.SetInternalToken(cfg.InternalToken)
			if cfg.InternalToken == "" {
				slog.Error("INTERNAL_TOKEN not set but python engine is configured — refusing to start (set INTERNAL_TOKEN env var or remove PYTHON_ENGINE_ADDRESS)")
				exitCode = 1
				return
			}
			api.StartCronScheduler(ctx, pythonClient)
			slog.Info("python engine configured", "addresses", addrs)
		}
	} else if !setupMode {
		slog.Warn("no python engine address configured — agent/graph/skill will be unavailable")
	}

	// ── RPA Browser Hub ──
	rpaHub := api.NewRPAHub()

	// ── Storage / Session Manager / HTTP（安装模式：跳过存储与会话，仅提供安装向导）──
	var sessionMgr *session.Manager
	var router http.Handler
	if setupMode {
		// 安装令牌（Jenkins 模式）：APP_SECRET 未配置时进程内随机生成并打印到日志，
		// 部署者凭令牌访问 /install?token=xxx；安装完成后端点关闭。
		token := api.InitInstallToken(cfg)
		slog.Warn("SETUP MODE: 系统未配置数据库/主密钥，仅提供安装向导（其余路由返回 503）",
			"install_url", "/install?token="+token)
		sessionMgr = session.NewManager(nil, nil)
		router = api.NewSetupRouter(cfg)
	} else {
		// ── Storage ──
		fileStore, err := storage.NewStore(cfg.StorageBackend, cfg.StorageRoot, cfg.S3Endpoint, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3UseSSL)
		if err != nil {
			slog.Error("file store init", "error", err)
			exitCode = 1
			return
		}
		atomicStore := storage.NewAtomicStore(fileStore)
		slog.Info("storage initialized", "backend", cfg.StorageBackend)

		// ── Session Manager ──
		sessionMgr = session.NewManager(db.Pool, db.Redis)
		slog.Info("session manager initialized")

		// ── Background Maintenance ──
		api.StartBlacklistCleaner(ctx)

		router = api.NewGatewayRouter(cfg, pythonClient, eventHub, sessionMgr, atomicStore, atomicRedis, rpaHub)
	}

	// ── HTTP Server ──
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			exitCode = 1
			done <- syscall.SIGQUIT
			return
		}
	}()

	<-done
	slog.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("server stopped")
}

// applyDBSettingsAfterConnect 在完成首次数据库连接后，读取 system_settings 中
// 已持久化的基础设施/业务配置并覆盖 cfg，使后续 Redis/存储/路由初始化使用 DB 值。
// 依赖：db.Pool 已就绪、cfg.AppSecret 已校验。
// 作用范围：仅影响进程「后续初始化」使用的配置（Redis 集群、CORS、存储、S3、
// Agent、限流、支付）；DB 连接本身使用 env/默认引导串，切换数据库集群需重启。
func applyDBSettingsAfterConnect(cfg *config.Config) {
	if db.Pool == nil {
		return
	}
	store := settings.New(db.Pool, cfg.AppSecret)
	ctx := context.Background()

	apply := func(category string, fn func(map[string]interface{})) {
		m, err := store.LoadConfig(ctx, category)
		if err != nil || len(m) == 0 {
			return
		}
		fn(m)
		slog.Info("applied settings from DB", "category", category)
	}

	apply("redis", func(m map[string]interface{}) {
		if v, ok := m["addr"].(string); ok && v != "" {
			cfg.RedisAddr = v
		}
		if v, ok := m["password"].(string); ok {
			cfg.RedisPassword = v
		}
		if v, ok := m["db"].(float64); ok {
			cfg.RedisDB = int(v)
		}
		if v, ok := m["mode"].(string); ok && v != "" {
			cfg.RedisMode = v
		}
	})

	apply("cors", func(m map[string]interface{}) {
		if v, ok := m["origins"].(string); ok && v != "" {
			cfg.CORSOrigins = v
		}
	})

	apply("storage", func(m map[string]interface{}) {
		if v, ok := m["backend"].(string); ok && v != "" {
			cfg.StorageBackend = v
		}
		if v, ok := m["root"].(string); ok && v != "" {
			cfg.StorageRoot = v
		}
	})

	apply("s3", func(m map[string]interface{}) {
		if v, ok := m["endpoint"].(string); ok && v != "" {
			cfg.S3Endpoint = v
		}
		if v, ok := m["bucket"].(string); ok && v != "" {
			cfg.S3Bucket = v
		}
		if v, ok := m["access_key"].(string); ok {
			cfg.S3AccessKey = v
		}
		if v, ok := m["secret_key"].(string); ok {
			cfg.S3SecretKey = v
		}
		if v, ok := m["use_ssl"].(bool); ok {
			cfg.S3UseSSL = v
		}
	})

	apply("agent", func(m map[string]interface{}) {
		if v, ok := m["max_turns"].(float64); ok && v > 0 {
			cfg.AgentMaxTurns = int(v)
		}
		if v, ok := m["max_tokens"].(float64); ok && v > 0 {
			cfg.AgentMaxTokens = int(v)
		}
		if v, ok := m["context_limit"].(float64); ok && v > 0 {
			cfg.AgentContextLimit = int(v)
		}
	})

	apply("rate_limit", func(m map[string]interface{}) {
		if v, ok := m["global"].(float64); ok && v > 0 {
			cfg.RateLimitGlobal = int(v)
		}
	})

	apply("payment", func(m map[string]interface{}) {
		if v, ok := m["public_base_url"].(string); ok && v != "" {
			cfg.PublicBaseURL = v
		}
		if v, ok := m["alipay_gateway"].(string); ok && v != "" {
			cfg.AlipayGateway = v
		}
	})
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
