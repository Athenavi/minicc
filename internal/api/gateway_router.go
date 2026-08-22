package api

import (
	"sync"
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/billing"
	"github.com/athenavi/minicc/internal/broadcast"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/engine"
	"github.com/athenavi/minicc/internal/session"
	"github.com/athenavi/minicc/internal/storage"
)

var startTime = time.Now()

// sessionCancels tracks running session contexts for cancellation support.
var sessionCancels sync.Map

// sessionCancel tracks the owner and cancel function of a running session task.
type sessionCancel struct {
	userID string
	cancel context.CancelFunc
}

// routeMiddleware is a middleware wrapper used by route registration helpers.
type routeMiddleware func(http.Handler) http.Handler

// middlewareChain wraps an http.Handler with a list of middleware functions.
func middlewareChain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// requestIDHeader generates a lightweight request ID.
func requestIDHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf [8]byte
		rand.Read(buf[:])
		id := hex.EncodeToString(buf[:])
		r.Header.Set("X-Request-ID", id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

// realIPHeader extracts the real IP from X-Forwarded-For or X-Real-IP.
func realIPHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			if idx := strings.Index(fwd, ","); idx >= 0 {
				r.RemoteAddr = strings.TrimSpace(fwd[:idx])
			} else {
				r.RemoteAddr = strings.TrimSpace(fwd)
			}
		} else if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
			r.RemoteAddr = realIP
		}
		next.ServeHTTP(w, r)
	})
}

