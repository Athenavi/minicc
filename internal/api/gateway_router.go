package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/athenavi/chiron/config"
	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/billing"
	"github.com/athenavi/chiron/internal/broadcast"
	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/engine"
	"github.com/athenavi/chiron/internal/session"
	"github.com/athenavi/chiron/internal/storage"
)

// metricsAuthMW 鍏佽涓ょ閴存潈鏂瑰紡鎶撳彇 /metrics锛?
// 1. METRICS_TOKEN 閰嶇疆鐨?Bearer token锛圥rometheus 鎶撳彇锛屽父閲忔椂闂存瘮杈冿級锛?
// 2. JWT admin 鏉冮檺锛圥ermAdminRead锛夈€?
func metricsAuthMW(cfg *config.Config, authMW routeMiddleware, h http.HandlerFunc) http.Handler {
	if cfg == nil || cfg.MetricsToken == "" {
		return authMW(RequirePermission(auth.PermAdminRead)(h))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+cfg.MetricsToken)) == 1 {
			h(w, r)
			return
		}
		authMW(RequirePermission(auth.PermAdminRead)(h)).ServeHTTP(w, r)
	})
}

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
		if _, err := rand.Read(buf[:]); err != nil {
			// 鏋佺鍥為€€锛歳and 澶辫触鐢ㄦ椂闂寸撼绉掑～鍏?
			nano := time.Now().UnixNano()
			for i := range buf {
				buf[i] = byte(nano >> (i * 8))
			}
		}
		id := hex.EncodeToString(buf[:])
		r.Header.Set("X-Request-ID", id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

// realIPHeader extracts the real IP from X-Forwarded-For / X-Real-IP,
// but ONLY when the direct peer is a trusted reverse proxy (P1 淇)锛?
// 鏃犳潯浠朵俊浠诲鎴风鍙吉閫犵殑 XFF 浼氱粫杩囨寜 IP 鐨勯檺娴佷笌楠岃瘉鐮佸け璐ュ崌绾с€?
func realIPHeader(trustedCIDRs []string) func(http.Handler) http.Handler {
	var trusted []*net.IPNet
	for _, c := range trustedCIDRs {
		if _, n, err := net.ParseCIDR(strings.TrimSpace(c)); err == nil {
			trusted = append(trusted, n)
		}
	}
	peerTrusted := func(remoteAddr string) bool {
		if len(trusted) == 0 {
			return false
		}
		host, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			host = remoteAddr
		}
		ip := net.ParseIP(strings.TrimSpace(host))
		if ip == nil {
			return false
		}
		for _, n := range trusted {
			if n.Contains(ip) {
				return true
			}
		}
		return false
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if peerTrusted(r.RemoteAddr) {
				if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
					if idx := strings.Index(fwd, ","); idx >= 0 {
						r.RemoteAddr = strings.TrimSpace(fwd[:idx])
					} else {
						r.RemoteAddr = strings.TrimSpace(fwd)
					}
				} else if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
					r.RemoteAddr = realIP
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// NewSetupRouter 瀹夎妯″紡璺敱锛氱郴缁熷皻鏈厤缃?APP_SECRET / 鏁版嵁搴撴椂锛?
// 浠呮彁渚涘畨瑁呭悜瀵肩鐐癸紙/v1/install/*锛岄渶瀹夎浠ょ墝锛変笌鍋ュ悍妫€鏌ワ紱
// 鍏朵綑涓€鍒囦笟鍔¤矾鐢辫繑鍥?503锛堟湭瀹夎锛岃瀹屾垚瀹夎鍚庨噸鍚級銆?
// 鍓嶇闈欐€侀〉闈㈢敱 nginx 绛夊弽鍚戜唬鐞嗘彁渚涳紝Go 缃戝叧涓嶈礋璐ｆ墭绠°€?
func NewSetupRouter(cfg *config.Config) http.Handler {
	mux := http.NewServeMux()

	publicMW := func(next http.Handler) http.Handler {
		return middlewareChain(next,
			RecoverMiddleware,
			TracingMiddleware,
			LoggingMiddleware,
			SecurityHeadersMiddleware,
			CORSMiddleware(cfg.CORSOrigins),
			MonitoringMiddleware,
			requestIDHeader,
			realIPHeader(cfg.TrustedProxyCIDRs),
		)
	}

	installHandler := NewInstallHandler(cfg)

	// 瀹夎绔偣锛氬繀椤绘惡甯﹀畨瑁呬护鐗岋紙X-Install-Token header 鎴??token= 鏌ヨ鍙傛暟锛?
	mux.Handle("GET /v1/install/step1", publicMW(installMW(http.HandlerFunc(installHandler.Step1))))
	mux.Handle("POST /v1/install/step2", publicMW(installMW(http.HandlerFunc(installHandler.Step2))))
	mux.Handle("POST /v1/install/step3", publicMW(installMW(http.HandlerFunc(installHandler.Step3))))
	mux.Handle("POST /v1/install/setup", publicMW(installMW(http.HandlerFunc(installHandler.Setup))))
	mux.Handle("GET /v1/install/status", publicMW(http.HandlerFunc(installHandler.Status)))

	// 鍋ュ悍妫€鏌ワ紙缂栨帓鍣ㄦ帰娲伙紱灏辩华妫€鏌ュ瀹炲弽鏄犱緷璧栫姸鎬侊級
	mux.Handle("GET /health", publicMW(http.HandlerFunc(handleHealth)))
	mux.Handle("GET /ready", publicMW(http.HandlerFunc(handleReadiness)))

	// 鍏朵綑鎵€鏈夋湭娉ㄥ唽璺敱锛氫笟鍔′笉鍙敤锛堟湭瀹夎锛?
	mux.Handle("/", publicMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServiceUnavailable(w, "system not installed: configure database and create admin via /install")
	})))

	return mux
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

	// Rate limiter 鈥?褰?Redis 鍙敤鏃朵娇鐢ㄥ垎甯冨紡闄愭祦鍣ㄣ€?
	// P1-2: 鍗曞疄渚嬪唴瀛橀檺娴佸湪澶氬壇鏈儴缃蹭笅璁℃暟鐙珛锛岀瓑浜庨檺娴佸け鏁堛€?
	// 鐢熶骇绛栫暐锛?
	//   - Redis 鍙敤锛氱敤鍒嗗竷寮忛檺娴佸櫒锛堟帹鑽愶級
	//   - Redis 涓嶅彲鐢?+ 鐢熶骇鐜锛坈fg.RateLimitFailClose=true锛夛細鍐欐搷浣滄嫆缁?
	//   - Redis 涓嶅彲鐢?+ 寮€鍙?娴嬭瘯锛氶檷绾у唴瀛橀檺娴侊紙鍗曞疄渚嬫湁鏁堬紝澶氬壇鏈け鏁堬級
	var rlMW func(http.Handler) http.Handler
	var distLimiter *DistributedRateLimiter
	rateLimitRPM := cfg.RateLimitRPM
	if rateLimitRPM <= 0 {
		slog.Warn("RateLimitRPM is 0 or unset, using default 60 RPM")
		rateLimitRPM = 60
	}
	if atomicRedis != nil {
		distLimiter = NewDistributedRateLimiter(
			atomicRedis.LoadRaw(),
			rateLimitRPM*10, // 鍏ㄥ眬锛氬崟瀹炰緥闄愬埗 脳 10
			rateLimitRPM*5,  // 绉熸埛锛氬崟瀹炰緥闄愬埗 脳 5
			rateLimitRPM,    // 鐢ㄦ埛锛氬崟瀹炰緥闄愬埗
		)
		rlMW = DistributedRateLimitMiddleware(distLimiter)
		slog.Info("distributed rate limiter enabled", "global", rateLimitRPM*10)
	} else if cfg.RateLimitFailClose {
		// 鐢熶骇 fail-close锛氬彧璇绘斁琛岋紝鍐欐搷浣滄嫆缁?
		rlMW = func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet && r.Method != http.MethodHead {
					http.Error(w, "rate limiter unavailable (redis down)", http.StatusServiceUnavailable)
					return
				}
				next.ServeHTTP(w, r)
			})
		}
		slog.Warn("Redis unavailable + fail-close enabled: write operations rejected")
	} else {
		// 寮€鍙?娴嬭瘯闄嶇骇锛氬唴瀛橀檺娴侊紙鍗曞疄渚嬫湁鏁堬紝澶氬壇鏈儴缃蹭笅搴旀敼鐢?Redis锛?
		rateLimiter := NewRateLimiter(rateLimitRPM)
		rateLimiter.CleanupVisitors(5 * time.Minute)
		rlMW = rateLimiter.Middleware
		slog.Warn("local rate limiter enabled (no Redis); not safe for multi-replica production")
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
			realIPHeader(cfg.TrustedProxyCIDRs),
		)
	}

	// Auth
	authenticator := auth.NewAuthenticator(cfg.JWTSecret, cfg.JWTExpiration)
	authHandler := NewAuthHandler(cfg)
	authMW := AuthMiddleware(authenticator)

	// SSO 涓夋柟鐧诲綍 + 浜烘満楠岃瘉锛堥槻鎺ュ彛婊ョ敤锛? 鐭俊楠岃瘉鐮佺櫥褰?
	ssoHandler := NewSSOHandler(authenticator, cfg)
	captchaHandler := NewCaptchaHandler(cfg)
	authHandler.SetCaptchaHandler(captchaHandler)
	smsHandler := NewSmsHandler(authenticator, cfg, captchaHandler)

	// Billing
	billingStore := billing.NewPGStore()
	billingStore.EnsureTables(context.Background())
	billingMgr := billing.NewManager(billingStore)
	billingMgr.Subscribe(billing.NewTransactionRecorder(billingStore))
	// P0-P1 淇锛氫綑棰濆凡鐢?Deduct/AddCredits 鍚屾鍐欏簱锛圥G 鍘熷瓙 UPDATE锛夛紝
	// 绉婚櫎 BalanceSyncer 寮傛钀藉簱璁㈤槄锛岄伩鍏嶅鍓湰 split-brain 涓庨噸澶嶆墸璐广€?

	// Agent execution semaphore
	agentSem := make(chan struct{}, cfg.AgentMaxConcurrency)

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
	mediaHandler.SetMediaRoot(cfg.StorageRoot)

	// 閫氱敤鍒嗙墖涓婁紶锛堟柇鐐圭画浼狅級
	uploadHandler := NewUploadHandler(authenticator, cfg.StorageRoot)
	uploadHandler.RegisterRoutes(mux, authMW, rlMW)

	// 鐢ㄦ埛渚у競鍦猴紙鎶€鑳?Agent/MCP 娴忚涓庝竴閿畨瑁咃級
	userMarketHandler := NewUserMarketHandler(cfg, pythonClient)
	registerUserMarketRoutes(mux, userMarketHandler, authMW, rlMW)

	// 妯″瀷璺敱锛氬璇濆彲鐢ㄦā鍨嬪垪琛?
	mux.Handle("GET /v1/models", authMW(rlMW(http.HandlerFunc(ListUserModels))))
	// 妯℃澘甯傚満锛氬伐浣滄祦/Agent/鎶€鑳?涓€閿娇鐢?
	templateHandler := NewTemplateHandler(pythonClient)
	templateHandler.RegisterRoutes(mux, authMW, rlMW)

	// 瀹氭椂鑷姩鍖栵細Webhook 瑙﹀彂锛坱oken 鍗抽壌鏉冿紝鍏紑浣嗛檺娴侊級
	mux.Handle("POST /v1/hooks/{jobID}", rlMW(http.HandlerFunc(HandleCronWebhook)))

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

	// Knowledge base 鈥?proxied to Python engine
	// SaaS 瀹夊叏: 鐭ヨ瘑搴撶嫭绔嬮檺娴?(姣忕鎴?QPS=50, Burst=100)
	var kbRateRedis db.RedisClient
	if atomicRedis != nil {
		kbRateRedis = atomicRedis.LoadRaw()
	}
	kbRateLimiter := NewTenantRateLimiter(kbRateRedis, 50, 100)
	kbRateMW := kbRateLimiter.Middleware

	// Admin handler
	adminHandler := NewAdminHandler(authenticator, fileStore, atomicRedis, pythonClient)
	adminHandler.rateLimiter = distLimiter
	adminHandler.appSecret = cfg.AppSecret

	// 鈹€鈹€ Route registration by functional domain 鈹€鈹€

	registerPublicEndpoints(mux, authMW, rlMW, publicMW, searchHandler, shareHandler, systemHandler, cfg)
	registerAgentRoutes(mux, authMW, rlMW, publicMW, sanitizeMW, submitHandler, billingMgr, agentSem, eventHub, sessionMgr, authenticator, rpaHub, cfg.InternalToken)
	registerAuthRoutes(mux, authHandler, authMW, rlMW)

	// 鈹€鈹€ SSO 涓夋柟鐧诲綍锛堝叕寮€娴佺▼ rlMW锛涚敤鎴疯嚜鍔?authMW锛涚鐞?authMW + sso:manage锛夆攢鈹€
	ssoHandler.RegisterPublicRoutes(mux, rlMW)
	ssoHandler.RegisterUserRoutes(mux, authMW)
	ssoHandler.RegisterAdminRoutes(mux, authMW)

	// 鈹€鈹€ 浜烘満楠岃瘉锛氬叕寮€閰嶇疆涓嬪彂锛堢櫥褰曢〉鎷夊彇锛? 绠＄悊閰嶇疆 鈹€鈹€
	captchaHandler.RegisterPublicRoutes(mux, rlMW)
	captchaHandler.RegisterAdminRoutes(mux, authMW)

	// 鈹€鈹€ 鐭俊楠岃瘉鐮佺櫥褰曪紙鍏紑娴佺▼ rlMW锛涚敤鎴疯嚜鍔?authMW锛涚鐞?authMW + sso:manage锛夆攢鈹€
	smsHandler.RegisterPublicRoutes(mux, rlMW)
	smsHandler.RegisterUserRoutes(mux, authMW)
	smsHandler.RegisterAdminRoutes(mux, authMW)
	registerSystemRoutes(mux, authMW, rlMW, sanitizeMW, installHandler, editorHandler, toolHandler, systemHandler, traceHandler)
	// 鍏ぇ宸ヤ綔鍙颁簰鑱旓細璺ㄥ彴鏈€杩戞椿鍔ㄨ仛鍚堬紙绉熸埛+鐢ㄦ埛闅旂锛?
	mux.Handle("GET /v1/activities", authMW(rlMW(http.HandlerFunc(handleActivities))))
	registerConversationRoutes(mux, conversationHandler, shareHandler, authMW, rlMW)
	registerMediaRoutes(mux, mediaHandler, authMW, rlMW, cfg.StorageRoot)
	registerPluginRoutes(mux, pluginHandler, authMW, rlMW)
	registerBillingRoutes(mux, billingHandler, authMW, rlMW)
	registerProxyRoutes(mux, authMW, rlMW, kbRateMW, pythonClient)

	// Agents (auth + rate limited; DB 椹卞姩 CRUD + 杩愯浼氳瘽)
	agentHandler := NewAgentHandler(authenticator, pythonClient, agentSem)
	mux.Handle("GET /v1/agents", authMW(rlMW(http.HandlerFunc(agentHandler.List))))
	mux.Handle("POST /v1/agents", authMW(rlMW(http.HandlerFunc(agentHandler.Create))))
	mux.Handle("GET /v1/agents/{id}", authMW(rlMW(http.HandlerFunc(agentHandler.Get))))
	mux.Handle("PUT /v1/agents/{id}", authMW(rlMW(http.HandlerFunc(agentHandler.Update))))
	mux.Handle("DELETE /v1/agents/{id}", authMW(rlMW(http.HandlerFunc(agentHandler.Delete))))
	mux.Handle("PUT /v1/agents/{id}/visibility", authMW(rlMW(http.HandlerFunc(agentHandler.SetVisibility))))
	mux.Handle("POST /v1/agents/{id}/run", authMW(rlMW(http.HandlerFunc(agentHandler.Run))))
	mux.Handle("GET /v1/agents/sessions", authMW(rlMW(http.HandlerFunc(agentHandler.ListSessions))))
	mux.Handle("GET /v1/agents/sessions/{id}", authMW(rlMW(http.HandlerFunc(agentHandler.GetSession))))
	// dispatch 淇濈暀 Python 浠ｇ悊锛坅gent 宸ュ叿閾惧唴閮ㄨ皟鐢紝闈為〉闈富閾捐矾锛?
	// 瀹夊叏锛氬繀椤荤粡杩?authMW锛屽惁鍒欐湭璁よ瘉鍙Е鍙戝伐鍏锋墽琛?
	mux.Handle("POST /v1/agents/dispatch", authMW(rlMW(sanitizeMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil {
			InternalError(w, "python engine not available")
			return
		}
		var body map[string]interface{}
		if err := DecodeJSON(w, r, &body); err != nil {
			BadRequest(w, "invalid request")
			return
		}
		// 韬唤娉ㄥ叆锛圥1锛夛細寮曟搸鏃犻壌鏉冿紝蹇呴』鐢?JWT claims 瑕嗙洊閫忎紶鐨勭鎴?鐢ㄦ埛韬唤锛?
		// 闃叉瀹㈡埛绔吉閫?tenant_id/user_id 鍐掑厖浠栦汉銆?
		if claims := auth.GetClaims(r.Context()); claims != nil {
			body["tenant_id"] = claims.TenantID
			body["user_id"] = claims.UserID
		}
		var resp map[string]interface{}
		if err := pythonClient.PostJSON(r.Context(), "/v1/agents/dispatch", body, &resp); err != nil {
			InternalError(w, "python agent dispatch failed")
			return
		}
		OK(w, resp)
	})))))

	// Skills (auth + rate limited + prompt sanitized, proxies to Python)
	skillHandler.RegisterRoutes(mux, authMW, rlMW, sanitizeMW)

	// Enterprise audit (auth + RequireEntPerm("audit:read"))
	NewEntAuditHandler().RegisterRoutes(mux, authMW)

	// Enterprise identity (auth + RequireEntPerm("ent:manage"))锛氱敤鎴?瑙掕壊/缇ょ粍/绉熸埛
	NewEntIdentityHandler().RegisterRoutes(mux, authMW)

	// Enterprise cost center锛坅uthMW锛屽唴閮ㄦ寜 PermAdminRead/Write 鍒嗙骇锛?
	NewEntCostCenterHandler(nil, nil).RegisterRoutes(mux, authMW)

	// Enterprise policy锛坅uthMW + RequireEntPerm("policy:manage")锛夛細闅愮妯″紡 + 妯″瀷绛栫暐
	NewEntPolicyHandler().RegisterRoutes(mux, authMW)

	// Enterprise market锛坅uthMW + RequireEntPerm("market:manage")锛夛細鑳藉姏甯傚満 + 绉熸埛鎺堟潈
	NewMarketHandler().RegisterRoutes(mux, authMW)

	// Mode (auth + rate limited)
	mux.Handle("GET /v1/mode", authMW(rlMW(http.HandlerFunc(modeHandler.GetMode))))
	mux.Handle("POST /v1/mode", authMW(rlMW(http.HandlerFunc(modeHandler.SetMode))))
	mux.Handle("POST /v1/permission/approve", authMW(rlMW(http.HandlerFunc(modeHandler.ApprovePermission))))
	mux.Handle("POST /v1/permission/reject", authMW(rlMW(http.HandlerFunc(modeHandler.RejectPermission))))

	registerAdminRoutes(mux, authMW, rlMW, adminHandler, pythonClient)

	// Wrap main mux with public middleware
	return publicMW(mux)
}

