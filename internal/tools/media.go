package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/athenavi/minicc/internal/db"
)

// MediaCreateTool saves generated content (text, code, CSV, etc.) to the media library.
// This allows the AI to persist generated files and reference them later.
type MediaCreateTool struct{}

func NewMediaCreateTool() *MediaCreateTool { return &MediaCreateTool{} }

func (t *MediaCreateTool) Name() string        { return "media_create" }
func (t *MediaCreateTool) Description() string  { return "Save generated content (CSV, code, text, markdown, etc.) to the media library for later reference." }

func (t *MediaCreateTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	if db.Pool == nil {
		return nil, fmt.Errorf("database not available — media library requires PostgreSQL")
	}

	name, _ := input["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	content, _ := input["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}

	mediaType, _ := input["type"].(string)
	if mediaType == "" {
		mediaType = "text"
	}

	category, _ := input["category"].(string)
	if category == "" {
		category = "generated"
	}

	tagsRaw, _ := input["tags"].([]interface{})
	tags := make([]string, 0, len(tagsRaw))
	for _, t := range tagsRaw {
		if s, ok := t.(string); ok {
			tags = append(tags, s)
		}
	}

	metadata := map[string]interface{}{}
	if metaRaw, ok := input["metadata"].(map[string]interface{}); ok {
		metadata = metaRaw
	}
	metadataJSON, _ := json.Marshal(metadata)

	tagsJSON := "{}"
	if len(tags) > 0 {
		// PostgreSQL text array format
		escaped := make([]string, len(tags))
		for i, tag := range tags {
			escaped[i] = fmt.Sprintf(`"%s"`, tag)
		}
		tagsJSON = "{" + joinStrings(escaped, ",") + "}"
	}

	id := fmt.Sprintf("media_%d", time.Now().UnixNano())
	size := int64(len(content))

	_, err := db.Pool.Exec(ctx,
		`INSERT INTO media_assets (id, type, name, content, category, tags, metadata, size, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, NOW(), NOW())`,
		id, mediaType, name, content, category, tagsJSON, string(metadataJSON), size)
	if err != nil {
		return nil, fmt.Errorf("create media asset: %w", err)
	}

	slog.Info("media asset created", "id", id, "name", name, "type", mediaType, "category", category)

	return map[string]interface{}{
		"output":   fmt.Sprintf("Media asset created: %s (type: %s, category: %s, id: %s)", name, mediaType, category, id),
		"id":       id,
		"name":     name,
		"type":     mediaType,
		"category": category,
		"size":     size,
	}, nil
}

func joinStrings(elems []string, sep string) string {
	if len(elems) == 0 {
		return ""
	}
	result := elems[0]
	for _, e := range elems[1:] {
		result += sep + e
	}
	return result
}
