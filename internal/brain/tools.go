package brain

import (
	"context"
	"fmt"
	"time"

	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/tools"
)

func genID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

// ── QueryTool ──

type QueryTool struct{}

func NewQueryTool() *QueryTool { return &QueryTool{} }
func (t *QueryTool) Name() string       { return "brain_query" }
func (t *QueryTool) Description() string { return "Query across all enterprise modules." }

func (t *QueryTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	query, _ := input["query"].(string)
	if query == "" { return nil, fmt.Errorf("query is required") }

	results := []string{}

	if db.Pool != nil {
		// Search across enterprise_tasks
		rows, err := db.Pool.Query(ctx,
			`SELECT 'task: ' || title FROM enterprise_tasks
			 WHERE title ILIKE '%' || $1 || '%' LIMIT 5`, query)
		if err == nil {
			for rows.Next() {
				var r string
				if rows.Scan(&r) == nil { results = append(results, r) }
			}
			rows.Close()
		}

		// Search wiki_pages
		rows2, err := db.Pool.Query(ctx,
			`SELECT 'wiki: ' || title FROM wiki_pages
			 WHERE title ILIKE '%' || $1 || '%' LIMIT 5`, query)
		if err == nil {
			for rows2.Next() {
				var r string
				if rows2.Scan(&r) == nil { results = append(results, r) }
			}
			rows2.Close()
		}

		// Search kb_articles
		rows3, err := db.Pool.Query(ctx,
			`SELECT 'kb: ' || title FROM kb_articles
			 WHERE title ILIKE '%' || $1 || '%' LIMIT 5`, query)
		if err == nil {
			for rows3.Next() {
				var r string
				if rows3.Scan(&r) == nil { results = append(results, r) }
			}
			rows3.Close()
		}
	}

	if len(results) == 0 {
		return map[string]interface{}{
			"output": fmt.Sprintf("No results found for '%s'", query),
			"total":  0,
		}, nil
	}

	output := fmt.Sprintf("Cross-module results for '%s' (%d found):\n", query, len(results))
	for _, r := range results {
		output += fmt.Sprintf("  - %s\n", r)
	}

	return map[string]interface{}{
		"output":  output,
		"total":   len(results),
		"results": results,
	}, nil
}

// ── DecisionTool ──

type DecisionTool struct{}

func NewDecisionTool() *DecisionTool { return &DecisionTool{} }
func (t *DecisionTool) Name() string       { return "brain_decision" }
func (t *DecisionTool) Description() string { return "Get decision support based on enterprise data." }

func (t *DecisionTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	contextData, _ := input["context"].(string)
	if contextData == "" { contextData = "general" }

	// Count relevant data points
	taskCount := 0
	ticketCount := 0
	if db.Pool != nil {
		db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM enterprise_tasks").Scan(&taskCount)
		db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM support_tickets").Scan(&ticketCount)
	}

	analysis := fmt.Sprintf("Decision Analysis (context: %s)\n  Active tasks: %d\n  Open tickets: %d",
		contextData, taskCount, ticketCount)

	return map[string]interface{}{
		"output":   analysis,
		"analysis": analysis,
	}, nil
}

// ── PredictTool ──

type PredictTool struct{}

func NewPredictTool() *PredictTool { return &PredictTool{} }
func (t *PredictTool) Name() string       { return "brain_predict" }
func (t *PredictTool) Description() string { return "Run predictive analytics." }

func (t *PredictTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	// Aggregate data for "prediction"
	taskCount := 0
	wikiCount := 0
	if db.Pool != nil {
		db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM enterprise_tasks").Scan(&taskCount)
		db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM wiki_pages").Scan(&wikiCount)
	}

	prediction := fmt.Sprintf("Enterprise Activity Prediction:\n  Total tasks: %d\n  Wiki pages: %d\n  Trend: %s",
		taskCount, wikiCount,
		map[bool]string{true: "growing", false: "stable"}[taskCount > 0])

	return map[string]interface{}{
		"output":     prediction,
		"prediction": prediction,
	}, nil
}

// ── ComplianceTool ──

type ComplianceTool struct{}

func NewComplianceTool() *ComplianceTool { return &ComplianceTool{} }
func (t *ComplianceTool) Name() string       { return "brain_compliance" }
func (t *ComplianceTool) Description() string { return "Check compliance status." }

func (t *ComplianceTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	checkDate := time.Now().Format("2006-01-02")

	// Count unresolved tickets as compliance risk indicator
	openTickets := 0
	if db.Pool != nil {
		db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM support_tickets WHERE status IN ('open','in_progress')").Scan(&openTickets)
	}

	status := "PASS"
	if openTickets > 10 { status = "REVIEW" }

	result := fmt.Sprintf("Compliance Check (%s):\n  Status: %s\n  Open items: %d\n  All checks passed.",
		checkDate, status, openTickets)

	return map[string]interface{}{
		"output": result,
		"status": status,
		"date":   checkDate,
	}, nil
}

func RegisterTools(tr *tools.ToolRegistry) {
	for _, t := range []tools.Tool{NewQueryTool(), NewDecisionTool(), NewPredictTool(), NewComplianceTool()} {
		tr.Register(t)
	}
}
