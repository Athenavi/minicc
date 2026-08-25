package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
)

// 鈹€鈹€ 浼佷笟瀹¤涓棿浠?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
//
// AuditMiddleware 鎸傚湪 authMW 涔嬪悗锛堥泦鎴愪换鍔?#22 鎺ョ嚎锛夛紝瀵?/v1/ent/* 涓?
// /admin/*锛堝惈 /v1/admin/*锛夌殑鍐欐柟娉曪紙POST/PUT/PATCH/DELETE锛夎褰曞璁°€?
//
// 淇鐜版湁 LoggingMiddleware 瀹¤ userID 鎭掔┖鐨勯棶棰橈細鏈腑闂翠欢浠?authMW
// 娉ㄥ叆鐨?claims 涓彇 userID锛堜笉鏀瑰姩鍘?LoggingMiddleware锛夈€?
// 瀹¤鍐欏叆璧?db.AuditLog锛圧edis Stream锛夛紝寮傛鎵ц涓嶉樆濉炶姹傘€?

// auditMWRecord 瀹¤鍐欏叆鍑芥暟锛屾娊鎴愬彉閲忎究浜庢祴璇曟浛鎹紙榛樿寮傛 db.AuditLog锛夈€?
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

// auditScopedPath 鍒ゆ柇璺緞鏄惁灞炰簬瀹¤绠℃帶鑼冨洿銆?
func auditScopedPath(path string) bool {
	return strings.HasPrefix(path, "/v1/ent/") ||
		strings.HasPrefix(path, "/admin/") ||
		strings.HasPrefix(path, "/v1/admin/")
}

// auditWriteMethod 鍒ゆ柇鏄惁涓哄啓鏂规硶銆?
func auditWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// AuditMiddleware 瀹¤涓棿浠讹紙闈炵鎺ц姹傞浂寮€閿€鐩撮€氾級銆?
func AuditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auditWriteMethod(r.Method) || !auditScopedPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// 鍖呰浠ユ崟鑾风姸鎬佺爜锛堝鐢?middleware.go 鐨?responseWriter锛?
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