// NewGatewayRouter creates a pure gateway router that proxies all business logic to Python.
func NewGatewayRouter(
	cfg *config.Config,
	pythonClient *engine.PythonClient,
	eventHub *broadcast.Hub,
	sessionMgr *session.Manager,
	fileStore *storage.AtomicStore,
	atomicRedis *db.AtomicRedis,
	rpaHub *RPAHub,
) http.Handler {
	mux := http.NewServeMux()

	// Rate limiter — 当 Redis 可用时使用分布式限流器，否则使用本地限流
	var rlMW func(http.Handler) http.Handler
	if atomicRedis != nil {
		distLimiter := NewDistributedRateLimiter(
			atomicRedis.LoadRaw(),
			cfg.RateLimitRPM*10,    // 全局：单实例限制 × 10
			cfg.RateLimitRPM*5,     // 租户：单实例限制 × 5
			cfg.RateLimitRPM,       // 用户：单实例限制
		)
		rlMW = DistributedRateLimitMiddleware(distLimiter)
		slog.Info("distributed rate limiter enabled", "global", cfg.RateLimitRPM*10)
	} else {
		rateLimiter := NewRateLimiter(cfg.RateLimitRPM)
		rateLimiter.CleanupVisitors(5 * time.Minute)
		rlMW = rateLimiter.Middleware
		slog.Info("local rate limiter enabled (no Redis)", "rpm", cfg.RateLimitRPM)
	}

	// Input sanitizer (prompt injection protection)
	inputSanitizer := NewInputSanitizer()
	sanitizeMW := SanitizeMiddleware(inputSanitizer)

	publicMW := func(next http.Handler) http.Handler {
		return middlewareChain(next,
			RecoverMiddleware,
			TracingMiddleware,
			LoggingMiddleware,
			SecurityHeadersMiddleware,
			CORSMiddleware(cfg.CORSOrigins),
			MonitoringMiddleware,
			requestIDHeader,
			realIPHeader,
		)
	}

	// Auth
	authenticator := auth.NewAuthenticator(cfg.JWTSecret, cfg.JWTExpiration)
	authHandler := NewAuthHandler(cfg)
	authMW := AuthMiddleware(authenticator)

	// SSO 三方登录 + 人机验证（防接口滥用）+ 短信验证码登录
	ssoHandler := NewSSOHandler(authenticator, cfg)
	captchaHandler := NewCaptchaHandler(cfg)
	authHandler.SetCaptchaHandler(captchaHandler)
	smsHandler := NewSmsHandler(authenticator, cfg, captchaHandler)

	// Billing
	billingStore := billing.NewPGStore()
	billingStore.EnsureTables(context.Background())
	billingMgr := billing.NewManager(billingStore)
	billingMgr.Subscribe(billing.NewTransactionRecorder(billingStore))
	billingMgr.Subscribe(billing.NewBalanceSyncer(billingStore, 5*time.Second))

	// Agent execution semaphore
	agentSem := make(chan struct{}, 20)

	// Submit handler (proxies to Python)
	submitHandler := NewSubmitHandler(pythonClient, sessionMgr, eventHub, billingMgr)

	// Install
	installHandler := NewInstallHandler(cfg)

	// Plugins (MCP server config management)
	pluginHandler := NewPluginHandler(cfg, authenticator)

	// Search (auth + rate limited)
	searchHandler := NewSearchHandler()

	// Editor
	editorHandler := NewEditorHandler(cfg.StorageRoot)

	// Conversation
	conversationHandler := NewConversationHandler(authenticator, sessionMgr)
	shareHandler := NewShareHandler(authenticator, sessionMgr)

	// Tool (proxies to Python)
	toolHandler := NewToolHandler(pythonClient, authenticator)

	// System
	systemHandler := NewSystemHandler()

	// Media
	mediaHandler := NewMediaHandler(fileStore, authenticator)

	// 通用分片上传（断点续传）
	uploadHandler := NewUploadHandler(authenticator, cfg.StorageRoot)
	uploadHandler.RegisterRoutes(mux, authMW, rlMW)

	// Billing handler (uses the same billingMgr as /submit to avoid split-brain cache)
	billingHandler := NewBillingHandler(billingMgr, authenticator, cfg)

	// Skill handler (proxies to Python)
	skillHandler := NewSkillHandler(pythonClient)

	// Mode + Permission (no Python dependency)
	modeStore := NewModeStore()
	permMgr := NewPermissionManager()
	modeHandler := NewModeHandler(modeStore, permMgr, eventHub)

	// Trace handler (Redis-backed, tenant-isolated)
	var traceHandler *TraceHandler
	if atomicRedis != nil {
		traceHandler = NewTraceHandler(atomicRedis.LoadRaw())
	}

	// Knowledge base — proxied to Python engine
	// SaaS 安全: 知识库独立限流 (每租户 QPS=50, Burst=100)
	var kbRateRedis db.RedisClient
	if atomicRedis != nil {
		kbRateRedis = atomicRedis.LoadRaw()
	}
	kbRateLimiter := NewTenantRateLimiter(kbRateRedis, 50, 100)
	kbRateMW := kbRateLimiter.Middleware

	// Admin handler
	adminHandler := NewAdminHandler(authenticator, fileStore, atomicRedis, pythonClient)

	// ── Route registration by functional domain ──

	registerPublicEndpoints(mux, authMW, rlMW, publicMW, searchHandler, shareHandler, systemHandler)
	registerAgentRoutes(mux, authMW, rlMW, publicMW, sanitizeMW, submitHandler, billingMgr, agentSem, eventHub, sessionMgr, authenticator, rpaHub)
	registerAuthRoutes(mux, authHandler, authMW, rlMW)

	// ── SSO 三方登录（公开流程 rlMW；用户自助 authMW；管理 authMW + sso:manage）──
	ssoHandler.RegisterPublicRoutes(mux, rlMW)
	ssoHandler.RegisterUserRoutes(mux, authMW)
	ssoHandler.RegisterAdminRoutes(mux, authMW)

	// ── 人机验证：公开配置下发（登录页拉取）+ 管理配置 ──
	captchaHandler.RegisterPublicRoutes(mux, rlMW)
	captchaHandler.RegisterAdminRoutes(mux, authMW)

	// ── 短信验证码登录（公开流程 rlMW；用户自助 authMW；管理 authMW + sso:manage）──
	smsHandler.RegisterPublicRoutes(mux, rlMW)
	smsHandler.RegisterUserRoutes(mux, authMW)
	smsHandler.RegisterAdminRoutes(mux, authMW)
	registerSystemRoutes(mux, authMW, rlMW, sanitizeMW, installHandler, editorHandler, toolHandler, systemHandler, traceHandler)
	registerConversationRoutes(mux, conversationHandler, shareHandler, authMW, rlMW)
	registerMediaRoutes(mux, mediaHandler, authMW, rlMW, cfg.StorageRoot)
	registerPluginRoutes(mux, pluginHandler, authMW, rlMW)
	registerBillingRoutes(mux, billingHandler, authMW, rlMW)
	registerProxyRoutes(mux, authMW, rlMW, kbRateMW, pythonClient)

	// Agents (auth + rate limited; DB 驱动 CRUD + 运行会话)
	agentHandler := NewAgentHandler(authenticator, pythonClient)
	mux.Handle("GET /v1/agents", authMW(rlMW(http.HandlerFunc(agentHandler.List))))
	mux.Handle("POST /v1/agents", authMW(rlMW(http.HandlerFunc(agentHandler.Create))))
	mux.Handle("GET /v1/agents/{id}", authMW(rlMW(http.HandlerFunc(agentHandler.Get))))
	mux.Handle("PUT /v1/agents/{id}", authMW(rlMW(http.HandlerFunc(agentHandler.Update))))
	mux.Handle("DELETE /v1/agents/{id}", authMW(rlMW(http.HandlerFunc(agentHandler.Delete))))
	mux.Handle("POST /v1/agents/{id}/run", authMW(rlMW(http.HandlerFunc(agentHandler.Run))))
	mux.Handle("GET /v1/agents/sessions", authMW(rlMW(http.HandlerFunc(agentHandler.ListSessions))))
	mux.Handle("GET /v1/agents/sessions/{id}", authMW(rlMW(http.HandlerFunc(agentHandler.GetSession))))
	// dispatch 保留 Python 代理（agent 工具链内部调用，非页面主链路）
	// 安全：必须经过 authMW，否则未认证可触发工具执行
	mux.Handle("POST /v1/agents/dispatch", authMW(rlMW(sanitizeMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			InternalError(w, "python engine not available")
			return
		}
		var body map[string]interface{}
		if err := DecodeJSON(w, r, &body); err != nil {
			BadRequest(w, "invalid request")
			return
		}
		var resp map[string]interface{}
		if err := pythonClient.PostJSON(r.Context(), "/v1/agents/dispatch", body, &resp); err != nil {
			InternalError(w, "python agent dispatch failed")
			return
		}
		OK(w, resp)
	})))))

	// Skills (rate limited, proxies to Python)
	skillHandler.RegisterRoutes(mux)

	// Enterprise audit (auth + RequireEntPerm("audit:read"))
	NewEntAuditHandler().RegisterRoutes(mux, authMW)

	// Enterprise identity (auth + RequireEntPerm("ent:manage"))：用户/角色/群组/租户
	NewEntIdentityHandler().RegisterRoutes(mux, authMW)

	// Enterprise cost center（authMW，内部按 PermAdminRead/Write 分级）
	NewEntCostCenterHandler(nil, nil).RegisterRoutes(mux, authMW)

	// Enterprise policy（authMW + RequireEntPerm("policy:manage")）：隐私模式 + 模型策略
	NewEntPolicyHandler().RegisterRoutes(mux, authMW)

	// Enterprise market（authMW + RequireEntPerm("market:manage")）：能力市场 + 租户授权
	NewMarketHandler().RegisterRoutes(mux, authMW)

	// Mode (auth + rate limited)
	mux.Handle("GET /v1/mode", authMW(rlMW(http.HandlerFunc(modeHandler.GetMode))))
	mux.Handle("POST /v1/mode", authMW(rlMW(http.HandlerFunc(modeHandler.SetMode))))
	mux.Handle("POST /v1/permission/approve", authMW(rlMW(http.HandlerFunc(modeHandler.ApprovePermission))))
	mux.Handle("POST /v1/permission/reject", authMW(rlMW(http.HandlerFunc(modeHandler.RejectPermission))))

	registerAdminRoutes(mux, authMW, adminHandler, pythonClient)

	// Wrap main mux with public middleware
	return publicMW(mux)
}