// 鈹€鈹€ Public endpoints 鈹€鈹€

func registerPublicEndpoints(
	mux *http.ServeMux,
	authMW, rlMW, publicMW routeMiddleware,
	searchHandler *SearchHandler,
	shareHandler *ShareHandler,
	systemHandler *SystemHandler,
	cfg *config.Config,
) {
	mux.Handle("GET /search", authMW(rlMW(http.HandlerFunc(searchHandler.Search))))

	// Public share view (no auth; revoked shares return 410 Gone)
	// 淇 P1锛氱Щ闄ゅ唴灞?publicMW 閲嶅鍖呰９锛堝灞?publicMW(mux) 宸插寘鍚棩蹇?瀹¤/杩借釜锛夛紝
	// 閬垮厤瀹¤ XAdd 鍙屽啓銆佽姹?ID 琚唴灞傞噸鏂扮敓鎴愩€?
	mux.Handle("GET /v1/share/{id}", rlMW(http.HandlerFunc(shareHandler.PublicGet)))

	mux.Handle("GET /health", rlMW(http.HandlerFunc(handleHealth)))
	// Prometheus 鎸囨爣绔偣锛氱敓浜ф敹鏁涗负闇€瑕?PermAdminRead 鏉冮檺锛岄伩鍏嶆硠婕忎笟鍔℃寚鏍?
	mux.Handle("GET /metrics", rlMW(metricsAuthMW(cfg, authMW, systemHandler.PrometheusMetrics)))
	// API 鏂囨。锛圤penAPI spec锛屽叕寮€锛屼緵 Swagger/Redoc 灞曠ず锛?
	mux.Handle("GET /docs/", http.StripPrefix("/docs/", http.FileServer(http.Dir("docs"))))
	mux.Handle("GET /ready", rlMW(http.HandlerFunc(handleReadiness)))
	// 寮曟搸閰嶇疆涓嬪彂锛圶-Internal-Token 淇濇姢锛孭ython 寮曟搸鍚姩鎷夊彇锛?
	mux.Handle("GET /v1/internal/engine-config", rlMW(internalTokenMW(cfg, EngineConfig(cfg))))
}

