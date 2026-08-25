package api

import (
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
)

// Activity 鑱氬悎鏈€杩戞椿鍔紙璺ㄥ伐浣滃彴锛夛紝渚涘墠绔?WorkstationNav 浣跨敤銆?type Activity struct {
	Workstation string `json:"workstation"`   // dialogue / agent / workflow / skill / knowledge / plugin
	Route       string `json:"route"`         // 璺宠浆璺敱
	Title       string `json:"title"`         // 娲诲姩鏍囬
	Status      string `json:"status"`        // 鍘熷鐘舵€?	StatusText  string `json:"status_text"`   // 灞曠ず鏂囨
	Timestamp   int64  `json:"timestamp"`     // Unix 姣
}

func activityStatusText(status string) string {
	switch status {
	case "running", "processing", "pending":
		return "杩涜涓?
	case "completed", "done", "active", "success":
		return "宸插畬鎴?
	case "failed", "error":
		return "澶辫触"
	case "uploading", "building":
		return "澶勭悊涓?
	default:
		return status
	}
}

// handleActivities 杩斿洖褰撳墠鐢ㄦ埛璺ㄥ叚澶у伐浣滃彴鐨勬渶杩戞椿鍔紙鎸夋椂闂村€掑簭锛岀鎴?鐢ㄦ埛闅旂锛夈€?// 浼樺寲锛氶€氳繃 UNION ALL 鍚堝苟涓夋鏌ヨ涓哄崟娆℃暟鎹簱寰€杩斻€?func handleActivities(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		Unauthorized(w, "missing tenant context")
		return
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}

	type item struct {
		workstation string
		route       string
		title       string
		status      string
		ts          time.Time
	}
	items := make([]item, 0, limit*3)
	ctx := r.Context()

	// Single combined query: UNION ALL across all three tables
	// Each sub-query returns (workstation, route, title, status, ts)
	const combinedSQL = `
		SELECT 'agent'::text, '/agents'::text, COALESCE(name,''), status, created_at FROM agent_sessions
			WHERE tenant_id = $1 AND user_id = $2 ORDER BY created_at DESC LIMIT $3
		UNION ALL
		SELECT 'dialogue'::text, '/chat'::text, COALESCE(title,''), 'active'::text, updated_at FROM sessions
			WHERE tenant_id = $1 AND user_id = $2 ORDER BY updated_at DESC LIMIT $3
		UNION ALL
		SELECT 'knowledge'::text, '/knowledge'::text, COALESCE(name,''), status, created_at FROM knowledge_documents
			WHERE tenant_id = $1 AND user_id = $2 ORDER BY created_at DESC LIMIT $3
		ORDER BY 5 DESC
		LIMIT $3
	`
	if rows, err := db.ReadPool().Query(ctx, combinedSQL, claims.TenantID, claims.UserID, limit*3); err == nil {
		for rows.Next() {
			var ws, route, title, status string
			var ts time.Time
			if err := rows.Scan(&ws, &route, &title, &status, &ts); err == nil {
				items = append(items, item{ws, route, title, status, ts})
			}
		}
		if err := rows.Err(); err != nil {
			slog.Warn("activities rows iteration error", "error", err)
		}
		rows.Close()
	}

	sort.Slice(items, func(i, j int) bool { return items[i].ts.After(items[j].ts) })
	if len(items) > limit {
		items = items[:limit]
	}

	out := make([]Activity, 0, len(items))
	for _, it := range items {
		out = append(out, Activity{
			Workstation: it.workstation,
			Route:       it.route,
			Title:       it.title,
			Status:      it.status,
			StatusText:  activityStatusText(it.status),
			Timestamp:   it.ts.UnixMilli(),
		})
	}
	OK(w, map[string]interface{}{"activities": out})
}
