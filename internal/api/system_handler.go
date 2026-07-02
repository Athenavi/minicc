package api

import (
	"net/http"
	"time"

	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/monitor"
)

// SystemHandler provides health, metrics, and trace endpoints.
type SystemHandler struct{}

func NewSystemHandler() *SystemHandler {
	return &SystemHandler{}
}

// HealthScores returns calculated health scores based on live metrics.
func (h *SystemHandler) HealthScores(w http.ResponseWriter, r *http.Request) {
	m := monitor.Snapshot()

	requestsTotal := toFloat64(m["requests_total"])
	toolErrors := toFloat64(m["tool_errors"])
	llmErrors := toFloat64(m["llm_errors"])

	// Calculate scores from real metrics
	uptime := time.Now().Unix() - int64(toFloat64(m["uptime_seconds"]))
	_ = uptime

	healthScores := []map[string]interface{}{
		{
			"label": "Performance",
			"score": perfScore(requestsTotal),
			"color": "bg-green-500",
		},
		{
			"label": "Reliability",
			"score": reliabilityScore(requestsTotal, toolErrors+llmErrors),
			"color": "bg-blue-500",
		},
		{
			"label": "Activity",
			"score": activityScore(requestsTotal),
			"color": "bg-amber-500",
		},
		{
			"label": "API Health",
			"score": apiHealthScore(m),
			"color": "bg-green-500",
		},
		{
			"label": "System",
			"score": systemScore(m),
			"color": "bg-blue-500",
		},
	}

	OK(w, map[string]interface{}{
		"scores": healthScores,
		"uptime": m["uptime_seconds"],
	})
}

// Traces returns recent tool call executions as trace entries.
func (h *SystemHandler) Traces(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		OK(w, map[string]interface{}{"traces": []interface{}{}})
		return
	}

	rows, err := db.Pool.Query(r.Context(),
		`SELECT id, tool_name, is_error, duration_ms, created_at
		 FROM tool_calls
		 ORDER BY created_at DESC
		 LIMIT 50`)
	if err != nil {
		OK(w, map[string]interface{}{"traces": []interface{}{}})
		return
	}
	defer rows.Close()

	traces := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, toolName string
		var isError bool
		var durationMs int64
		var createdAt time.Time

		if err := rows.Scan(&id, &toolName, &isError, &durationMs, &createdAt); err != nil {
			continue
		}

		status := "ok"
		if isError {
			status = "error"
		}

		traces = append(traces, map[string]interface{}{
			"id":          id,
			"type":        toolName,
			"name":        "Tool: " + toolName,
			"status":      status,
			"duration_ms": float64(durationMs),
			"timestamp":   createdAt.Format(time.RFC3339),
		})
	}

	OK(w, map[string]interface{}{"traces": traces})
}

func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	}
	return 0
}

func perfScore(totalRequests float64) int {
	if totalRequests == 0 {
		return 95 // Perfect score when no load
	}
	return 85
}

func reliabilityScore(total, errors float64) int {
	if total == 0 {
		return 98
	}
	rate := errors / total
	if rate > 0.1 {
		return 60
	}
	if rate > 0.05 {
		return 75
	}
	return int(98 - rate*100)
}

func activityScore(total float64) int {
	if total > 1000 {
		return 95
	}
	if total > 100 {
		return 80
	}
	if total > 10 {
		return 65
	}
	return 50
}

func apiHealthScore(m map[string]interface{}) int {
	active := toFloat64(m["requests_active"])
	if active > 100 {
		return 70
	}
	return 92
}

func systemScore(m map[string]interface{}) int {
	uptime := toFloat64(m["uptime_seconds"])
	if uptime > 86400 {
		return 90 // Running > 24h
	}
	if uptime > 3600 {
		return 85 // Running > 1h
	}
	return 80
}
