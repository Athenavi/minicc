package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
)

// ── 企业审计中间件 ───────────────────────────────────────────────────────
//
// AuditMiddleware 挂在 authMW 之后（集成任务 #22 接线），对 /v1/ent/* 与
// /admin/*（含 /v1/admin/*）的写方法（POST/PUT/PATCH/DELETE）记录审计。
//
// 修复现有 LoggingMiddleware 审计 userID 恒空的问题：本中间件从 authMW
// 注入的 claims 中取 userID（不改动原 LoggingMiddleware）。
// 审计写入走 db.AuditLog（Redis Stream），异步执行不阻塞请求。

// auditMWRecord 审计写入函数，抽成变量便于测试替换（默认异步 db.AuditLog）。
var auditMWRecord = func(userID, action, resource, detail, ip string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("audit middleware: record panic", "panic", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		db.AuditLog(ctx, userID, action, resource, detail, ip, nil)
	}()
}

// auditScopedPath 判断路径是否属于审计管控范围。
func auditScopedPath(path string) bool {
	return strings.HasPrefix(path, "/v1/ent/") ||
		strings.HasPrefix(path, "/admin/") ||
		strings.HasPrefix(path, "/v1/admin/")
}

// auditWriteMethod 判断是否为写方法。
func auditWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// AuditMiddleware 审计中间件（非管控请求零开销直通）。
func AuditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auditWriteMethod(r.Method) || !auditScopedPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// 包装以捕获状态码（复用 middleware.go 的 responseWriter）
		var flusher http.Flusher
		if f, ok := w.(http.Flusher); ok {
			flusher = f
		}
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK, flusher: flusher}
		next.ServeHTTP(rw, r)

		userID := ""
		if claims := auth.GetClaims(r.Context()); claims != nil {
			userID = claims.UserID
		}
		action := r.Method + " " + r.URL.Path
		if len(action) > 64 {
			action = action[:64]
		}
		auditMWRecord(userID, action, r.URL.Path, fmt.Sprintf("status=%d", rw.status), r.RemoteAddr)
	})
}