// ── Public endpoints ──

func registerPublicEndpoints(
	mux *http.ServeMux,
	authMW, rlMW, publicMW routeMiddleware,
	searchHandler *SearchHandler,
	shareHandler *ShareHandler,
	systemHandler *SystemHandler,
) {
	mux.Handle("GET /search", authMW(rlMW(http.HandlerFunc(searchHandler.Search))))

	// Public share view (no auth; revoked shares return 410 Gone)
	mux.Handle("GET /share/{id}", rlMW(publicMW(http.HandlerFunc(shareHandler.PublicGet))))

	mux.Handle("GET /health", rlMW(publicMW(http.HandlerFunc(handleHealth))))
	// Prometheus 指标端点（公开，供 Prometheus 抓取；生产建议加内网限制）
	mux.Handle("GET /metrics", rlMW(publicMW(http.HandlerFunc(systemHandler.PrometheusMetrics))))
	mux.Handle("GET /ready", rlMW(publicMW(http.HandlerFunc(handleReadiness))))
}

// ── Agent submit/cancel/events ──

func registerAgentRoutes(
	mux *http.ServeMux,
	authMW, rlMW, publicMW, sanitizeMW routeMiddleware,
	submitHandler *SubmitHandler,
	billingMgr *billing.Manager,
	agentSem chan struct{},
	eventHub *broadcast.Hub,
	sessionMgr *session.Manager,
	authenticator *auth.Authenticator,
	rpaHub *RPAHub,
) {
	mux.Handle("POST /v1/agent/approval", authMW(rlMW(http.HandlerFunc(submitHandler.SubmitApproval))))

	mux.Handle("POST /submit", authMW(publicMW(sanitizeMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Content   string                 `json:"content"`
			SessionID string                 `json:"session_id"`
			LLMConfig map[string]interface{} `json:"llm_config"`
		}
		if err := DecodeJSON(w, r, &body); err != nil {
			BadRequest(w, "invalid request")
			return
		}
		if body.Content == "" {
			BadRequest(w, "content is required")
			return
		}
		claims := auth.GetClaims(r.Context())
		if claims == nil {
			Unauthorized(w, ErrAuthRequired)
			return
		}
		userID := claims.UserID

		// Billing pre-check
		if billingMgr != nil {
			count, err := billingMgr.DailyFreeCount(r.Context(), userID)
			overFreeQuota := err != nil || count >= billing.DailyFreeLimit
			if overFreeQuota {
				if balance, balErr := billingMgr.GetBalance(userID); balErr == nil && balance <= 0 {
					JSON(w, http.StatusPaymentRequired, APIResponse{
						Success: false,
						Error:   "insufficient credits — please recharge in Billing",
					})
					return
				}
			}
		}

		// Reject concurrent submits within the same session
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		if _, loaded := sessionCancels.LoadOrStore(body.SessionID, sessionCancel{userID: userID, cancel: cancel}); loaded {
			cancel() // cancel the new one since there's already an active task
			BadRequest(w, "task already running for this session")
			return
		}

		select {
		case agentSem <- struct{}{}:
		default:
			sessionCancels.Delete(body.SessionID)
			cancel()
			TooManyRequests(w)
			return
		}

		Accepted(w, map[string]string{"status": "accepted", "session_id": body.SessionID})
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("submit handler panic", "panic", r)
				}
			}()
			defer func() { <-agentSem }()
			defer cancel()
			defer sessionCancels.Delete(body.SessionID)
			submitHandler.HandleSubmit(ctx, userID, body.SessionID, body.Content, body.LLMConfig)
		}()
	})))))

	mux.Handle("POST /cancel", authMW(rlMW(publicMW(http.HandlerFunc(handleCancel)))))

	// SSE + WebSocket
	mux.Handle("GET /events", authMW(rlMW(SSEHandler(eventHub, sessionMgr))))
	mux.HandleFunc("GET /ws/{sessionId}", WebSocketHandler(NewWebSocketHub(), eventHub, authenticator, sessionMgr))
	mux.HandleFunc("GET /ws/rpa", RPAWebSocketHandler(rpaHub, authenticator))
}