// 鈹€鈹€ Agent submit/cancel/events 鈹€鈹€

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
	internalToken string,
) {
	mux.Handle("POST /v1/agent/approval", authMW(rlMW(http.HandlerFunc(submitHandler.SubmitApproval))))

	mux.Handle("POST /submit", authMW(sanitizeMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
						Error:   "insufficient credits 鈥?please recharge in Billing",
					})
					return
				}
			}
		}

		// Reject concurrent submits within the same session
		ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
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
	}))))

	mux.Handle("POST /cancel", authMW(rlMW(http.HandlerFunc(handleCancel))))

	// SSE + WebSocket
	mux.Handle("GET /events", authMW(rlMW(SSEHandler(eventHub, sessionMgr))))
	mux.HandleFunc("GET /ws/{sessionId}", WebSocketHandler(NewWebSocketHub(), eventHub, authenticator, sessionMgr))
	mux.HandleFunc("GET /ws/rpa", RPAWebSocketHandler(rpaHub, authenticator))
	// 娴忚鍣?RPA 妗ワ紙Python engine 鈫?缃戝叧 鈫?鎻掍欢锛涗粎鍏变韩 internal token 鍙皟锛?
	mux.Handle("POST /v1/rpa/exec", rlMW(http.HandlerFunc(RPAExecHandler(rpaHub, internalToken))))
	mux.Handle("GET /v1/rpa/clients", rlMW(http.HandlerFunc(RPAClientsHandler(rpaHub, internalToken))))
}

