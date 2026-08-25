package api

import (
	"bufio"
	"context"
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

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/monitor"
)

type responseWriter struct {
	http.ResponseWriter
	status  int
	bytes   int
	flusher http.Flusher
}
// 鈹€鈹€ JWT 榛戝悕鍗曟湰鍦?TTL 缂撳瓨锛圥1 浼樺寲锛氬噺灏戠儹璺緞姣忔璇锋眰鐨?Redis 寰€杩旓級鈹€鈹€
// 姝ｇ紦瀛橈紙宸叉媺榛戯級15 鍒嗛挓鏈夋晥锛涜礋缂撳瓨锛堟湭鎷夐粦锛変粎 30 绉掞紝纭繚鐧诲嚭鎾ら攢
// 鍦?鈮?0s 鍐呭叏灞€鐢熸晥锛堝鍓湰浠嶄互 Redis 涓烘渶缁堜簨瀹炴簮锛夈€?type jwtBlacklistEntry struct {
	blacklisted bool
	checkedAt   time.Time
}

var jwtBlacklistCache sync.Map // jti -> jwtBlacklistEntry

const (
	jwtBlacklistHitTTL  = 15 * time.Minute
	jwtBlacklistMissTTL = 30 * time.Second
)

// checkJWTBlacklisted 浼樺厛鏌ユ湰鍦扮紦瀛橈紝miss 鏃跺洖婧?Redis 骞跺洖濉€?func checkJWTBlacklisted(ctx context.Context, jti string) (bool, error) {
	if v, ok := jwtBlacklistCache.Load(jti); ok {
		e := v.(jwtBlacklistEntry)
		ttl := jwtBlacklistMissTTL
		if e.blacklisted {
			ttl = jwtBlacklistHitTTL
		}
		if time.Since(e.checkedAt) < ttl {
			return e.blacklisted, nil
		}
	}
	if db.Redis == nil {
		return false, nil
	}
	n, err := db.Redis.Exists(ctx, "jwt:blacklist:"+jti).Result()
	if err != nil {
		return false, err
	}
	jwtBlacklistCache.Store(jti, jwtBlacklistEntry{blacklisted: n > 0, checkedAt: time.Now()})
	return n > 0, nil
}

// markJWTBlacklisted 鐧诲嚭鏃跺悓姝ユ湰鍦版缂撳瓨锛堥厤鍚?Redis 鍐欏叆锛夈€?func markJWTBlacklisted(jti string) {
	if jti == "" {
		return
	}
	jwtBlacklistCache.Store(jti, jwtBlacklistEntry{blacklisted: true, checkedAt: time.Now()})
}

// StartBlacklistCleaner 瀹氭湡娓呯悊杩囨湡鐨?JWT 榛戝悕鍗曟湰鍦扮紦瀛樻潯鐩紝闃叉鍐呭瓨娉勬紡銆?// 姣?10 鍒嗛挓鎵弿涓€娆★紝鍒犻櫎瓒呰繃 1 灏忔椂鏈洿鏂扮殑鏉＄洰銆?func StartBlacklistCleaner(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				expired := 0
				cutoff := time.Now().Add(-1 * time.Hour)
				jwtBlacklistCache.Range(func(key, value any) bool {
					e := value.(jwtBlacklistEntry)
					if e.checkedAt.Before(cutoff) {
						jwtBlacklistCache.Delete(key)
						expired++
					}
					return true
				})
				if expired > 0 {
					slog.Debug("cleaned expired JWT blacklist cache entries", "count", expired)
				}
			}
		}
	}()
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
			auditUserID := ""
			if claims := auth.GetClaims(r.Context()); claims != nil {
				auditUserID = claims.UserID
			}
			db.AuditLog(r.Context(), auditUserID, r.Method, r.URL.Path, "", r.RemoteAddr, map[string]interface{}{
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

// CORSMiddleware 澶勭悊 CORS銆俛llowOrigin 鏄€楀彿鍒嗛殧鐧藉悕鍗曪紱"*" 鍦?AllowCredentials=true
// 涓嬭繚鍙?CORS 瑙勮寖涓旈珮鍗憋紝鏄惧紡鎷掔粷銆?func CORSMiddleware(allowOrigin string) func(http.Handler) http.Handler {
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

// SecurityHeadersMiddleware 娉ㄥ叆閫氱敤瀹夊叏鍝嶅簲澶淬€?// CSP_CONNECT_SRC 閫氳繃 env 娉ㄥ叆锛堥粯璁ょ暀绌哄垯涓嶅己鍒?connect-src 鐧藉悕鍗曪紝
// 鐢遍儴缃叉柟鎸夌敓浜у煙鍚嶉厤缃紝閬垮厤 localhost 鍐欐瀵艰嚧鐢熶骇鐜琚樆鏂級銆?func SecurityHeadersMiddleware(next http.Handler) http.Handler {
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

			// 1. Cookie (primary for browser clients; SSE 缁?withCredentials 鎼哄甫)
			if c, err := r.Cookie("chiron_token"); err == nil && c.Value != "" {
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
				// Validate API key against PostgreSQL锛堝惈 tenant_id 涓?revoked 鐘舵€佹牎楠岋紝澶氱鎴烽殧绂伙級
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
				// P1-5: tenant_id 涓虹┖鐩存帴鎷掔粷锛屼笉鍐嶅洖閫€ DefaultTenantID銆?				// 鍘嗗彶鏁版嵁涓?tenant_id=NULL 鐨?user 璧?DefaultTenantID 浼氳惤鍒伴粯璁ょ鎴凤紝
				// 閫犳垚璺ㄧ鎴锋暟鎹闂紱澶氱鎴烽儴缃插繀椤诲己鍒舵瘡涓敤鎴风粦瀹氱鎴枫€?				if tenantID == "" {
					slog.Warn("API key bound to user with null tenant_id, rejecting",
						"user_id", userID)
					Unauthorized(w, "user has no tenant binding; contact admin")
					return
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

			// 鈹€鈹€ JWT 榛戝悕鍗曟鏌ワ紙鐧诲嚭鍚庣殑 token 绔嬪嵆澶辨晥锛夆攢鈹€
			if claims.ID != "" {
				blacklisted, err := checkJWTBlacklisted(r.Context(), claims.ID)
				if err != nil {
					slog.Warn("jwt blacklist check failed", "error", err)
				}
				if blacklisted {
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