// ── Auth (login / register / logout) ──

func registerAuthRoutes(mux *http.ServeMux, authHandler *AuthHandler, authMW, rlMW routeMiddleware) {
	// Auth (public, rate limited)
	mux.Handle("POST /v1/auth/login", rlMW(http.HandlerFunc(authHandler.Login)))
	mux.Handle("POST /v1/auth/register", rlMW(http.HandlerFunc(authHandler.Register)))
	mux.Handle("POST /v1/auth/refresh", rlMW(http.HandlerFunc(authHandler.Refresh)))
	mux.Handle("POST /v1/auth/logout", rlMW(http.HandlerFunc(authHandler.Logout)))
	// SSO cookie → Bearer token 会话引导（公开：httpOnly cookie 自带凭据）
	mux.Handle("GET /v1/auth/session", rlMW(http.HandlerFunc(authHandler.Session)))
	mux.Handle("GET /v1/auth/profile", authMW(rlMW(http.HandlerFunc(authHandler.Profile))))
	mux.Handle("PUT /v1/auth/profile", authMW(rlMW(http.HandlerFunc(authHandler.UpdateProfile))))
}

// ── System (install / editor / tools / health / trace) ──

func registerSystemRoutes(
	mux *http.ServeMux,
	authMW, rlMW, sanitizeMW routeMiddleware,
	installHandler *InstallHandler,
	editorHandler *EditorHandler,
	toolHandler *ToolHandler,
	systemHandler *SystemHandler,
	traceHandler *TraceHandler,
) {
	// Install (public, rate limited)
	mux.Handle("GET /v1/install/status", rlMW(http.HandlerFunc(installHandler.Status)))
	mux.Handle("POST /v1/install/setup", rlMW(http.HandlerFunc(installHandler.Setup)))

	// Editor (auth + rate limited)
	mux.Handle("GET /api/editor/files", authMW(rlMW(http.HandlerFunc(editorHandler.ListFiles))))
	mux.Handle("GET /api/editor/read", authMW(rlMW(http.HandlerFunc(editorHandler.ReadFile))))
	mux.Handle("POST /api/editor/write", authMW(rlMW(http.HandlerFunc(editorHandler.WriteFile))))

	// Tools (rate limited, proxies to Python)
	mux.Handle("GET /v1/tools", rlMW(http.HandlerFunc(toolHandler.ListTools)))
	mux.Handle("POST /v1/tools/execute", authMW(rlMW(sanitizeMW(http.HandlerFunc(toolHandler.ExecuteTool)))))

	// System (rate limited; spans/traces 仅管理员可见，S 安全修复：原为公开信息泄露)
	mux.Handle("GET /v1/system/health", rlMW(http.HandlerFunc(systemHandler.HealthScores)))
	mux.Handle("GET /v1/system/spans", authMW(rlMW(RequirePermission(auth.PermAdminRead)(http.HandlerFunc(systemHandler.Spans)))))
	mux.Handle("GET /v1/system/traces", authMW(rlMW(RequirePermission(auth.PermAdminRead)(http.HandlerFunc(systemHandler.Traces)))))
	mux.Handle("GET /v1/metrics", authMW(rlMW(http.HandlerFunc(systemHandler.Metrics))))

	// Trace (user-level call chain tracing, tenant-isolated)
	if traceHandler != nil {
		mux.Handle("GET /v1/traces", authMW(rlMW(http.HandlerFunc(traceHandler.ListTraces))))
		mux.Handle("GET /v1/traces/{trace_id}", authMW(rlMW(http.HandlerFunc(traceHandler.GetTrace))))
	}
}

