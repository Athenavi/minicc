package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/athenavi/minicc/internal/agent"
	"github.com/athenavi/minicc/internal/api"
	"github.com/athenavi/minicc/internal/broadcast"
	"github.com/athenavi/minicc/internal/cache"
	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/graph"
	"github.com/athenavi/minicc/internal/llm"
	"github.com/athenavi/minicc/internal/monitor"
	"github.com/athenavi/minicc/internal/pm"
	"github.com/athenavi/minicc/internal/session"
	"github.com/athenavi/minicc/internal/tools"
)

func main() {
	cfg := config.Load()

	// Logger
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.LogLevel),
	})))

	slog.Info("starting minicc v2", "version", "2.0.0", "port", cfg.Port)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// ── PostgreSQL ──
	pgConnected := false
	if err := db.ConnectPostgres(ctx, cfg.PostgresDSN, cfg.PostgresMaxConn, cfg.PostgresMinConn); err != nil {
		slog.Warn("postgres not available", "error", err)
	} else {
		pgConnected = true
		defer db.ClosePostgres()
		// Atlas-compatible migrations (in transaction per migration, checksum-verified)
		if err := db.RunAtlasMigrations(ctx, db.Pool, "migrations"); err != nil {
			slog.Warn("migrations failed", "error", err)
		}
	}

	// ── Redis ──
	if err := db.ConnectRedis(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB); err != nil {
		slog.Warn("redis not available", "error", err)
	} else {
		defer db.CloseRedis()
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

	// ── LLM Gateway ──
	llmGateway := llm.NewGateway(5 * time.Minute)
	if cfg.LLMAPIKey != "" {
		llmGateway.AddProvider(llm.NewOpenAIProvider(cfg.LLMAPIKey, cfg.LLMBaseURL, cfg.LLMModel))
	} else {
		slog.Warn("no LLM API key configured — chat will return error responses")
	}

	// ── Tool Registry ──
	toolRegistry := tools.NewToolRegistry()
	tools.RegisterCommonTools(toolRegistry, cfg.StorageRoot)
	slog.Info("tools registered", "count", len(toolRegistry.List()))

	// ── Agent Registry ──
	agentRegistry := agent.NewRegistry()
	agent.RegisterDefaults(agentRegistry)
	agentSessionMgr := agent.NewSessionManager()
	agent.RegisterTools(toolRegistry, agentRegistry, agentSessionMgr)
	slog.Info("agents registered", "count", len(agentRegistry.List()), "tools", 5)

	// ── Graph Engine ──
	graph.RegisterTools(toolRegistry)
	slog.Info("graph tools registered")

	// ── PM Tools ──
	pm.RegisterTools(toolRegistry)
	slog.Info("pm tools registered")

	// ── Shell Tools ──
	tools.RegisterShellTools(toolRegistry, nil)
	slog.Info("shell tools registered")

	// ── Web Tools ──
	tools.RegisterWebTools(toolRegistry)
	slog.Info("web tools registered")

	// ── Search Tools ──
	tools.RegisterSearchTools(toolRegistry, cfg.StorageRoot)
	slog.Info("search tools registered")

	// ── Enterprise Tools ──
	tools.RegisterEnterpriseTools(toolRegistry)
	slog.Info("enterprise tools registered")

	// ── DevOps Tools ──
	tools.RegisterDevOpsTools(toolRegistry)
	slog.Info("devops tools registered")

	// ── RPA Tools ──
	tools.RegisterRPATools(toolRegistry)
	slog.Info("rpa tools registered")

	slog.Info("all tools registered", "total", len(toolRegistry.List()))

	// ── Session Manager ──
	sessionMgr := session.NewManager()
	slog.Info("session manager initialized")

	// ── Redis Cache ──
	var redisCache *cache.Cache
	if db.Redis != nil {
		redisCache = cache.New(db.Redis, "minicc:cache:", 15*time.Minute)
		slog.Info("redis cache initialized")
	} else {
		redisCache = cache.New(nil, "minicc:cache:", 15*time.Minute)
		slog.Warn("redis cache initialized in local-only mode")
	}
	_ = redisCache // available for wiring to components

	// ── LLM Rate Limiter ──
	rateLimiter := llm.NewRateLimiter(db.Redis, 100, 1000, 10000)
	rateLimiter.Cleanup(5 * time.Minute)
	slog.Info("llm rate limiter initialized")

	// ── HTTP Server ──
	router := api.NewRouter(cfg, llmGateway, toolRegistry, eventHub, agentRegistry, sessionMgr, rateLimiter)
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
			os.Exit(1)
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
