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

	// Search (no auth)
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
	kbRateLimiter := NewTenantRateLimiter(atomicRedis.LoadRaw(), 50, 100)
	kbRateMW := kbRateLimiter.Middleware
	
	kbProxy := func(method, path string, body any) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if pythonClient == nil || !pythonClient.IsConnected() {
				InternalError(w, "python engine not available")
				return
			}
			claims := getAuthClaims(r, authenticator)
			if claims == nil {
				Unauthorized(w, "authentication required")
				return
			}
			proxiedPath := path + "?user_id=" + claims.UserID
			var resp interface{}
			var err error
			switch method {
			case "GET":
				err = pythonClient.GetJSON(r.Context(), proxiedPath, &resp)
			case "POST":
				var reqBody map[string]interface{}
				if body != nil {
					reqBody = body.(map[string]interface{})
				} else {
					var b map[string]interface{}
					if err2 := DecodeJSON(w, r, &b); err2 != nil {
						return
					}
					reqBody = b
				}
				err = pythonClient.PostJSON(r.Context(), proxiedPath, reqBody, &resp)
			case "PUT":
				var putBody map[string]interface{}
				if body != nil {
					putBody = body.(map[string]interface{})
				} else {
					var b map[string]interface{}
					if err2 := DecodeJSON(w, r, &b); err2 != nil {
						return
					}
					putBody = b
				}
				err = pythonClient.PutJSON(r.Context(), proxiedPath, putBody, &resp)
			case "DELETE":
				err = pythonClient.DeleteJSON(r.Context(), proxiedPath, &resp)
			}
			if err != nil {
				slog.Error("kb proxy error", "path", proxiedPath, "error", err)
				InternalError(w, strings.TrimSpace(err.Error()))
				return
			}
			OK(w, resp)
		}
	}

	// Admin handler
	adminHandler := NewAdminHandler(authenticator, fileStore, atomicRedis, pythonClient)

	// ── Public endpoints ──

	mux.HandleFunc("GET /search", searchHandler.Search)

	// Public share view (no auth; revoked shares return 410 Gone)
	mux.Handle("GET /share/{id}", rlMW(publicMW(http.HandlerFunc(shareHandler.PublicGet))))

	mux.Handle("GET /health", rlMW(publicMW(http.HandlerFunc(handleHealth))))
	mux.Handle("GET /ready", rlMW(publicMW(http.HandlerFunc(handleReadiness))))

	mux.Handle("POST /v1/agent/approval", authMW(rlMW(http.HandlerFunc(submitHandler.SubmitApproval))))
	mux.Handle("POST /submit", publicMW(sanitizeMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		claims := getAuthClaims(r, authenticator)
		if claims == nil {
			Unauthorized(w, "authentication required")
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

		select {
		case agentSem <- struct{}{}:
		default:
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
			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()
			submitHandler.HandleSubmit(ctx, userID, body.SessionID, body.Content, body.LLMConfig)
		}()
	}))))

	mux.Handle("POST /cancel", rlMW(publicMW(http.HandlerFunc(handleCancel))))

	// SSE + WebSocket
	mux.Handle("GET /events", authMW(rlMW(SSEHandler(eventHub, sessionMgr))))
	mux.HandleFunc("GET /ws/{sessionId}", WebSocketHandler(NewWebSocketHub(), eventHub, authenticator, sessionMgr))
	mux.HandleFunc("GET /ws/rpa", RPAWebSocketHandler(rpaHub, authenticator))

	// ── /v1/* routes (rate limited) ──

	// Auth (public, rate limited)
	mux.Handle("POST /v1/auth/login", rlMW(http.HandlerFunc(authHandler.Login)))
	mux.Handle("POST /v1/auth/register", rlMW(http.HandlerFunc(authHandler.Register)))
	mux.Handle("POST /v1/auth/refresh", rlMW(http.HandlerFunc(authHandler.Refresh)))
	mux.Handle("POST /v1/auth/logout", rlMW(http.HandlerFunc(authHandler.Logout)))
	mux.Handle("GET /v1/auth/profile", authMW(rlMW(http.HandlerFunc(authHandler.Profile))))
	mux.Handle("PUT /v1/auth/profile", authMW(rlMW(http.HandlerFunc(authHandler.UpdateProfile))))

	// Install (public, rate limited)
	mux.Handle("GET /v1/install/status", rlMW(http.HandlerFunc(installHandler.Status)))
	mux.Handle("POST /v1/install/setup", rlMW(http.HandlerFunc(installHandler.Setup)))

	// Editor (auth + rate limited)
	mux.Handle("GET /api/editor/files", authMW(rlMW(http.HandlerFunc(editorHandler.ListFiles))))
	mux.Handle("GET /api/editor/read", authMW(rlMW(http.HandlerFunc(editorHandler.ReadFile))))
	mux.Handle("POST /api/editor/write", authMW(rlMW(http.HandlerFunc(editorHandler.WriteFile))))

	// Conversations (auth + rate limited)
	mux.Handle("GET /v1/conversations", authMW(rlMW(http.HandlerFunc(conversationHandler.List))))
	mux.Handle("POST /v1/conversations", authMW(rlMW(http.HandlerFunc(conversationHandler.Create))))
	mux.Handle("GET /v1/conversations/{id}", authMW(rlMW(http.HandlerFunc(conversationHandler.Get))))
	mux.Handle("PUT /v1/conversations/{id}", authMW(rlMW(http.HandlerFunc(conversationHandler.Update))))
	mux.Handle("DELETE /v1/conversations/{id}", authMW(rlMW(http.HandlerFunc(conversationHandler.Delete))))

	// Conversation shares (auth + rate limited; public GET below)
	mux.Handle("POST /v1/conversations/{id}/share", authMW(rlMW(http.HandlerFunc(shareHandler.Create))))
	mux.Handle("GET /v1/conversations/{id}/share", authMW(rlMW(http.HandlerFunc(shareHandler.GetActive))))
	mux.Handle("DELETE /v1/conversations/{id}/share", authMW(rlMW(http.HandlerFunc(shareHandler.Revoke))))

	// Tools (rate limited, proxies to Python)
	mux.Handle("GET /v1/tools", rlMW(http.HandlerFunc(toolHandler.ListTools)))
	mux.Handle("POST /v1/tools/execute", rlMW(sanitizeMW(http.HandlerFunc(toolHandler.ExecuteTool))))

	// System (rate limited; spans/traces 仅管理员可见，S 安全修复：原为公开信息泄露)
	mux.Handle("GET /v1/system/health", rlMW(http.HandlerFunc(systemHandler.HealthScores)))
	mux.Handle("GET /v1/system/spans", authMW(rlMW(RequirePermission(auth.PermAdminRead)(http.HandlerFunc(systemHandler.Spans)))))
	mux.Handle("GET /v1/system/traces", authMW(rlMW(RequirePermission(auth.PermAdminRead)(http.HandlerFunc(systemHandler.Traces)))))
	mux.Handle("GET /v1/metrics", rlMW(http.HandlerFunc(systemHandler.Metrics)))

	// Trace (user-level call chain tracing, tenant-isolated)
	if traceHandler != nil {
		mux.Handle("GET /v1/traces", authMW(rlMW(http.HandlerFunc(traceHandler.ListTraces))))
		mux.Handle("GET /v1/traces/{trace_id}", authMW(rlMW(http.HandlerFunc(traceHandler.GetTrace))))
	}

	// Media (rate limited)
	mux.Handle("GET /v1/media", rlMW(http.HandlerFunc(mediaHandler.List)))
	mux.Handle("POST /v1/media", rlMW(http.HandlerFunc(mediaHandler.Create)))
	mux.Handle("POST /v1/media/folders", rlMW(http.HandlerFunc(mediaHandler.CreateFolder)))
	mux.Handle("GET /v1/media/folders", rlMW(http.HandlerFunc(mediaHandler.ListFolders)))
	mux.Handle("POST /v1/media/upload", rlMW(http.HandlerFunc(mediaHandler.Upload)))
	mux.Handle("POST /v1/media/presign", rlMW(http.HandlerFunc(mediaHandler.PresignUpload)))
	mux.Handle("POST /v1/media/complete", rlMW(http.HandlerFunc(mediaHandler.CompleteUpload)))
	mux.Handle("POST /v1/media/batch-delete", rlMW(http.HandlerFunc(mediaHandler.BatchDelete)))
	mux.Handle("PUT /v1/media/{id}", rlMW(http.HandlerFunc(mediaHandler.Update)))
	mux.Handle("GET /v1/media/{id}/download", rlMW(http.HandlerFunc(mediaHandler.Download)))
	mux.Handle("POST /v1/media/{id}/share", rlMW(http.HandlerFunc(mediaHandler.Share)))
	mux.Handle("DELETE /v1/media/{id}", rlMW(http.HandlerFunc(mediaHandler.Delete)))

	// Plugins (auth + rate limited)
	mux.Handle("GET /v1/plugins", authMW(rlMW(http.HandlerFunc(pluginHandler.List))))
	mux.Handle("POST /v1/plugins/{name}/install", authMW(rlMW(http.HandlerFunc(pluginHandler.Install))))
	mux.Handle("PUT /v1/plugins/{name}", authMW(rlMW(http.HandlerFunc(pluginHandler.Update))))
	mux.Handle("POST /v1/plugins/{name}/test", authMW(rlMW(http.HandlerFunc(pluginHandler.Test))))
	mux.Handle("DELETE /v1/plugins/{name}", authMW(rlMW(http.HandlerFunc(pluginHandler.Uninstall))))

	// Billing (auth + rate limited)
	mux.Handle("GET /v1/billing/balance", authMW(rlMW(http.HandlerFunc(billingHandler.GetBalance))))
	mux.Handle("GET /v1/billing/history", authMW(rlMW(http.HandlerFunc(billingHandler.GetHistory))))
	mux.Handle("POST /v1/billing/recharge", authMW(rlMW(http.HandlerFunc(billingHandler.Recharge))))
	mux.Handle("POST /v1/billing/pay", authMW(rlMW(http.HandlerFunc(billingHandler.CreatePayment))))
	mux.Handle("GET /v1/billing/orders/{id}", authMW(rlMW(http.HandlerFunc(billingHandler.GetOrder))))
	// 支付渠道异步回调（无 auth：支付宝验签 / 微信平台证书验签 + AES-GCM 解密）
	mux.Handle("POST /v1/billing/callback/alipay", rlMW(http.HandlerFunc(billingHandler.AlipayCallback)))
	mux.Handle("POST /v1/billing/callback/wechat", rlMW(http.HandlerFunc(billingHandler.WechatCallback)))
	mux.Handle("POST /v1/billing/paypal-capture", rlMW(http.HandlerFunc(billingHandler.PayPalCapture)))
	mux.Handle("GET /v1/billing/usage", authMW(rlMW(http.HandlerFunc(billingHandler.GetUsage))))

	// Graphs (auth + rate limited, proxies to Python)
	graphProxy := func(method, path string, body any) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if pythonClient == nil || !pythonClient.IsConnected() {
				InternalError(w, "python engine not available")
				return
			}
			claims := getAuthClaims(r, authenticator)
			if claims == nil {
				Unauthorized(w, "authentication required")
				return
			}
			proxiedPath := path
			if strings.Contains(path, "?") {
				proxiedPath = path + "&user_id=" + claims.UserID
			} else {
				proxiedPath = path + "?user_id=" + claims.UserID
			}
			var resp interface{}
			var err error
			switch method {
			case "GET":
				err = pythonClient.GetJSON(r.Context(), proxiedPath, &resp)
			case "POST":
				var reqBody map[string]interface{}
				if body != nil {
					reqBody = body.(map[string]interface{})
				} else {
					if err2 := DecodeJSON(w, r, &reqBody); err2 != nil {
						return
					}
				}
				err = pythonClient.PostJSON(r.Context(), proxiedPath, reqBody, &resp)
			case "DELETE":
				err = pythonClient.DeleteJSON(r.Context(), proxiedPath, &resp)
			}
			if err != nil {
				slog.Error("graph proxy error", "path", proxiedPath, "error", err)
				InternalError(w, strings.TrimSpace(err.Error()))
				return
			}
			OK(w, resp)
		}
	}
	mux.Handle("GET /v1/graphs", authMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		graphProxy("GET", "/v1/graphs", nil)(w, r)
	}))))
	mux.Handle("POST /v1/graphs", authMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		graphProxy("POST", "/v1/graphs", nil)(w, r)
	}))))
	mux.Handle("GET /v1/graphs/{id}", authMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		graphProxy("GET", "/v1/graphs/"+r.PathValue("id"), nil)(w, r)
	}))))
	mux.Handle("DELETE /v1/graphs/{id}", authMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		graphProxy("DELETE", "/v1/graphs/"+r.PathValue("id"), nil)(w, r)
	}))))
	mux.Handle("POST /v1/graphs/{id}/execute", authMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		graphProxy("POST", "/v1/graphs/"+r.PathValue("id")+"/execute", nil)(w, r)
	}))))

	// Workflow 执行状态与历史（代理 Python；status 支持内存实例 + DB 回退）
	mux.Handle("GET /v1/workflows/instances", authMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		graphProxy("GET", "/v1/workflows/instances", nil)(w, r)
	}))))
	mux.Handle("GET /v1/workflows/{id}/status", authMW(rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		graphProxy("GET", "/v1/workflows/"+r.PathValue("id")+"/status", nil)(w, r)
	}))))

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
	mux.Handle("POST /v1/agents/dispatch", rlMW(sanitizeMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	}))))

	// Skills (rate limited, proxies to Python)
	skillHandler.RegisterRoutes(mux)

	// Mode (auth + rate limited)
	mux.Handle("GET /v1/mode", authMW(rlMW(http.HandlerFunc(modeHandler.GetMode))))
	mux.Handle("POST /v1/mode", authMW(rlMW(http.HandlerFunc(modeHandler.SetMode))))
	mux.Handle("POST /v1/permission/approve", authMW(rlMW(http.HandlerFunc(modeHandler.ApprovePermission))))
	mux.Handle("POST /v1/permission/reject", authMW(rlMW(http.HandlerFunc(modeHandler.RejectPermission))))

	// Knowledge Base (auth + rate limited + tenant QPS limit, proxies to Python)
	mux.Handle("GET /v1/kb", authMW(kbRateMW(http.HandlerFunc(kbProxy("GET", "/v1/kb", nil)))))
	mux.Handle("POST /v1/kb", authMW(kbRateMW(http.HandlerFunc(kbProxy("POST", "/v1/kb", nil)))))
	mux.Handle("GET /v1/kb/{id}", authMW(kbRateMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kbProxy("GET", "/v1/kb/"+r.PathValue("id"), nil)(w, r)
	}))))
	mux.Handle("PUT /v1/kb/{id}", authMW(kbRateMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kbProxy("PUT", "/v1/kb/"+r.PathValue("id"), nil)(w, r)
	}))))
	mux.Handle("DELETE /v1/kb/{id}", authMW(kbRateMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kbProxy("DELETE", "/v1/kb/"+r.PathValue("id"), nil)(w, r)
	}))))
	mux.Handle("POST /v1/kb/{id}/documents", authMW(kbRateMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			InternalError(w, "python engine not available")
			return
		}
		claims := getAuthClaims(r, authenticator)
		if claims == nil {
			Unauthorized(w, "authentication required")
			return
		}
		pythonClient.ForwardRequest(w, r, "/v1/kb/"+r.PathValue("id")+"/documents?user_id="+claims.UserID)
	}))))
	mux.Handle("GET /v1/kb/{id}/documents", authMW(kbRateMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kbProxy("GET", "/v1/kb/"+r.PathValue("id")+"/documents", nil)(w, r)
	}))))
	mux.Handle("POST /v1/kb/{id}/build", authMW(kbRateMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kbProxy("POST", "/v1/kb/"+r.PathValue("id")+"/build", nil)(w, r)
	}))))
	mux.Handle("POST /v1/kb/{id}/query", authMW(kbRateMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kbProxy("POST", "/v1/kb/"+r.PathValue("id")+"/query", nil)(w, r)
	}))))

	// Admin routes (auth + admin permission)
	adminPermMW := RequirePermission(auth.PermAdminRead)
	adminMux := http.NewServeMux()
	adminHandler.RegisterRoutes(adminMux)
	// Strip the /v1/admin prefix so adminMux patterns (e.g. "GET /metrics") match correctly
	adminStrip := http.StripPrefix("/v1/admin", adminMux)
	mux.Handle("GET /v1/admin/metrics", authMW(adminPermMW(adminStrip)))
	mux.Handle("GET /v1/admin/users", authMW(adminPermMW(adminStrip)))
	mux.Handle("GET /v1/admin/users/{id}", authMW(adminPermMW(adminStrip)))
	mux.Handle("PUT /v1/admin/users/{id}", authMW(adminPermMW(adminStrip)))
	mux.Handle("DELETE /v1/admin/users/{id}", authMW(adminPermMW(adminStrip)))
	mux.Handle("GET /v1/admin/system", authMW(adminPermMW(adminStrip)))
	mux.Handle("POST /v1/admin/maintenance", authMW(adminPermMW(adminStrip)))
	mux.Handle("POST /v1/admin/backup", authMW(adminPermMW(adminStrip)))
	mux.Handle("POST /v1/admin/restore", authMW(adminPermMW(adminStrip)))
	mux.Handle("GET /v1/admin/kb", authMW(adminPermMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			InternalError(w, "python engine not available")
			return
		}
		claims := getAuthClaims(r, authenticator)
		if claims == nil {
			Unauthorized(w, "authentication required")
			return
		}
		r.Header.Set("X-User-Role", claims.Role)
		pythonClient.ForwardRequest(w, r, "/v1/admin/kb?user_id="+claims.UserID)
	}))))

	// Storage admin routes
	mux.Handle("GET /v1/admin/storage", authMW(adminPermMW(adminStrip)))
	mux.Handle("PUT /v1/admin/storage", authMW(adminPermMW(adminStrip)))
	mux.Handle("POST /v1/admin/storage/test", authMW(adminPermMW(adminStrip)))

	// Redis admin routes
	mux.Handle("GET /v1/admin/redis", authMW(adminPermMW(adminStrip)))
	mux.Handle("PUT /v1/admin/redis", authMW(adminPermMW(adminStrip)))
	mux.Handle("POST /v1/admin/redis/test", authMW(adminPermMW(adminStrip)))

	// Queue admin routes
	mux.Handle("GET /v1/admin/queue", authMW(adminPermMW(adminStrip)))
	mux.Handle("POST /v1/admin/queue/flush", authMW(adminPermMW(adminStrip)))
	mux.Handle("POST /v1/admin/queue/pause", authMW(adminPermMW(adminStrip)))

	// Cache admin routes
	mux.Handle("GET /v1/admin/cache/stats", authMW(adminPermMW(adminStrip)))

	// Performance admin routes
	mux.Handle("GET /v1/admin/performance", authMW(adminPermMW(adminStrip)))

	// API Key admin routes (direct handlers, avoid adminMux path mismatch)
	mux.Handle("GET /v1/admin/api-keys", authMW(adminPermMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			OK(w, map[string]interface{}{"keys": []interface{}{}, "stats": map[string]interface{}{"total": 0, "active": 0, "rate_limited": 0, "circuit_open": 0}})
			return
		}
		pythonClient.ForwardRequest(w, r, "/v1/admin/api-keys")
	}))))
	mux.Handle("POST /v1/admin/api-keys", authMW(adminPermMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			BadRequest(w, "python engine not available")
			return
		}
		pythonClient.ForwardRequest(w, r, "/v1/admin/api-keys")
	}))))
	mux.Handle("PUT /v1/admin/api-keys/{id}", authMW(adminPermMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			BadRequest(w, "python engine not available")
			return
		}
		pythonClient.ForwardRequest(w, r, "/v1/admin/api-keys/"+r.PathValue("id"))
	}))))
	mux.Handle("DELETE /v1/admin/api-keys/{id}", authMW(adminPermMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			BadRequest(w, "python engine not available")
			return
		}
		pythonClient.ForwardRequest(w, r, "/v1/admin/api-keys/"+r.PathValue("id"))
}))))

	// Settings admin routes
	mux.Handle("PUT /v1/admin/settings", authMW(adminPermMW(adminStrip)))

	// Media file serving
	mux.Handle("GET /media/", http.StripPrefix("/media/", http.FileServer(http.Dir(cfg.StorageRoot+"/media"))))

	// Wrap main mux with public middleware
	return publicMW(mux)
}