// ── Conversations ──

func registerConversationRoutes(
	mux *http.ServeMux,
	conversationHandler *ConversationHandler,
	shareHandler *ShareHandler,
	authMW, rlMW routeMiddleware,
) {
	// Conversations (auth + rate limited)
	mux.Handle("GET /v1/conversations", authMW(rlMW(http.HandlerFunc(conversationHandler.List))))
	mux.Handle("POST /v1/conversations", authMW(rlMW(http.HandlerFunc(conversationHandler.Create))))
	mux.Handle("GET /v1/conversations/{id}", authMW(rlMW(http.HandlerFunc(conversationHandler.Get))))
	mux.Handle("PUT /v1/conversations/{id}", authMW(rlMW(http.HandlerFunc(conversationHandler.Update))))
	mux.Handle("DELETE /v1/conversations/{id}", authMW(rlMW(http.HandlerFunc(conversationHandler.Delete))))

	// Conversation shares (auth + rate limited; public GET in registerPublicEndpoints)
	mux.Handle("POST /v1/conversations/{id}/share", authMW(rlMW(http.HandlerFunc(shareHandler.Create))))
	mux.Handle("GET /v1/conversations/{id}/share", authMW(rlMW(http.HandlerFunc(shareHandler.GetActive))))
	mux.Handle("DELETE /v1/conversations/{id}/share", authMW(rlMW(http.HandlerFunc(shareHandler.Revoke))))
}

// ── Media ──

func registerMediaRoutes(
	mux *http.ServeMux,
	mediaHandler *MediaHandler,
	authMW, rlMW routeMiddleware,
	storageRoot string,
) {
	// Media (auth + rate limited)
	mux.Handle("GET /v1/media", authMW(rlMW(http.HandlerFunc(mediaHandler.List))))
	mux.Handle("POST /v1/media", authMW(rlMW(http.HandlerFunc(mediaHandler.Create))))
	mux.Handle("POST /v1/media/folders", authMW(rlMW(http.HandlerFunc(mediaHandler.CreateFolder))))
	mux.Handle("GET /v1/media/folders", authMW(rlMW(http.HandlerFunc(mediaHandler.ListFolders))))
	mux.Handle("POST /v1/media/upload", authMW(rlMW(http.HandlerFunc(mediaHandler.Upload))))
	mux.Handle("POST /v1/media/presign", authMW(rlMW(http.HandlerFunc(mediaHandler.PresignUpload))))
	mux.Handle("POST /v1/media/complete", authMW(rlMW(http.HandlerFunc(mediaHandler.CompleteUpload))))
	mux.Handle("POST /v1/media/batch-delete", authMW(rlMW(http.HandlerFunc(mediaHandler.BatchDelete))))
	mux.Handle("PUT /v1/media/{id}", authMW(rlMW(http.HandlerFunc(mediaHandler.Update))))
	mux.Handle("GET /v1/media/{id}/download", authMW(rlMW(http.HandlerFunc(mediaHandler.Download))))
	mux.Handle("POST /v1/media/{id}/share", authMW(rlMW(http.HandlerFunc(mediaHandler.Share))))
	mux.Handle("DELETE /v1/media/{id}", authMW(rlMW(http.HandlerFunc(mediaHandler.Delete))))

	// Media file serving
	mux.Handle("GET /media/", rlMW(http.StripPrefix("/media/", http.FileServer(http.Dir(storageRoot+"/media")))))
}

// ── Plugins ──

func registerPluginRoutes(mux *http.ServeMux, pluginHandler *PluginHandler, authMW, rlMW routeMiddleware) {
	// Plugins (auth + rate limited)
	mux.Handle("GET /v1/plugins", authMW(rlMW(http.HandlerFunc(pluginHandler.List))))
	mux.Handle("POST /v1/plugins/{name}/install", authMW(rlMW(http.HandlerFunc(pluginHandler.Install))))
	mux.Handle("PUT /v1/plugins/{name}", authMW(rlMW(http.HandlerFunc(pluginHandler.Update))))
	mux.Handle("POST /v1/plugins/{name}/test", authMW(rlMW(http.HandlerFunc(pluginHandler.Test))))
	mux.Handle("DELETE /v1/plugins/{name}", authMW(rlMW(http.HandlerFunc(pluginHandler.Uninstall))))
}

