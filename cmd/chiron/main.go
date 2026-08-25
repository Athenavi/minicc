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

	"github.com/athenavi/chiron/internal/api"
	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/broadcast"
	"github.com/athenavi/chiron/config"
	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/engine"
	"github.com/athenavi/chiron/internal/monitor"
	"github.com/athenavi/chiron/internal/session"
	"github.com/athenavi/chiron/internal/settings"
	"github.com/athenavi/chiron/internal/storage"
)

func main() {
	// 瀹芥澗鍔犺浇锛欰PP_SECRET 缂哄け/寮卞€兼椂涓嶅啀閫€鍑猴紙瀹夎妯″紡闇€瑕佸厛閰嶇疆涓诲瘑閽ワ級銆?
	cfg := config.LoadAllowUnconfigured()

	// Logger
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.LogLevel),
	})))

	slog.Info("starting chiron gateway", "version", "0.1.260825.01", "port", cfg.Port)

	// Use defer+os.Exit pattern so deferred cleanups always run
	exitCode := 0
	defer func() { os.Exit(exitCode) }()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 瀹夎妯″紡锛坰etup mode锛夛細APP_SECRET 鏈厤缃紙鏃犳硶娲剧敓 JWT / 鍔犲瘑瀵嗛挜锛屽繀椤诲厛閰嶇疆涓诲瘑閽ワ級
	// 鎴?PostgreSQL 涓嶅彲杈撅紙闇€鍦ㄥ畨瑁呭悜瀵间腑閰嶇疆 DSN 鍚庨噸鍚敓鏁堬級鏃惰繘鍏ワ紱
	// 璇ユā寮忎笅浠呮彁渚涘畨瑁呭悜瀵肩鐐癸紝涓氬姟璺敱杩斿洖 503銆?
	setupMode := !cfg.ValidateAppSecret()

	// 鈹€鈹€ install.lock 閲嶅惎鐢熸晥锛氬畨瑁呭畬鎴愬悗锛岀敤鍚戝鍔犲瘑淇濆瓨鐨?DSN/Redis 閰嶇疆瑕嗙洊寮曞杩炴帴鍊?鈹€鈹€
	if !setupMode {
		api.ApplyInstallLockConfig(cfg)
	}

	// 鈹€鈹€ PostgreSQL 鈹€鈹€
	pgConnected := false
	if !setupMode && len(cfg.PostgresReadDSNs) > 0 {
		// Read replicas configured 鈥?use DatabaseRouter for read/write splitting
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
			// 骞傜瓑 seed 榛樿绉熸埛锛堜笉渚濊禆杩佺Щ鐘舵€侊紱缂哄け鏃舵敞鍐屼細杩濆弽澶栭敭 23503锛?
			if err := db.EnsureDefaultTenant(ctx, router.Write()); err != nil {
				slog.Warn("ensure default tenant failed", "error", err)
			}
			slog.Info("database router enabled", "read_replicas", len(cfg.PostgresReadDSNs))
		}
	}

	if !setupMode && !pgConnected {
		// No read replicas or router failed 鈥?fall back to single pool
		if err := db.ConnectPostgres(ctx, cfg.PostgresDSN, cfg.PostgresMaxConn, cfg.PostgresMinConn); err != nil {
			// 鏃犳暟鎹簱闄嶇骇锛氳繘鍏ュ畨瑁呮ā寮忥紝鐢卞畨瑁呭悜瀵奸厤缃?DSN锛堜繚瀛樺悗閲嶅惎鐢熸晥锛?
			slog.Warn("failed to connect to PostgreSQL 鈥?entering setup mode; configure database via install wizard", "error", err)
		} else {
			pgConnected = true
			defer db.ClosePostgres()
			if err := db.RunAtlasMigrations(ctx, db.Pool, "migrations"); err != nil {
				slog.Warn("migrations failed", "error", err)
			}
			// 骞傜瓑 seed 榛樿绉熸埛锛堜笉渚濊禆杩佺Щ鐘舵€侊紱缂哄け鏃舵敞鍐屼細杩濆弽澶栭敭 23503锛?
			if err := db.EnsureDefaultTenant(ctx, db.Pool); err != nil {
				slog.Warn("ensure default tenant failed", "error", err)
			}
		}
	}

	if !pgConnected {
		setupMode = true
		slog.Warn("SETUP MODE: PostgreSQL unavailable 鈥?only install wizard will be served")
	}

	// 骞傜瓑鎾甯傚満鐩綍绀轰緥锛堟妧鑳?Agent/MCP锛涚洰褰曢潪绌哄垯璺宠繃锛?
	if pgConnected {
		if err := db.SeedMarketCatalog(ctx, db.Pool); err != nil {
			slog.Warn("seed market catalog failed", "error", err)
		}
	}

	// 寮曞锛氳繛涓婃暟鎹簱鍚庯紝璇诲彇鍚庡彴宸叉寔涔呭寲鐨勫熀纭€璁炬柦/涓氬姟閰嶇疆瑕嗙洊 cfg銆?
	// 浣垮悗缁?Redis/瀛樺偍/璺敱鍒濆鍖栦娇鐢?DB 鍊尖€斺€旀敮鎸佷粎鍑?APP_SECRET 鍒囨崲 Redis 闆嗙兢绛夛紝閲嶅惎鐢熸晥銆?
	if pgConnected {
		applyDBSettingsAfterConnect(cfg)
	}

	// 鈹€鈹€ Redis 鈹€鈹€
	// 浜у搧鍐崇瓥(2026-08-22)銆孯edis 蹇呴渶銆佹棤闄嶇骇銆嶅凡淇锛歊edis 涓嶅彲鐢ㄦ椂闄嶇骇杩愯
	// 锛堝唴瀛橀檺娴併€佹棤浼氳瘽鐑紦瀛?骞挎挱/瀹¤娴侊級锛涙暟鎹簱缂哄け鏃跺畨瑁呮ā寮忓畬鍏ㄤ笉闇€瑕?Redis銆?
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
		slog.Warn("Redis unavailable 鈥?degraded mode (no distributed rate limit / session cache / broadcast / audit stream)", "error", redisErr)
	} else {
		atomicRedis = db.NewAtomicRedis(redisClient)
		db.Redis = atomicRedis
		defer atomicRedis.Close()
		slog.Info("redis initialized", "mode", cfg.RedisMode)
	}

	// 鈹€鈹€ Audit Consumer: Redis Stream audit:events 鈫?PG audit_logs 鎵归噺钀藉簱 鈹€鈹€
	if db.Redis != nil {
		auditSink := db.NewDefaultAuditSink()
		defer auditSink.Close()
		auditCtx, auditCancel := context.WithCancel(context.Background())
		defer auditCancel()
		go func() { _ = db.NewAuditConsumer(db.Redis, auditSink.Handle).Start(auditCtx) }()
		slog.Info("audit consumer started", "stream", "audit:events")
	}

	// 鈹€鈹€ Monitor 鈹€鈹€
	monitor.Init()

	// 鈹€鈹€ Auth: Initialize JWT authenticator 鈹€鈹€
	// 瀹夎妯″紡涓?APP_SECRET 鏈厤缃€丣WT 瀵嗛挜涓嶅彲娲剧敓锛岃烦杩囪璇佸垵濮嬪寲锛?
	// 瀹夎瀹屾垚锛圫tep 3 瑕佹眰 APP_SECRET 鏈夋晥锛夐噸鍚悗鎸夋甯告ā寮忓垵濮嬪寲銆?
	if setupMode {
		slog.Warn("auth skipped: setup mode (APP_SECRET 鏈厤缃紝瀹夎瀹屾垚鍚庨噸鍚敓鏁?")
	} else {
		auth.InitJWTAuth()
		if !config.ValidateJWTSecret(cfg.JWTSecret) {
			slog.Error("FATAL: JWT_SECRET is weak or not set. Generate a strong secret (32+ chars) and set JWT_SECRET env var")
			exitCode = 1
			return
		}
		slog.Info("auth initialized", "jwt_secret_set", cfg.JWTSecret != "")
	}

	// 鈹€鈹€ Rate Limiter: initialized per-router in GatewayRouter 鈹€鈹€
	slog.Info("rate limiter configured", "default_rpm", cfg.RateLimitRPM)

	// 鈹€鈹€ Event Hub 鈹€鈹€
	var eventHub *broadcast.Hub
	if !setupMode {
		if db.Redis != nil {
			eventHub = broadcast.NewHub(db.Redis)
		} else {
			eventHub = broadcast.NewHub(nil)
		}
		defer eventHub.Close()
	}

	// 鈹€鈹€ Python AI Engine Client 鈹€鈹€
	// 瀹夎妯″紡璺宠繃锛欼NTERNAL_TOKEN 娲剧敓鑷?APP_SECRET锛屾湭閰嶇疆鏃舵棤娉曞缓绔嬪彲淇￠€氶亾銆?
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
				slog.Error("INTERNAL_TOKEN not set but python engine is configured 鈥?refusing to start (set INTERNAL_TOKEN env var or remove PYTHON_ENGINE_ADDRESS)")
				exitCode = 1
				return
			}
			api.StartCronScheduler(ctx, pythonClient)
			slog.Info("python engine configured", "addresses", addrs)
		}
	} else if !setupMode {
		slog.Warn("no python engine address configured 鈥?agent/graph/skill will be unavailable")
	}

	// 鈹€鈹€ RPA Browser Hub 鈹€鈹€
	rpaHub := api.NewRPAHub()

	// 鈹€鈹€ Storage / Session Manager / HTTP锛堝畨瑁呮ā寮忥細璺宠繃瀛樺偍涓庝細璇濓紝浠呮彁渚涘畨瑁呭悜瀵硷級鈹€鈹€
	var sessionMgr *session.Manager
	var router http.Handler
	if setupMode {
		// 瀹夎浠ょ墝锛圝enkins 妯″紡锛夛細APP_SECRET 鏈厤缃椂杩涚▼鍐呴殢鏈虹敓鎴愬苟鎵撳嵃鍒版棩蹇楋紝
		// 閮ㄧ讲鑰呭嚟浠ょ墝璁块棶 /install?token=xxx锛涘畨瑁呭畬鎴愬悗绔偣鍏抽棴銆?
		token := api.InitInstallToken(cfg)
		slog.Warn("SETUP MODE: 绯荤粺鏈厤缃暟鎹簱/涓诲瘑閽ワ紝浠呮彁渚涘畨瑁呭悜瀵硷紙鍏朵綑璺敱杩斿洖 503锛?,
			"install_url", "/install?token="+token)
		sessionMgr = session.NewManager(nil, nil)
		router = api.NewSetupRouter(cfg)
	} else {
		// 鈹€鈹€ Storage 鈹€鈹€
		fileStore, err := storage.NewStore(cfg.StorageBackend, cfg.StorageRoot, cfg.S3Endpoint, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3UseSSL)
		if err != nil {
			slog.Error("file store init", "error", err)
			exitCode = 1
			return
		}
		atomicStore := storage.NewAtomicStore(fileStore)
		slog.Info("storage initialized", "backend", cfg.StorageBackend)

		// 鈹€鈹€ Session Manager 鈹€鈹€
		sessionMgr = session.NewManager(db.Pool, db.Redis)
		slog.Info("session manager initialized")

		// 鈹€鈹€ Background Maintenance 鈹€鈹€
		api.StartBlacklistCleaner(ctx)

		router = api.NewGatewayRouter(cfg, pythonClient, eventHub, sessionMgr, atomicStore, atomicRedis, rpaHub)
	}

	// 鈹€鈹€ HTTP Server 鈹€鈹€
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

// applyDBSettingsAfterConnect 鍦ㄥ畬鎴愰娆℃暟鎹簱杩炴帴鍚庯紝璇诲彇 system_settings 涓?
// 宸叉寔涔呭寲鐨勫熀纭€璁炬柦/涓氬姟閰嶇疆骞惰鐩?cfg锛屼娇鍚庣画 Redis/瀛樺偍/璺敱鍒濆鍖栦娇鐢?DB 鍊笺€?
// 渚濊禆锛歞b.Pool 宸插氨缁€乧fg.AppSecret 宸叉牎楠屻€?
// 浣滅敤鑼冨洿锛氫粎褰卞搷杩涚▼銆屽悗缁垵濮嬪寲銆嶄娇鐢ㄧ殑閰嶇疆锛圧edis 闆嗙兢銆丆ORS銆佸瓨鍌ㄣ€丼3銆?
// Agent銆侀檺娴併€佹敮浠橈級锛汥B 杩炴帴鏈韩浣跨敤 env/榛樿寮曞涓诧紝鍒囨崲鏁版嵁搴撻泦缇ら渶閲嶅惎銆?
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