// 鈹€鈹€ Auth (login / register / logout) 鈹€鈹€

func registerAuthRoutes(mux *http.ServeMux, authHandler *AuthHandler, authMW, rlMW routeMiddleware) {
	// Auth (public, rate limited)
	mux.Handle("POST /v1/auth/login", rlMW(http.HandlerFunc(authHandler.Login)))
	mux.Handle("POST /v1/auth/register", rlMW(http.HandlerFunc(authHandler.Register)))
	mux.Handle("POST /v1/auth/refresh", rlMW(http.HandlerFunc(authHandler.Refresh)))
	mux.Handle("POST /v1/auth/logout", rlMW(http.HandlerFunc(authHandler.Logout)))
	// SSO cookie 鈫?Bearer token 浼氳瘽寮曞锛堝叕寮€锛歨ttpOnly cookie 鑷甫鍑嵁锛?
	mux.Handle("GET /v1/auth/session", rlMW(http.HandlerFunc(authHandler.Session)))
	mux.Handle("GET /v1/auth/profile", authMW(rlMW(http.HandlerFunc(authHandler.Profile))))
	mux.Handle("PUT /v1/auth/profile", authMW(rlMW(http.HandlerFunc(authHandler.UpdateProfile))))
}

// 鈹€鈹€ System (install / editor / tools / health / trace) 鈹€鈹€

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
	// Install wizard steps (setup mode: database/master key not configured)
	mux.Handle("GET /v1/install/step1", rlMW(http.HandlerFunc(installHandler.Step1)))
	mux.Handle("POST /v1/install/step2", rlMW(http.HandlerFunc(installHandler.Step2)))
	mux.Handle("POST /v1/install/step3", rlMW(http.HandlerFunc(installHandler.Step3)))

	// Editor (admin 鏉冮檺 + rate limited)
	// S 瀹夊叏淇锛氱紪杈戝櫒鐩存帴璇诲啓鍏变韩鏈嶅姟鍣ㄥ伐浣滃尯锛堝惈娌欑/鍒嗙墖/鎻掍欢鏁版嵁锛夛紝
	// 浠呴檺绠＄悊鍛橈紙鍒楄〃/璇?PermAdminRead锛屽啓=PermAdminWrite锛夛紝鏅€?user 鏃犳潈璁块棶銆?
	mux.Handle("GET /api/editor/files", authMW(rlMW(RequirePermission(auth.PermAdminRead)(http.HandlerFunc(editorHandler.ListFiles)))))
	mux.Handle("GET /api/editor/read", authMW(rlMW(RequirePermission(auth.PermAdminRead)(http.HandlerFunc(editorHandler.ReadFile)))))
	mux.Handle("POST /api/editor/write", authMW(rlMW(RequirePermission(auth.PermAdminWrite)(http.HandlerFunc(editorHandler.WriteFile)))))

	// Tools (rate limited, proxies to Python)
	mux.Handle("GET /v1/tools", rlMW(http.HandlerFunc(toolHandler.ListTools)))
	mux.Handle("POST /v1/tools/execute", authMW(rlMW(sanitizeMW(http.HandlerFunc(toolHandler.ExecuteTool)))))

	// System (rate limited; spans/traces 浠呯鐞嗗憳鍙锛孲 瀹夊叏淇锛氬師涓哄叕寮€淇℃伅娉勯湶)
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

