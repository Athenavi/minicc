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
	"github.com/athenavi/minicc/internal/broadcast"
	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/engine"
	"github.com/athenavi/minicc/internal/monitor"
	"github.com/athenavi/minicc/internal/session"
	"github.com/athenavi/minicc/internal/storage"
)

func main() {
	cfg := config.Load()

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

	// ── PostgreSQL ──
	pgConnected := false
	if len(cfg.PostgresReadDSNs) > 0 {
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
			slog.Info("database router enabled", "read_replicas", len(cfg.PostgresReadDSNs))
		}
	}

	if !pgConnected {
		// No read replicas or router failed — fall back to single pool
		if err := db.ConnectPostgres(ctx, cfg.PostgresDSN, cfg.PostgresMaxConn, cfg.PostgresMinConn); err != nil {
			slog.Warn("postgres not available", "error", err)
		} else {
			pgConnected = true
			defer db.ClosePostgres()
			if err := db.RunAtlasMigrations(ctx, db.Pool, "migrations"); err != nil {
				slog.Warn("migrations failed", "error", err)
			}
		}
	}

	// ── Redis ──
	var atomicRedis *db.AtomicRedis
	redisCfg := db.RedisConfig{
		Mode:          cfg.RedisMode,
		Addr:          cfg.RedisAddr,
		Password:      cfg.RedisPassword,
		DB:            cfg.RedisDB,
		Addrs:         cfg.RedisAddrs,
		MasterName:    cfg.RedisMasterName,
		SentinelAddrs: cfg.RedisSentinelAddrs,
	}
	redisClient, redisErr := db.NewRedisClient(redisCfg)
	if redisErr != nil {
		slog.Warn("redis not available", "error", redisErr)
	} else {
		atomicRedis = db.NewAtomicRedis(redisClient)
		db.Redis = atomicRedis
		defer atomicRedis.Close()
		slog.Info("redis initialized", "mode", cfg.RedisMode)
	}

	if !pgConnected {
		slog.Warn("running WITHOUT database — auth/login will use dev mode")
	}

	// ── Monitor ──
	monitor.Init()

	// ── Event Hub ──
	var eventHub *broadcast.Hub
	if db.Redis != nil {
		eventHub = broadcast.NewHub(db.Redis)
	} else {
		eventHub = broadcast.NewHub(nil)
	}
	defer eventHub.Close()

	// ── Python AI Engine Client ──
	var pythonClient *engine.PythonClient
	if cfg.PythonEngineAddress != "" {
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
			slog.Info("python engine configured", "addresses", addrs)
		}
	} else {
		slog.Warn("no python engine address configured — agent/graph/skill will be unavailable")
	}

	// ── RPA Browser Hub ──
	rpaHub := api.NewRPAHub()

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
	sessionMgr := session.NewManager(db.Pool, db.Redis)
	slog.Info("session manager initialized")

	// ── HTTP Server ──
	router := api.NewGatewayRouter(cfg, pythonClient, eventHub, sessionMgr, atomicStore, atomicRedis, rpaHub)
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
