package api

import (
	"log/slog"
	"net/http"

	"github.com/athenavi/chiron/config"
	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/settings"
)

// internalTokenMW 鏍￠獙 X-Internal-Token锛屼緵 Go 缃戝叧鍐呴儴绔偣锛堝紩鎿庨厤缃笅鍙戯級浣跨敤銆?// 缂哄け/涓嶅尮閰嶆椂杩斿洖 401锛岀粷涓嶉€忓嚭瑙ｅ瘑鍚庣殑鏁忔劅閰嶇疆銆?func internalTokenMW(cfg *config.Config, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.InternalToken == "" || r.Header.Get("X-Internal-Token") != cfg.InternalToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// EngineConfig GET /v1/internal/engine-config
// 渚?Python AI 寮曟搸鍚姩鏃舵媺鍙栥€宲ython銆嶅垎绫荤殑鍚庡彴閰嶇疆锛堟晱鎰熼敭宸茬敱 APP_SECRET 瑙ｅ瘑锛夈€?// 浠呮帴鍙楀甫 X-Internal-Token 鐨勫唴閮ㄨ皟鐢紝闃叉閰嶇疆澶栨硠銆?func EngineConfig(cfg *config.Config) http.HandlerFunc {
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
