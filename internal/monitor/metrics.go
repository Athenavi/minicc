package monitor

import (
	"log/slog"
	"time"
)

// Metrics holds simple counters for monitoring.
type Metrics struct {
	RequestsTotal   int64
	RequestsActive  int64
	LLMCallsTotal   int64
	LLMErrorsTotal  int64
	ToolCallsTotal  int64
	ToolErrorsTotal int64
	StartTime       time.Time
}

var Global = &Metrics{StartTime: time.Now()}

func IncRequests() {
	Global.RequestsTotal++
	Global.RequestsActive++
}

func DecRequests() {
	Global.RequestsActive--
}

func IncLLMCall() {
	Global.LLMCallsTotal++
}

func IncLLMError() {
	Global.LLMErrorsTotal++
}

func IncToolCall() {
	Global.ToolCallsTotal++
}

func IncToolError() {
	Global.ToolErrorsTotal++
}

func Snapshot() map[string]interface{} {
	return map[string]interface{}{
		"requests_total":   Global.RequestsTotal,
		"requests_active":  Global.RequestsActive,
		"llm_calls":        Global.LLMCallsTotal,
		"llm_errors":       Global.LLMErrorsTotal,
		"tool_calls":       Global.ToolCallsTotal,
		"tool_errors":      Global.ToolErrorsTotal,
		"uptime_seconds":   time.Since(Global.StartTime).Seconds(),
		"started_at":       Global.StartTime.Format(time.RFC3339),
	}
}

func Init() {
	slog.Info("monitor initialized", "started_at", Global.StartTime.Format(time.RFC3339))
}
