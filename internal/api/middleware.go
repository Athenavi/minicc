package api

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/monitor"
)

type responseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
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

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rw, r)

		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"bytes", rw.bytes,
			"duration", time.Since(start).String(),
			"ip", r.RemoteAddr,
		)
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

func CORSMiddleware(allowOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allowed := false
			for _, o := range strings.Split(allowOrigin, ",") {
				if strings.TrimSpace(o) == origin || allowOrigin == "*" {
					allowed = true
					break
				}
			}
			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			if !allowed && allowOrigin != "*" {
				// Still need to handle preflight even for disallowed origins
				w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
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

func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// AuthMiddleware validates JWT from cookie or Authorization header.
func AuthMiddleware(a *auth.Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := ""

			// 1. Try cookie (primary for browser clients)
			if c, err := r.Cookie("minicc_token"); err == nil && c.Value != "" {
				tokenStr = c.Value
			}

			// 2. Try Authorization: Bearer <token> (for API clients)
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
					// Validate API key against PostgreSQL
					if db.Pool != nil {
						var userID, role string
						err := db.Pool.QueryRow(r.Context(),
							`SELECT u.id, u.role FROM users u
							 JOIN api_keys ak ON ak.user_id = u.id
							 WHERE ak.key_hash = sha256($1)`, key).Scan(&userID, &role)
						if err == nil {
							claims := &auth.Claims{
								UserID: userID,
								Role:   role,
								Perms:  auth.RolePermissions[role],
							}
							ctx := auth.WithClaims(r.Context(), claims)
							next.ServeHTTP(w, r.WithContext(ctx))
							return
						}
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
}

type visitor struct {
	count    int
	resetAt  time.Time
}

func NewRateLimiter(rpm int) *RateLimiter {
	return &RateLimiter{
		visitors: make(map[string]*visitor),
		rpm:      rpm,
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
		for {
			time.Sleep(interval)
			rl.mu.Lock()
			now := time.Now()
			for ip, v := range rl.visitors {
				if now.After(v.resetAt) {
					delete(rl.visitors, ip)
				}
			}
			rl.mu.Unlock()
		}
	}()
}

// MonitoringMiddleware tracks request counts.
func MonitoringMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		monitor.IncRequests()
		defer monitor.DecRequests()
		next.ServeHTTP(w, r)
	})
}
