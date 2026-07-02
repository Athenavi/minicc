package support

import (
	"context"
	"fmt"

	"github.com/athenavi/minicc/internal/tools"
)

// TicketTool creates a support ticket.
type TicketTool struct{}

func NewTicketTool() *TicketTool { return &TicketTool{} }
func (t *TicketTool) Name() string       { return "support_ticket_create" }
func (t *TicketTool) Description() string { return "Create a customer support ticket." }

func (t *TicketTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	subject, _ := input["subject"].(string)
	if subject == "" { return nil, fmt.Errorf("subject is required") }
	priority, _ := input["priority"].(string)
	if priority == "" { priority = "medium" }
	return map[string]interface{}{
		"output":   fmt.Sprintf("Ticket created: %s (priority: %s)", subject, priority),
		"subject":  subject,
		"priority": priority,
	}, nil
}

// KBSearchTool searches the knowledge base.
type KBSearchTool struct{}

func NewKBSearchTool() *KBSearchTool { return &KBSearchTool{} }
func (t *KBSearchTool) Name() string       { return "support_kb_search" }
func (t *KBSearchTool) Description() string { return "Search the knowledge base for answers." }

func (t *KBSearchTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	query, _ := input["query"].(string)
	if query == "" { return nil, fmt.Errorf("query is required") }
	return map[string]interface{}{
		"output": fmt.Sprintf("KB results for '%s':\n  (simulated)", query),
	}, nil
}

// ChatbotTool generates a chatbot reply.
type ChatbotTool struct{}

func NewChatbotTool() *ChatbotTool { return &ChatbotTool{} }
func (t *ChatbotTool) Name() string       { return "support_chatbot_reply" }
func (t *ChatbotTool) Description() string { return "Generate an AI chatbot reply to a customer question." }

func (t *ChatbotTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	question, _ := input["question"].(string)
	if question == "" { return nil, fmt.Errorf("question is required") }
	return map[string]interface{}{
		"output":  fmt.Sprintf("Chatbot reply to: %s\n  (simulated AI response)", question),
	}, nil
}

// CampaignTool creates a marketing campaign.
type CampaignTool struct{}

func NewCampaignTool() *CampaignTool { return &CampaignTool{} }
func (t *CampaignTool) Name() string       { return "marketing_campaign_create" }
func (t *CampaignTool) Description() string { return "Create an automated marketing campaign." }

func (t *CampaignTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	name, _ := input["name"].(string)
	if name == "" { return nil, fmt.Errorf("name is required") }
	return map[string]interface{}{
		"output": fmt.Sprintf("Campaign created: %s", name),
	}, nil
}

// ABTestTool runs an A/B test.
type ABTestTool struct{}

func NewABTestTool() *ABTestTool { return &ABTestTool{} }
func (t *ABTestTool) Name() string       { return "marketing_abtest" }
func (t *ABTestTool) Description() string { return "Run an A/B test experiment." }

func (t *ABTestTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"output": "[abtest] A/B test created (simulated)"}, nil
}

func RegisterTools(tr *tools.ToolRegistry) {
	for _, t := range []tools.Tool{NewTicketTool(), NewKBSearchTool(), NewChatbotTool(), NewCampaignTool(), NewABTestTool()} {
		tr.Register(t)
	}
}