// ── Billing ──

func registerBillingRoutes(mux *http.ServeMux, billingHandler *BillingHandler, authMW, rlMW routeMiddleware) {
	// Billing (auth + rate limited)
	mux.Handle("GET /v1/billing/balance", authMW(rlMW(http.HandlerFunc(billingHandler.GetBalance))))
	mux.Handle("GET /v1/billing/history", authMW(rlMW(http.HandlerFunc(billingHandler.GetHistory))))
	mux.Handle("POST /v1/billing/recharge", authMW(rlMW(http.HandlerFunc(billingHandler.Recharge))))
	mux.Handle("POST /v1/billing/pay", authMW(rlMW(http.HandlerFunc(billingHandler.CreatePayment))))
	mux.Handle("GET /v1/billing/orders/{id}", authMW(rlMW(http.HandlerFunc(billingHandler.GetOrder))))
	// 支付渠道异步回调（无 auth：支付宝验签 / 微信平台证书验签 + AES-GCM 解密）
	mux.Handle("POST /v1/billing/callback/alipay", rlMW(http.HandlerFunc(billingHandler.AlipayCallback)))
	mux.Handle("POST /v1/billing/callback/wechat", rlMW(http.HandlerFunc(billingHandler.WechatCallback)))
	mux.Handle("POST /v1/billing/paypal-capture", authMW(rlMW(http.HandlerFunc(billingHandler.PayPalCapture))))
	mux.Handle("GET /v1/billing/usage", authMW(rlMW(http.HandlerFunc(billingHandler.GetUsage))))
}

// ── Python engine proxy routes (graphs / workflows / knowledge base) ──

