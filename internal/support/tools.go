package support

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/tools"
)

func genID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func nullableStr(s string) *string {
	if s == "" { return nil }
	return &s
}

// ── TicketTool ──

type TicketTool struct{}

func NewTicketTool() *TicketTool { return &TicketTool{} }
func (t *TicketTool) Name() string       { return "support_ticket_create" }
func (t *TicketTool) Description() string { return "Create a customer support ticket." }

func (t *TicketTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	subject, _ := input["subject"].(string)
	if subject == "" { return nil, fmt.Errorf("subject is required") }
	desc, _ := input["description"].(string)
	priority, _ := input["priority"].(string)
	if priority == "" { priority = "medium" }

	id := genID("tkt")

	if db.Pool != nil {
		db.Pool.Exec(ctx,
			`INSERT INTO support_tickets (id, subject, description, priority, status, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, 'open', NOW(), NOW())`,
			id, subject, nullableStr(desc), priority)
	}

	return map[string]interface{}{
		"output":   fmt.Sprintf("Support ticket created: %s (ID: %s, priority: %s)", subject, id, priority),
		"id":       id,
		"subject":  subject,
		"priority": priority,
	}, nil
}

// ── KBSearchTool ──

type KBSearchTool struct{}

func NewKBSearchTool() *KBSearchTool { return &KBSearchTool{} }
func (t *KBSearchTool) Name() string       { return "support_kb_search" }
func (t *KBSearchTool) Description() string { return "Search the knowledge base." }

func (t *KBSearchTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	query, _ := input["query"].(string)
	if query == "" { return nil, fmt.Errorf("query is required") }

	results := []map[string]string{}
	if db.Pool != nil {
		rows, err := db.Pool.Query(ctx,
			`SELECT id, title, category FROM kb_articles
			 WHERE title ILIKE '%' || $1 || '%' OR content ILIKE '%' || $1 || '%'
			 ORDER BY updated_at DESC LIMIT 20`, query)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id, title, cat string
				if rows.Scan(&id, &title, &cat) == nil {
					results = append(results, map[string]string{"id": id, "title": title, "category": cat})
				}
			}
		}
	}

	if len(results) == 0 {
		return map[string]interface{}{
			"output": fmt.Sprintf("No KB articles found for '%s'", query),
			"total":  0,
		}, nil
	}

	output := fmt.Sprintf("KB articles for '%s' (%d found):\n", query, len(results))
	for _, r := range results {
		cat := r["category"]
		if cat != "" { cat = " [" + cat + "]" }
		output += fmt.Sprintf("  - %s (ID: %s)%s\n", r["title"], r["id"], cat)
	}

	return map[string]interface{}{
		"output":  output,
		"total":   len(results),
		"results": results,
	}, nil
}

// ── ChatbotTool ──

type ChatbotTool struct{}

func NewChatbotTool() *ChatbotTool { return &ChatbotTool{} }
func (t *ChatbotTool) Name() string       { return "support_chatbot_reply" }
func (t *ChatbotTool) Description() string { return "Generate a chatbot reply." }

func (t *ChatbotTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	question, _ := input["question"].(string)
	if question == "" { return nil, fmt.Errorf("question is required") }

	// Store interaction in messages
	if db.Pool != nil {
		qID := genID("chat")
		db.Pool.Exec(ctx,
			`INSERT INTO messages (id, session_id, role, content, created_at)
			 VALUES ($1, 'support_chatbot', 'user', $2, NOW())`,
			qID, question)
	}

	return map[string]interface{}{
		"output":   fmt.Sprintf("Chatbot received question (%d chars). An agent will follow up if needed.", len(question)),
		"question": question,
	}, nil
}

// ── CampaignTool ──

type CampaignTool struct{}

func NewCampaignTool() *CampaignTool { return &CampaignTool{} }
func (t *CampaignTool) Name() string       { return "marketing_campaign_create" }
func (t *CampaignTool) Description() string { return "Create a marketing campaign." }

func (t *CampaignTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	name, _ := input["name"].(string)
	if name == "" { return nil, fmt.Errorf("name is required") }
	desc, _ := input["description"].(string)
	cType, _ := input["type"].(string)
	if cType == "" { cType = "email" }

	config := map[string]interface{}{}
	if cfg, ok := input["config"].(map[string]interface{}); ok {
		config = cfg
	}
	configJSON, _ := json.Marshal(config)

	id := genID("camp")

	if db.Pool != nil {
		db.Pool.Exec(ctx,
			`INSERT INTO marketing_campaigns (id, name, description, campaign_type, config, status, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, 'draft', NOW(), NOW())`,
			id, name, nullableStr(desc), cType, string(configJSON))
	}

	return map[string]interface{}{
		"output": fmt.Sprintf("Campaign created: %s (ID: %s, type: %s)", name, id, cType),
		"id":     id,
		"name":   name,
		"type":   cType,
	}, nil
}

// ── ABTestTool ──

type ABTestTool struct{}

func NewABTestTool() *ABTestTool { return &ABTestTool{} }
func (t *ABTestTool) Name() string       { return "marketing_abtest" }
func (t *ABTestTool) Description() string { return "Create an A/B test." }

func (t *ABTestTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	name, _ := input["name"].(string)
	if name == "" { return nil, fmt.Errorf("name is required") }

	config := map[string]interface{}{
		"type": "abtest",
	}
	if v, ok := input["variants"]; ok { config["variants"] = v }
	if m, ok := input["metric"]; ok { config["metric"] = m }
	configJSON, _ := json.Marshal(config)

	id := genID("ab")

	if db.Pool != nil {
		db.Pool.Exec(ctx,
			`INSERT INTO marketing_campaigns (id, name, campaign_type, config, status, created_at, updated_at)
			 VALUES ($1, $2, 'abtest', $3, 'draft', NOW(), NOW())`,
			id, name, string(configJSON))
	}

	return map[string]interface{}{
		"output": fmt.Sprintf("A/B test created: %s (ID: %s)", name, id),
		"id":     id,
		"name":   name,
	}, nil
}

func RegisterTools(tr *tools.ToolRegistry) {
	for _, t := range []tools.Tool{NewTicketTool(), NewKBSearchTool(), NewChatbotTool(), NewCampaignTool(), NewABTestTool()} {
		tr.Register(t)
	}
}