// 鈹€鈹€ Conversations 鈹€鈹€

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

// 鈹€鈹€ Media 鈹€鈹€

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

	// Media file serving锛圥0 瀛樺偍鍨?XSS 闃叉姢锛歨tml/xml 鐩存帴鎷掓湇鍔★紱svg 浠?CSP sandbox 杈撳嚭锛涘叏閲?nosniff锛?
	mediaFileServer := http.StripPrefix("/media/", http.FileServer(http.Dir(storageRoot+"/media")))
	mux.Handle("GET /media/", rlMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.ToLower(filepath.Ext(r.URL.Path)) {
		case ".html", ".htm", ".xml", ".xhtml", ".swf":
			http.NotFound(w, r)
			return
		case ".svg":
			w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mediaFileServer.ServeHTTP(w, r)
	})))
	// 绛惧悕 URL锛圥0 淇锛夛細绛惧彂 + 鏍￠獙鍚庢湇鍔?
	mux.Handle("POST /v1/media/{id}/sign", authMW(rlMW(http.HandlerFunc(mediaHandler.SignMedia))))
	mux.Handle("GET /media/s/{assetID}", rlMW(http.HandlerFunc(mediaHandler.ServeSignedMedia)))
}

// registerUserMarketRoutes 鐢ㄦ埛渚у競鍦鸿矾鐢憋紙鎶€鑳?Agent/MCP 娴忚涓庝竴閿畨瑁咃級銆?
func registerUserMarketRoutes(mux *http.ServeMux, h *UserMarketHandler, authMW, rlMW routeMiddleware) {
	mux.Handle("GET /v1/market", authMW(rlMW(http.HandlerFunc(h.List))))
	mux.Handle("POST /v1/market/{type}/{itemID}/install", authMW(rlMW(http.HandlerFunc(h.Install))))
}