func registerProxyRoutes(
	mux *http.ServeMux,
	authMW, rlMW, kbRateMW routeMiddleware,
	pythonClient *engine.PythonClient,
) {
	// pythonProxy is a factory for reverse-proxy handlers to the Python engine.
	// All routes using this factory are wrapped with authMW, so claims are
	// guaranteed present in context.
	type proxyOpt struct {
		methods []string // allowed HTTP methods; empty = GET+POST+PUT+DELETE
		logTag  string
	}
	newProxy := func(prefix string, opt proxyOpt) func(func(*http.Request) string) http.HandlerFunc {
		return func(buildPath func(*http.Request) string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				if pythonClient == nil || !pythonClient.IsConnected() {
					InternalError(w, "python engine not available")
					return
				}
				claims := auth.GetClaims(r.Context())
			if claims == nil || claims.TenantID == "" {
				Unauthorized(w, "missing tenant context")
				return
			}
			// 多租户隔离：透传 tenant_id 给 Python 引擎（query 参数兼容，header 见 pythonClient.WithTenant）
			proxiedPath := buildPath(r) + "?user_id=" + claims.UserID + "&tenant_id=" + claims.TenantID
				var resp interface{}
				var err error
				switch r.Method {
				case "GET":
					err = pythonClient.GetJSON(r.Context(), proxiedPath, &resp)
				case "POST":
					var body map[string]interface{}
					if err2 := DecodeJSON(w, r, &body); err2 != nil {
						return
					}
					err = pythonClient.PostJSON(r.Context(), proxiedPath, body, &resp)
				case "PUT":
					var body map[string]interface{}
					if err2 := DecodeJSON(w, r, &body); err2 != nil {
						return
					}
					err = pythonClient.PutJSON(r.Context(), proxiedPath, body, &resp)
				case "DELETE":
					err = pythonClient.DeleteJSON(r.Context(), proxiedPath, &resp)
				}
				if err != nil {
					slog.Error(opt.logTag+" proxy error", "path", proxiedPath, "error", err)
					logAndRespond(w, err, http.StatusInternalServerError, "internal error")
					return
				}
				OK(w, resp)
			}
		}
	}

	// Helper: build path functions for parameterised routes
	pathFn := func(static string) func(*http.Request) string {
		return func(*http.Request) string { return static }
	}
	pathParam := func(prefix string) func(*http.Request) string {
		return func(r *http.Request) string { return prefix + "/" + r.PathValue("id") }
	}
	pathParamSuffix := func(prefix, suffix string) func(*http.Request) string {
		return func(r *http.Request) string {
			return prefix + "/" + r.PathValue("id") + suffix
		}
	}

	// Graphs (auth + rate limited, proxies to Python)
	graphP := newProxy("", proxyOpt{logTag: "graph"})
	mux.Handle("GET /v1/graphs", authMW(rlMW(graphP(pathFn("/v1/graphs")))))
	mux.Handle("POST /v1/graphs", authMW(rlMW(graphP(pathFn("/v1/graphs")))))
	mux.Handle("GET /v1/graphs/{id}", authMW(rlMW(graphP(pathParam("/v1/graphs")))))
	mux.Handle("DELETE /v1/graphs/{id}", authMW(rlMW(graphP(pathParam("/v1/graphs")))))
	mux.Handle("POST /v1/graphs/{id}/execute", authMW(rlMW(graphP(pathParamSuffix("/v1/graphs", "/execute")))))

	// Workflow 执行状态与历史（代理 Python；status 支持内存实例 + DB 回退）
	mux.Handle("GET /v1/workflows/instances", authMW(rlMW(graphP(pathFn("/v1/workflows/instances")))))
	mux.Handle("GET /v1/workflows/{id}/status", authMW(rlMW(graphP(pathParamSuffix("/v1/workflows", "/status")))))

	// Knowledge Base (auth + rate limited + tenant QPS limit, proxies to Python)
	kbP := newProxy("/v1/kb", proxyOpt{logTag: "kb"})
	mux.Handle("GET /v1/kb", authMW(kbRateMW(kbP(pathFn("/v1/kb")))))
	mux.Handle("POST /v1/kb", authMW(kbRateMW(kbP(pathFn("/v1/kb")))))
	mux.Handle("GET /v1/kb/{id}", authMW(kbRateMW(kbP(pathParam("/v1/kb")))))
	mux.Handle("PUT /v1/kb/{id}", authMW(kbRateMW(kbP(pathParam("/v1/kb")))))
	mux.Handle("DELETE /v1/kb/{id}", authMW(kbRateMW(kbP(pathParam("/v1/kb")))))
	mux.Handle("POST /v1/kb/{id}/documents", authMW(kbRateMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			InternalError(w, "python engine not available")
			return
		}
		claims := auth.GetClaims(r.Context())
		pythonClient.ForwardRequest(w, r, "/v1/kb/"+r.PathValue("id")+"/documents?user_id="+claims.UserID)
	}))))
	mux.Handle("GET /v1/kb/{id}/documents", authMW(kbRateMW(kbP(pathParamSuffix("/v1/kb", "/documents")))))
	mux.Handle("POST /v1/kb/{id}/build", authMW(kbRateMW(kbP(pathParamSuffix("/v1/kb", "/build")))))
	mux.Handle("POST /v1/kb/{id}/query", authMW(kbRateMW(kbP(pathParamSuffix("/v1/kb", "/query")))))

	// Unified chat / quick-execute (六大工作台统一入口, proxies to Python TaskRouter)
	chatP := newProxy("", proxyOpt{logTag: "chat"})
	mux.Handle("POST /v1/chat/submit", authMW(rlMW(chatP(pathFn("/v1/chat/submit")))))
	mux.Handle("GET /v1/chat/sessions/{id}/messages", authMW(rlMW(chatP(pathParamSuffix("/v1/chat/sessions", "/messages")))))
	// quick-execute 为 chat/submit 的语义别名（前端快捷执行入口）
	mux.Handle("POST /v1/quick-execute", authMW(rlMW(chatP(pathFn("/v1/chat/submit")))))

	// Capabilities discovery (能力注册中心: 六大工作台能力发现)
	capP := newProxy("", proxyOpt{logTag: "capabilities"})
	mux.Handle("GET /v1/capabilities", authMW(rlMW(capP(pathFn("/v1/capabilities")))))
	mux.Handle("POST /v1/capabilities/search", authMW(rlMW(capP(pathFn("/v1/capabilities/search")))))

	// Memory (用户长期记忆 L2 档案卡: 列表/CRUD/语义检索/智能整理, 代理 Python)
	memP := newProxy("", proxyOpt{logTag: "memory"})
	mux.Handle("GET /v1/memory/profile", authMW(rlMW(memP(pathFn("/v1/memory/profile")))))
	mux.Handle("POST /v1/memory/profile", authMW(rlMW(memP(pathFn("/v1/memory/profile")))))
	mux.Handle("PUT /v1/memory/profile", authMW(rlMW(memP(pathFn("/v1/memory/profile")))))
	mux.Handle("DELETE /v1/memory/profile/{id}", authMW(rlMW(memP(pathParam("/v1/memory/profile")))))
	mux.Handle("POST /v1/memory/profile/clear", authMW(rlMW(memP(pathFn("/v1/memory/profile/clear")))))
	mux.Handle("POST /v1/memory/search", authMW(rlMW(memP(pathFn("/v1/memory/search")))))
	mux.Handle("POST /v1/memory/organize", authMW(rlMW(memP(pathFn("/v1/memory/organize")))))
	mux.Handle("GET /v1/memory/organize/status", authMW(rlMW(memP(pathFn("/v1/memory/organize/status")))))
	mux.Handle("GET /v1/memory/summaries", authMW(rlMW(memP(pathFn("/v1/memory/summaries")))))
	mux.Handle("GET /v1/memory/conflicts", authMW(rlMW(memP(pathFn("/v1/memory/conflicts")))))
	mux.Handle("POST /v1/memory/conflicts/{conflict_id}/resolve", authMW(rlMW(memP(pathFn("/v1/memory/conflicts")))))
}

