package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/db"
)

// MediaCreateTool saves generated content (text, code, CSV, etc.) to the media library.
// This allows the AI to persist generated files and reference them later.
type MediaCreateTool struct{}

func NewMediaCreateTool() *MediaCreateTool { return &MediaCreateTool{} }

func (t *MediaCreateTool) Name() string        { return "media_create" }
func (t *MediaCreateTool) Description() string  { return "Save generated content (CSV, code, text, markdown, etc.) to the media library for later reference." }
func (t *MediaCreateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"name": map[string]interface{}{
			"type":        "string",
			"description": "Name for the media asset (e.g. report.csv, output.txt)",
		},
		"content": map[string]interface{}{
			"type":        "string",
			"description": "Content to save",
		},
		"type": map[string]interface{}{
			"type":        "string",
			"description": "Asset type: text, csv, code, image, document, etc.",
		},
		"category": map[string]interface{}{
			"type":        "string",
			"description": "Category for organization (default: generated)",
		},
		"tags": map[string]interface{}{
			"type":        "array",
			"description": "Tags for the media asset",
			"items":       map[string]interface{}{"type": "string"},
		},
	}
}

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

// ── Image Generation Tool ─────────────────────────────────────────────────

// ImageGenerateTool generates a placeholder SVG image and saves it to the media library.
// Uses SVG generation — no external API required.
type ImageGenerateTool struct{}

func NewImageGenerateTool() *ImageGenerateTool { return &ImageGenerateTool{} }

func (t *ImageGenerateTool) Name() string        { return "image_generate" }
func (t *ImageGenerateTool) Description() string  { return "Generate an SVG image from a text description and save it to the media library." }
func (t *ImageGenerateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"prompt": map[string]interface{}{
			"type":        "string",
			"description": "Description of the image to generate (subject, style, colors)",
		},
		"width": map[string]interface{}{
			"type":        "number",
			"description": "Image width in pixels (default: 800)",
		},
		"height": map[string]interface{}{
			"type":        "number",
			"description": "Image height in pixels (default: 600)",
		},
		"category": map[string]interface{}{
			"type":        "string",
			"description": "Category tag for organization (default: generated)",
		},
	}
}

func (t *ImageGenerateTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	prompt, _ := input["prompt"].(string)
	if prompt == "" {
		prompt = "Generated Image"
	}

	width := 800
	if w, ok := input["width"].(float64); ok && w > 0 {
		width = int(w)
	}
	height := 600
	if h, ok := input["height"].(float64); ok && h > 0 {
		height = int(h)
	}

	// Generate a descriptive SVG image based on the prompt
	svg := generateSVG(prompt, width, height)

	name := sanitizeFilename(prompt) + ".svg"
	category, _ := input["category"].(string)
	if category == "" {
		category = "generated"
	}

	// Save to media library via DB
	if db.Pool == nil {
		return nil, fmt.Errorf("database not available — media library requires PostgreSQL")
	}

	mediaType := "image"
	id := fmt.Sprintf("media_%d", time.Now().UnixNano())
	tagsJSON := fmt.Sprintf(`{"%s"}`, category)
	metadata := fmt.Sprintf(`{"prompt":"%s","width":%d,"height":%d,"format":"svg"}`, prompt, width, height)

	_, err := db.Pool.Exec(ctx,
		`INSERT INTO media_assets (id, type, name, content, category, tags, metadata, size, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, NOW(), NOW())`,
		id, mediaType, name, svg, category, tagsJSON, metadata, int64(len(svg)))
	if err != nil {
		return nil, fmt.Errorf("save image to media library: %w", err)
	}

	slog.Info("image generated and saved", "id", id, "name", name, "width", width, "height", height)

	return map[string]interface{}{
		"output":   fmt.Sprintf("Image generated and saved to media library: %s (%dx%d, SVG)\n  ID: %s\n  You can view it by fetching media asset %s", name, width, height, id, id),
		"id":       id,
		"name":     name,
		"type":     "image",
		"format":   "svg",
		"width":    width,
		"height":   height,
		"category": category,
		"content":  svg,
		"size":     len(svg),
	}, nil
}