// 鈹€鈹€ Plugins 鈹€鈹€

func registerPluginRoutes(mux *http.ServeMux, pluginHandler *PluginHandler, authMW, rlMW routeMiddleware) {
	// Plugins (auth + rate limited)
	mux.Handle("GET /v1/plugins", authMW(rlMW(http.HandlerFunc(pluginHandler.List))))
	mux.Handle("POST /v1/plugins/{name}/install", authMW(rlMW(http.HandlerFunc(pluginHandler.Install))))
	mux.Handle("PUT /v1/plugins/{name}", authMW(rlMW(http.HandlerFunc(pluginHandler.Update))))
	mux.Handle("POST /v1/plugins/{name}/test", authMW(rlMW(http.HandlerFunc(pluginHandler.Test))))
	mux.Handle("DELETE /v1/plugins/{name}", authMW(rlMW(http.HandlerFunc(pluginHandler.Uninstall))))
}

// 鈹€鈹€ Billing 鈹€鈹€

func registerBillingRoutes(mux *http.ServeMux, billingHandler *BillingHandler, authMW, rlMW routeMiddleware) {
	// Billing (auth + rate limited)
	mux.Handle("GET /v1/billing/balance", authMW(rlMW(http.HandlerFunc(billingHandler.GetBalance))))
	mux.Handle("GET /v1/billing/history", authMW(rlMW(http.HandlerFunc(billingHandler.GetHistory))))
	mux.Handle("POST /v1/billing/recharge", authMW(rlMW(http.HandlerFunc(billingHandler.Recharge))))
	mux.Handle("POST /v1/billing/pay", authMW(rlMW(http.HandlerFunc(billingHandler.CreatePayment))))
	mux.Handle("GET /v1/billing/orders/{id}", authMW(rlMW(http.HandlerFunc(billingHandler.GetOrder))))
	// 鏀粯娓犻亾寮傛鍥炶皟锛堟棤 auth锛氭敮浠樺疂楠岀 / 寰俊骞冲彴璇佷功楠岀 + AES-GCM 瑙ｅ瘑锛?
	mux.Handle("POST /v1/billing/callback/alipay", rlMW(http.HandlerFunc(billingHandler.AlipayCallback)))
	mux.Handle("POST /v1/billing/callback/wechat", rlMW(http.HandlerFunc(billingHandler.WechatCallback)))
	mux.Handle("POST /v1/billing/paypal-capture", authMW(rlMW(http.HandlerFunc(billingHandler.PayPalCapture))))
	mux.Handle("GET /v1/billing/usage", authMW(rlMW(http.HandlerFunc(billingHandler.GetUsage))))
}