// ── Admin ──

func registerAdminRoutes(
	mux *http.ServeMux,
	authMW routeMiddleware,
	adminHandler *AdminHandler,
	pythonClient *engine.PythonClient,
) {
	// Admin routes (auth + admin permission)
	// 读操作用 PermAdminRead，写操作（PUT/DELETE/POST）必须 PermAdminWrite，避免读权限执行写
	adminReadMW := RequirePermission(auth.PermAdminRead)
	adminWriteMW := RequirePermission(auth.PermAdminWrite)
	adminMux := http.NewServeMux()
	adminHandler.RegisterRoutes(adminMux)
	// Strip the /v1/admin prefix so adminMux patterns (e.g. "GET /metrics") match correctly
	adminStrip := http.StripPrefix("/v1/admin", adminMux)
	mux.Handle("GET /v1/admin/metrics", authMW(adminReadMW(adminStrip)))
	mux.Handle("GET /v1/admin/users", authMW(adminReadMW(adminStrip)))
	mux.Handle("GET /v1/admin/users/{id}", authMW(adminReadMW(adminStrip)))
	mux.Handle("PUT /v1/admin/users/{id}", authMW(adminWriteMW(adminStrip)))
	mux.Handle("DELETE /v1/admin/users/{id}", authMW(adminWriteMW(adminStrip)))
	mux.Handle("GET /v1/admin/system", authMW(adminReadMW(adminStrip)))
	mux.Handle("POST /v1/admin/maintenance", authMW(adminWriteMW(adminStrip)))
	mux.Handle("POST /v1/admin/backup", authMW(adminWriteMW(adminStrip)))
	mux.Handle("POST /v1/admin/restore", authMW(adminWriteMW(adminStrip)))
	mux.Handle("GET /v1/admin/kb", authMW(adminReadMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			InternalError(w, "python engine not available")
			return
		}
		claims := auth.GetClaims(r.Context())
		r.Header.Set("X-User-Role", claims.Role)
		pythonClient.ForwardRequest(w, r, "/v1/admin/kb?user_id="+claims.UserID)
	}))))

	// Storage admin routes
	mux.Handle("GET /v1/admin/storage", authMW(adminReadMW(adminStrip)))
	mux.Handle("PUT /v1/admin/storage", authMW(adminWriteMW(adminStrip)))
	mux.Handle("POST /v1/admin/storage/test", authMW(adminWriteMW(adminStrip)))

	// Redis admin routes
	mux.Handle("GET /v1/admin/redis", authMW(adminReadMW(adminStrip)))
	mux.Handle("PUT /v1/admin/redis", authMW(adminWriteMW(adminStrip)))
	mux.Handle("POST /v1/admin/redis/test", authMW(adminWriteMW(adminStrip)))

	// Queue admin routes
	mux.Handle("GET /v1/admin/queue", authMW(adminReadMW(adminStrip)))
	mux.Handle("POST /v1/admin/queue/flush", authMW(adminWriteMW(adminStrip)))
	mux.Handle("POST /v1/admin/queue/pause", authMW(adminWriteMW(adminStrip)))

	// Cache admin routes
	mux.Handle("GET /v1/admin/cache/stats", authMW(adminReadMW(adminStrip)))

	// Performance admin routes
	mux.Handle("GET /v1/admin/performance", authMW(adminReadMW(adminStrip)))

	// API Key admin routes (direct handlers, avoid adminMux path mismatch)
	mux.Handle("GET /v1/admin/api-keys", authMW(adminReadMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			OK(w, map[string]interface{}{"keys": []interface{}{}, "stats": map[string]interface{}{"total": 0, "active": 0, "rate_limited": 0, "circuit_open": 0}})
			return
		}
		pythonClient.ForwardRequest(w, r, "/v1/admin/api-keys")
	}))))
	mux.Handle("POST /v1/admin/api-keys", authMW(adminWriteMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			BadRequest(w, "python engine not available")
			return
		}
		pythonClient.ForwardRequest(w, r, "/v1/admin/api-keys")
	}))))
	mux.Handle("PUT /v1/admin/api-keys/{id}", authMW(adminWriteMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			BadRequest(w, "python engine not available")
			return
		}
		pythonClient.ForwardRequest(w, r, "/v1/admin/api-keys/"+r.PathValue("id"))
	}))))
	mux.Handle("DELETE /v1/admin/api-keys/{id}", authMW(adminWriteMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			BadRequest(w, "python engine not available")
			return
		}
		pythonClient.ForwardRequest(w, r, "/v1/admin/api-keys/"+r.PathValue("id"))
	}))))

	// Settings admin routes
	mux.Handle("PUT /v1/admin/settings", authMW(adminWriteMW(adminStrip)))
}
