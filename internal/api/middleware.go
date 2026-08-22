package api

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/monitor"
)

type responseWriter struct {
	http.ResponseWriter
	status  int
	bytes   int
	flusher http.Flusher
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

func (rw *responseWriter) Flush() {
	if rw.flusher != nil {
		rw.flusher.Flush()
	}
}

// Hijack implements http.Hijacker, required for WebSocket upgrades through middleware.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		var flusher http.Flusher
		if f, ok := w.(http.Flusher); ok {
			flusher = f
		}
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK, flusher: flusher}

		next.ServeHTTP(rw, r)

		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"bytes", rw.bytes,
			"duration", time.Since(start).String(),
			"ip", r.RemoteAddr,
		)

		// Write audit log for non-GET requests
		if r.Method != "GET" && r.Method != "OPTIONS" && rw.status < 500 {
			db.AuditLog(r.Context(), "", r.Method, r.URL.Path, "", r.RemoteAddr, map[string]interface{}{
				"status": rw.status,
				"method": r.Method,
				"path":   r.URL.Path,
			})
		}
	})
}

// TracingMiddleware creates a span for each HTTP request and attaches it to the context.
// Must be placed early in the middleware chain so downstream handlers can use monitor.GetSpan.
func TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := monitor.StartSpan(r.Context(), r.Method+" "+r.URL.Path, "server")
		span.SetTag("http.method", r.Method)
		span.SetTag("http.path", r.URL.Path)
		span.SetTag("http.remote_addr", r.RemoteAddr)
		defer span.End()

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "panic", rec, "path", r.URL.Path)
				InternalError(w, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CORSMiddleware 处理 CORS。allowOrigin 是逗号分隔白名单；"*" 在 AllowCredentials=true
// 下违反 CORS 规范且高危，显式拒绝。
func CORSMiddleware(allowOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allowed := false
			if allowOrigin != "*" && origin != "" {
				for _, o := range strings.Split(allowOrigin, ",") {
					if strings.TrimSpace(o) == origin {
						allowed = true
						break
					}
				}
			}
			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-API-Key")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeadersMiddleware 注入通用安全响应头。
// CSP_CONNECT_SRC 通过 env 注入（默认留空则不强制 connect-src 白名单，
// 由部署方按生产域名配置，避免 localhost 写死导致生产环境被阻断）。
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	cspConnectSrc := strings.TrimSpace(os.Getenv("CSP_CONNECT_SRC"))
	if cspConnectSrc == "" {
		cspConnectSrc = "'self'"
	}
	csp := "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: blob:; connect-src " + cspConnectSrc + "; " +
		"font-src 'self' https://fonts.gstatic.com; frame-ancestors 'none'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

// AuthMiddleware validates JWT from cookie or Authorization header.
func AuthMiddleware(a *auth.Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := ""

			// 1. Cookie (primary for browser clients; SSE 经 withCredentials 携带)
			if c, err := r.Cookie("minicc_token"); err == nil && c.Value != "" {
				tokenStr = c.Value
			}

			// 2. Authorization: Bearer <token> (for API clients)
			if tokenStr == "" {
				if ah := r.Header.Get("Authorization"); ah != "" {
					if strings.HasPrefix(ah, "Bearer ") {
						tokenStr = strings.TrimPrefix(ah, "Bearer ")
					}
				}
			}

			// 2. Try X-API-Key header
		if tokenStr == "" {
			if key := r.Header.Get("X-API-Key"); key != "" {
				// Validate API key against PostgreSQL（含 tenant_id 与 revoked 状态校验，多租户隔离）
				var userID, role, tenantID string
				keyHash := sha256.Sum256([]byte(key))
				err := db.ReadPool().QueryRow(r.Context(),
					`SELECT u.id, u.role, COALESCE(u.tenant_id, '') AS tenant_id
					 FROM users u
					 JOIN api_keys ak ON ak.user_id = u.id
					 WHERE ak.key_hash = $1
					   AND COALESCE(ak.revoked, false) = false
					   AND (ak.expires_at IS NULL OR ak.expires_at > NOW())`,
					hex.EncodeToString(keyHash[:])).Scan(&userID, &role, &tenantID)
				if err == nil {
					if tenantID == "" {
						tenantID = db.DefaultTenantID // 单租户兼容
					}
					claims := &auth.Claims{
						UserID:   userID,
						Role:     role,
						TenantID: tenantID,
						Perms:    auth.RolePermissions[role],
					}
					ctx := auth.WithClaims(r.Context(), claims)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				Unauthorized(w, "invalid API key")
				return
			}
		}

			if tokenStr == "" {
				Unauthorized(w, "missing authorization")
				return
			}

			claims, err := a.ValidateToken(tokenStr)
			if err != nil {
				Unauthorized(w, "invalid or expired token")
				return
			}

			// ── JWT 黑名单检查（登出后的 token 立即失效）──
			if claims.ID != "" && db.Redis != nil {
				blacklisted, err := db.Redis.Exists(r.Context(), "jwt:blacklist:"+claims.ID).Result()
				if err != nil {
					slog.Warn("jwt blacklist check failed", "error", err)
				}
				if blacklisted > 0 {
					Unauthorized(w, "token has been revoked")
					return
				}
			}

			ctx := auth.WithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequirePermission checks that the authenticated user has the specified permission.
func RequirePermission(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := auth.GetClaims(r.Context())
			if !auth.HasPermission(claims, perm) {
				Forbidden(w, "insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitMiddleware implements a simple in-memory token bucket per IP.
// For production, this should use Redis (sliding window).
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rpm      int
	stopCh   chan struct{}
}

type visitor struct {
	count   int
	resetAt time.Time
}

func NewRateLimiter(rpm int) *RateLimiter {
	return &RateLimiter{
		visitors: make(map[string]*visitor),
		rpm:      rpm,
		stopCh:   make(chan struct{}),
	}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if idx := strings.LastIndex(ip, ":"); idx >= 0 {
			ip = ip[:idx]
		}

		rl.mu.Lock()
		v, exists := rl.visitors[ip]
		now := time.Now()

		if !exists || now.After(v.resetAt) {
			rl.visitors[ip] = &visitor{count: 1, resetAt: now.Add(1 * time.Minute)}
			rl.mu.Unlock()
			next.ServeHTTP(w, r)
			return
		}

		if v.count >= rl.rpm {
			rl.mu.Unlock()
			TooManyRequests(w)
			return
		}

		v.count++
		rl.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}

// CleanupVisitors periodically removes stale entries.
func (rl *RateLimiter) CleanupVisitors(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("rate limiter cleanup panic", "panic", r)
			}
		}()
		for {
			select {
			case <-ticker.C:
				rl.mu.Lock()
				now := time.Now()
				for ip, v := range rl.visitors {
					if now.After(v.resetAt) {
						delete(rl.visitors, ip)
					}
				}
				rl.mu.Unlock()
			case <-rl.stopCh:
				return
			}
		}
	}()
}

// Stop cleanly terminates the cleanup goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// MonitoringMiddleware tracks request counts.
func MonitoringMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		monitor.IncRequests()
		defer monitor.DecRequests()
		next.ServeHTTP(w, r)
	})
}