// 鈹€鈹€ Python engine proxy routes (graphs / workflows / knowledge base) 鈹€鈹€

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
				if pythonClient == nil {
					InternalError(w, "python engine not available")
					return
				}
				claims := auth.GetClaims(r.Context())
				if claims == nil || claims.TenantID == "" {
					Unauthorized(w, "missing tenant context")
					return
				}
				// 澶氱鎴烽殧绂伙細閫忎紶 tenant_id 缁?Python 寮曟搸锛坬uery 鍙傛暟鍏煎锛宧eader 瑙?pythonClient.WithTenant锛?
				proxiedPath := buildPath(r) + "?user_id=" + claims.UserID + "&tenant_id=" + claims.TenantID
				var resp interface{}
				var err error
				switch r.Method {
				case "GET":
					err = pythonClient.GetJSON(r.Context(), proxiedPath, &resp)
				case "POST":
					var body map[string]interface{}
					if err2 := DecodeJSON(w, r, &body); err2 != nil {
						BadRequest(w, ErrInvalidReq)
						return
					}
					err = pythonClient.PostJSON(r.Context(), proxiedPath, body, &resp)
				case "PUT":
					var body map[string]interface{}
					if err2 := DecodeJSON(w, r, &body); err2 != nil {
						BadRequest(w, ErrInvalidReq)
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
	// pathParamNamed 鏀寔鑷畾涔夎矾寰勫弬鏁板悕锛堝 {conflict_id}锛夛紝鐢ㄤ簬淇鍙傛暟涓㈠け鐨勪唬鐞嗚矾鐢便€?
	pathParamNamed := func(prefix, param, suffix string) func(*http.Request) string {
		return func(r *http.Request) string {
			return prefix + "/" + r.PathValue(param) + suffix
		}
	}

	// Graphs (auth + rate limited, proxies to Python)
	graphP := newProxy("", proxyOpt{logTag: "graph"})
	mux.Handle("GET /v1/graphs", authMW(rlMW(graphP(pathFn("/v1/graphs")))))
	mux.Handle("POST /v1/graphs", authMW(rlMW(graphP(pathFn("/v1/graphs")))))
	mux.Handle("GET /v1/graphs/{id}", authMW(rlMW(graphP(pathParam("/v1/graphs")))))
	mux.Handle("DELETE /v1/graphs/{id}", authMW(rlMW(graphP(pathParam("/v1/graphs")))))
	mux.Handle("POST /v1/graphs/{id}/execute", authMW(rlMW(graphP(pathParamSuffix("/v1/graphs", "/execute")))))

	// Workflow 鎵ц鐘舵€佷笌鍘嗗彶锛堜唬鐞?Python锛泂tatus 鏀寔鍐呭瓨瀹炰緥 + DB 鍥為€€锛?
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
		if pythonClient == nil {
			InternalError(w, "python engine not available")
			return
		}
		claims := auth.GetClaims(r.Context())
		pythonClient.ForwardRequest(w, r, "/v1/kb/"+r.PathValue("id")+"/documents?user_id="+claims.UserID+"&tenant_id="+claims.TenantID)
	}))))
	mux.Handle("GET /v1/kb/{id}/documents", authMW(kbRateMW(kbP(pathParamSuffix("/v1/kb", "/documents")))))
	mux.Handle("POST /v1/kb/{id}/build", authMW(kbRateMW(kbP(pathParamSuffix("/v1/kb", "/build")))))
	mux.Handle("POST /v1/kb/{id}/query", authMW(kbRateMW(kbP(pathParamSuffix("/v1/kb", "/query")))))
	// 鐭ヨ瘑搴撳垹闄ゆ枃妗ｏ紙P1 淇锛歅ython 绔凡鏈?DELETE /{kb_id}/documents?doc_id=锛?
	// 缃戝叧姝ゅ墠缂哄け璇ヨ矾鐢卞鑷村墠绔垹闄ゆ枃妗?404锛?
	mux.Handle("PUT /v1/kb/{id}/visibility", authMW(kbRateMW(http.HandlerFunc(handleKBVisibility))))
	mux.Handle("DELETE /v1/kb/{id}/documents", authMW(kbRateMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil {
			InternalError(w, "python engine not available")
			return
		}
		claims := auth.GetClaims(r.Context())
		if claims == nil || claims.TenantID == "" {
			Unauthorized(w, "missing tenant context")
			return
		}
		// 淇濈暀鍘熷 doc_id 鏌ヨ鍙傛暟锛坣ewProxy 鐨?buildPath 浼氫涪寮冨師濮?query锛?
		target := "/v1/kb/" + r.PathValue("id") + "/documents?doc_id=" + url.QueryEscape(r.URL.Query().Get("doc_id")) +
			"&user_id=" + claims.UserID + "&tenant_id=" + claims.TenantID
		var resp interface{}
		if err := pythonClient.DeleteJSON(r.Context(), target, &resp); err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, "python engine error")
			return
		}
		OK(w, resp)
	}))))

	// Unified chat / quick-execute (鍏ぇ宸ヤ綔鍙扮粺涓€鍏ュ彛, proxies to Python TaskRouter)
	chatP := newProxy("", proxyOpt{logTag: "chat"})
	mux.Handle("POST /v1/chat/submit", authMW(rlMW(chatP(pathFn("/v1/chat/submit")))))
	mux.Handle("GET /v1/chat/sessions/{id}/messages", authMW(rlMW(chatP(pathParamSuffix("/v1/chat/sessions", "/messages")))))
	// quick-execute 涓?chat/submit 鐨勮涔夊埆鍚嶏紙鍓嶇蹇嵎鎵ц鍏ュ彛锛?
	mux.Handle("POST /v1/quick-execute", authMW(rlMW(chatP(pathFn("/v1/chat/submit")))))

	// Capabilities discovery (鑳藉姏娉ㄥ唽涓績: 鍏ぇ宸ヤ綔鍙拌兘鍔涘彂鐜?
	capP := newProxy("", proxyOpt{logTag: "capabilities"})
	mux.Handle("GET /v1/capabilities", authMW(rlMW(capP(pathFn("/v1/capabilities")))))
	mux.Handle("POST /v1/capabilities/search", authMW(rlMW(capP(pathFn("/v1/capabilities/search")))))

	// Memory (鐢ㄦ埛闀挎湡璁板繂 L2 妗ｆ鍗? 鍒楄〃/CRUD/璇箟妫€绱?鏅鸿兘鏁寸悊, 浠ｇ悊 Python)
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
	mux.Handle("POST /v1/memory/conflicts/{conflict_id}/resolve", authMW(rlMW(memP(pathParamNamed("/v1/memory/conflicts", "conflict_id", "/resolve")))))
	mux.Handle("DELETE /v1/memory/conflicts/{conflict_id}", authMW(rlMW(memP(pathParamNamed("/v1/memory/conflicts", "conflict_id", "")))))
}

// 鈹€鈹€ Admin 鈹€鈹€

