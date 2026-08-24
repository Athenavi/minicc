package api

import (
	"log/slog"
	"net/http"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/settings"
)

// internalTokenMW 校验 X-Internal-Token，供 Go 网关内部端点（引擎配置下发）使用。
// 缺失/不匹配时返回 401，绝不透出解密后的敏感配置。
func internalTokenMW(cfg *config.Config, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.InternalToken == "" || r.Header.Get("X-Internal-Token") != cfg.InternalToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// EngineConfig GET /v1/internal/engine-config
// 供 Python AI 引擎启动时拉取「python」分类的后台配置（敏感键已由 APP_SECRET 解密）。
// 仅接受带 X-Internal-Token 的内部调用，防止配置外泄。
func EngineConfig(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db.Pool == nil {
			NotFound(w, "database unavailable")
			return
		}
		store := settings.New(db.Pool, cfg.AppSecret)
		m, err := store.LoadConfig(r.Context(), "python")
		if err != nil {
			slog.Error("engine config load failed", "error", err)
			InternalError(w, "failed to load engine config")
			return
		}
		OK(w, m)
	}
}