// generateSVG creates a descriptive SVG based on the prompt text.
func generateSVG(prompt string, width, height int) string {
	// Extract keywords for visual customization
	promptLower := strings.ToLower(prompt)
	bgColor := "#1a1a2e"
	accentColor := "#e94560"
	textColor := "#ffffff"

	if strings.Contains(promptLower, "sunset") || strings.Contains(promptLower, "sun") || strings.Contains(promptLower, "warm") {
		bgColor = "#2d1b00"
		accentColor = "#ff6b35"
	} else if strings.Contains(promptLower, "ocean") || strings.Contains(promptLower, "lake") || strings.Contains(promptLower, "water") || strings.Contains(promptLower, "blue") {
		bgColor = "#0a1628"
		accentColor = "#4fc3f7"
	} else if strings.Contains(promptLower, "forest") || strings.Contains(promptLower, "nature") || strings.Contains(promptLower, "green") {
		bgColor = "#0d2818"
		accentColor = "#66bb6a"
	} else if strings.Contains(promptLower, "city") || strings.Contains(promptLower, "urban") || strings.Contains(promptLower, "night") {
		bgColor = "#0d0d0d"
		accentColor = "#ffd54f"
	} else if strings.Contains(promptLower, "space") || strings.Contains(promptLower, "galaxy") || strings.Contains(promptLower, "star") {
		bgColor = "#000011"
		accentColor = "#7c4dff"
	}

	// Sanitize prompt for display
	displayPrompt := prompt
	if len(displayPrompt) > 80 {
		displayPrompt = displayPrompt[:80] + "..."
	}

	// Calculate integer positions for circles
	cx1, cy1, r1 := int(float64(width)*0.7), int(float64(height)*0.3), int(float64(width)*0.15)
	cx2, cy2, r2 := int(float64(width)*0.2), int(float64(height)*0.7), int(float64(width)*0.12)
	cx3, cy3, r3 := int(float64(width)*0.5), int(float64(height)*0.8), int(float64(width)*0.2)
	tx1, ty1 := int(float64(width)*0.5), int(float64(height)*0.45)
	tx2, ty2 := int(float64(width)*0.5), int(float64(height)*0.5)

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">
  <defs>
    <linearGradient id="bg" x1="0%%" y1="0%%" x2="100%%" y2="100%%">
      <stop offset="0%%" style="stop-color:%s;stop-opacity:1" />
      <stop offset="100%%" style="stop-color:%s;stop-opacity:0.6" />
    </linearGradient>
    <linearGradient id="accent" x1="0%%" y1="0%%" x2="100%%" y2="0%%">
      <stop offset="0%%" style="stop-color:%s;stop-opacity:0.3" />
      <stop offset="50%%" style="stop-color:%s;stop-opacity:0.8" />
      <stop offset="100%%" style="stop-color:%s;stop-opacity:0.3" />
    </linearGradient>
  </defs>
  <rect width="%d" height="%d" fill="url(#bg)" />
  <circle cx="%d" cy="%d" r="%d" fill="url(#accent)" opacity="0.4" />
  <circle cx="%d" cy="%d" r="%d" fill="url(#accent)" opacity="0.3" />
  <circle cx="%d" cy="%d" r="%d" fill="url(#accent)" opacity="0.2" />
  <text x="%d" y="%d" font-family="Arial, sans-serif" font-size="24" fill="%s" text-anchor="middle" opacity="0.9">%s</text>
  <text x="%d" y="%d" font-family="Arial, sans-serif" font-size="14" fill="%s" text-anchor="middle" opacity="0.6">Generated by MiniCC</text>
</svg>`,
		width, height, width, height,
		bgColor, accentColor,
		accentColor, accentColor, accentColor,
		width, height,
		cx1, cy1, r1,
		cx2, cy2, r2,
		cx3, cy3, r3,
		tx1, ty1, textColor, escapeXML(displayPrompt),
		tx2, ty2, textColor,
	)
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

func sanitizeFilename(s string) string {
	var result []rune
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			result = append(result, r)
		} else if r == ' ' {
			result = append(result, '_')
		}
	}
	if len(result) == 0 {
		return "image"
	}
	// Limit length
	if len(result) > 60 {
		result = result[:60]
	}
	return string(result)
}