func registerAdminRoutes(
	mux *http.ServeMux,
	authMW routeMiddleware,
	rlMW routeMiddleware,
	adminHandler *AdminHandler,
	pythonClient *engine.PythonClient,
) {
	// Admin routes (auth + admin permission + rate limit)
	// P1-3: 鎵€鏈?admin 璺敱蹇呴』鎸傞檺娴侊紝闃叉琚姭鎸佺殑 admin token 鏃犻檺璋冪敤
	// 閫犳垚鐮村潖锛坆ackup/restore/users DELETE 绛夋晱鎰熸搷浣滐級銆?
	// 璇绘搷浣滅敤 PermAdminRead锛屽啓鎿嶄綔锛圥UT/DELETE/POST锛夊繀椤?PermAdminWrite銆?
	// P1-4: 鐢ㄦ埛绠＄悊璺敱锛圥UT/DELETE /v1/admin/users锛夊繀椤?PermUsersManage锛?
	// 璇ユ潈闄愪粎 owner 瑙掕壊鎸佹湁锛屾櫘閫?admin 涓嶅簲鑳藉垹/鏀圭敤鎴枫€?
	adminReadMW := RequirePermission(auth.PermAdminRead)
	adminWriteMW := RequirePermission(auth.PermAdminWrite)
	usersManageMW := RequirePermission(auth.PermUsersManage)
	adminMux := http.NewServeMux()
	adminHandler.RegisterRoutes(adminMux)
	// Strip the /v1/admin prefix so adminMux patterns (e.g. "GET /metrics") match correctly
	adminStrip := http.StripPrefix("/v1/admin", adminMux)
	mux.Handle("GET /v1/admin/metrics", authMW(rlMW(adminReadMW(adminStrip))))
	mux.Handle("GET /v1/admin/users", authMW(rlMW(adminReadMW(adminStrip))))
	mux.Handle("GET /v1/admin/users/{id}", authMW(rlMW(adminReadMW(adminStrip))))
	// 鐢ㄦ埛鍐欐搷浣滄敹绱т负 PermUsersManage锛堜粎 owner锛?
	mux.Handle("PUT /v1/admin/users/{id}", authMW(rlMW(usersManageMW(adminStrip))))
	mux.Handle("DELETE /v1/admin/users/{id}", authMW(rlMW(usersManageMW(adminStrip))))
	mux.Handle("GET /v1/admin/system", authMW(rlMW(adminReadMW(adminStrip))))
	mux.Handle("POST /v1/admin/maintenance", authMW(rlMW(adminWriteMW(adminStrip))))
	mux.Handle("POST /v1/admin/backup", authMW(rlMW(adminWriteMW(adminStrip))))
	mux.Handle("POST /v1/admin/restore", authMW(rlMW(adminWriteMW(adminStrip))))
	mux.Handle("GET /v1/admin/kb", authMW(rlMW(adminReadMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			InternalError(w, "python engine not available")
			return
		}
		claims := auth.GetClaims(r.Context())
		r.Header.Set("X-User-Role", claims.Role)
		pythonClient.ForwardRequest(w, r, "/v1/admin/kb?user_id="+claims.UserID+"&tenant_id="+claims.TenantID)
	})))))

	// Storage admin routes
	mux.Handle("GET /v1/admin/storage", authMW(rlMW(adminReadMW(adminStrip))))
	mux.Handle("PUT /v1/admin/storage", authMW(rlMW(adminWriteMW(adminStrip))))
	mux.Handle("POST /v1/admin/storage/test", authMW(rlMW(adminWriteMW(adminStrip))))

	// Redis admin routes
	mux.Handle("GET /v1/admin/redis", authMW(rlMW(adminReadMW(adminStrip))))
	mux.Handle("PUT /v1/admin/redis", authMW(rlMW(adminWriteMW(adminStrip))))
	mux.Handle("POST /v1/admin/redis/test", authMW(rlMW(adminWriteMW(adminStrip))))

	// Queue admin routes
	mux.Handle("GET /v1/admin/queue", authMW(rlMW(adminReadMW(adminStrip))))
	mux.Handle("POST /v1/admin/queue/flush", authMW(rlMW(adminWriteMW(adminStrip))))
	mux.Handle("POST /v1/admin/queue/pause", authMW(rlMW(adminWriteMW(adminStrip))))

	// Cache admin routes
	mux.Handle("GET /v1/admin/cache/stats", authMW(rlMW(adminReadMW(adminStrip))))

	// Performance admin routes
	mux.Handle("GET /v1/admin/performance", authMW(rlMW(adminReadMW(adminStrip))))

	// API Key admin routes (direct handlers, avoid adminMux path mismatch)
	mux.Handle("GET /v1/admin/api-keys", authMW(rlMW(adminReadMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			OK(w, map[string]interface{}{"keys": []interface{}{}, "stats": map[string]interface{}{"total": 0, "active": 0, "rate_limited": 0, "circuit_open": 0}})
			return
		}
		pythonClient.ForwardRequest(w, r, "/v1/admin/api-keys")
	})))))
	mux.Handle("POST /v1/admin/api-keys", authMW(rlMW(adminWriteMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			BadRequest(w, "python engine not available")
			return
		}
		pythonClient.ForwardRequest(w, r, "/v1/admin/api-keys")
	})))))
	mux.Handle("PUT /v1/admin/api-keys/{id}", authMW(rlMW(adminWriteMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			BadRequest(w, "python engine not available")
			return
		}
		pythonClient.ForwardRequest(w, r, "/v1/admin/api-keys/"+r.PathValue("id"))
	})))))
	mux.Handle("DELETE /v1/admin/api-keys/{id}", authMW(rlMW(adminWriteMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pythonClient == nil || !pythonClient.IsConnected() {
			BadRequest(w, "python engine not available")
			return
		}
		pythonClient.ForwardRequest(w, r, "/v1/admin/api-keys/"+r.PathValue("id"))
	})))))

	// Settings admin routes
	mux.Handle("PUT /v1/admin/settings", authMW(rlMW(adminWriteMW(adminStrip))))
}
