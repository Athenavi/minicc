package skill

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Generator creates skill definitions from natural language descriptions.
// It uses templates and heuristics rather than an LLM call, so it works
// without external dependencies. An LLM-powered version can be added later.
type Generator struct {
	store *SkillStore
}

func NewGenerator(store *SkillStore) *Generator {
	return &Generator{store: store}
}

// GenerateFromDesc creates a SkillDef based on a natural language description.
// The generator parses the description to extract:
//   - skill name
//   - exec type (python/shell/http/prompt)
//   - parameters
//   - source code/template
//
// Returns the generated SkillDef (not yet saved to store).
func (g *Generator) GenerateFromDesc(description string) (*SkillDef, error) {
	if description == "" {
		return nil, fmt.Errorf("description is required")
	}

	desc := strings.ToLower(description)

	// Detect exec type from description keywords
	execType := detectExecType(desc)

	// Extract a name from the description
	name := generateName(description)

	// Build parameters list from description
	params := extractParameters(desc)

	// Generate exec source template
	source := generateSource(name, execType, desc)

	skill := &SkillDef{
		Name:        name,
		Description: description,
		Version:     "1.0.0",
		Author:      "ai-generated",
		Tags:        generateTags(desc),
		Exec: ExecConfig{
			Type:   execType,
			Source: source,
		},
		Parameters:  params,
		Source:      "generated",
		InstalledAt: time.Now(),
	}

	return skill, nil
}

// detectExecType infers the execution type from description keywords.
func detectExecType(desc string) ExecType {
	// Count keyword matches for each type
	pythonScore := countKeywords(desc, []string{"python", "data", "csv", "json", "analyze", "transform", "convert", "parse", "extract"})
	shellScore := countKeywords(desc, []string{"shell", "command", "file", "directory", "system", "git", "docker", "process"})
	httpScore := countKeywords(desc, []string{"http", "api", "webhook", "fetch", "rest", "get", "post", "request", "url"})

	if pythonScore >= shellScore && pythonScore >= httpScore {
		return ExecPython
	}
	if shellScore >= httpScore {
		return ExecShell
	}
	return ExecHTTP
}

// generateName creates a snake_case name from the description.
func generateName(desc string) string {
	// Take first meaningful words
	words := strings.Fields(desc)
	var nameWords []string
	for _, w := range words {
		w = strings.Trim(w, ".,!?;:")
		if len(w) > 2 && !isStopWord(w) {
			nameWords = append(nameWords, w)
		}
		if len(nameWords) >= 4 {
			break
		}
	}
	if len(nameWords) == 0 {
		return "custom_skill"
	}
	return strings.Join(nameWords, "_")
}

// extractParameters tries to extract parameter hints from the description.
func extractParameters(desc string) []Parameter {
	var params []Parameter

	// Look for "with <param>" or "using <param>" patterns
	words := strings.Fields(desc)
	for i, w := range words {
		lower := strings.ToLower(w)
		if (lower == "with" || lower == "using" || lower == "from") && i+1 < len(words) {
			paramName := strings.Trim(words[i+1], ".,!?;:")
			if len(paramName) > 2 {
				params = append(params, Parameter{
					Name:        strings.ToLower(paramName),
					Type:        "string",
					Description: fmt.Sprintf("The %s to process", paramName),
					Required:    true,
				})
			}
		}
	}

	// If no params found, add a generic "input" parameter
	if len(params) == 0 {
		params = append(params, Parameter{
			Name:        "input",
			Type:        "string",
			Description: "Input data to process",
			Required:    true,
		})
	}

	return params
}

// generateSource creates a stub source code template.
func generateSource(name string, execType ExecType, desc string) string {
	switch execType {
	case ExecPython:
		return fmt.Sprintf(`# %s
# Auto-generated skill
def run(input_data):
    # TODO: implement based on: %s
    result = {"message": "Skill executed", "input": input_data}
    return result

output = run(input_data)
print(__import__('json').dumps(output))
`, name, truncate(desc, 60))
	case ExecShell:
		return fmt.Sprintf(`echo "Running: %s"`, name)
	case ExecHTTP:
		return fmt.Sprintf(`https://api.example.com/%s`, name)
	case ExecPrompt:
		return fmt.Sprintf(`You are a %s assistant. Process the user's request based on: %s`, name, truncate(desc, 60))
	default:
		return "# TODO: implement"
	}
}

// generateTags creates tags from description keywords.
func generateTags(desc string) []string {
	var tags []string
	seen := make(map[string]bool)
	words := strings.Fields(strings.ToLower(desc))
	for _, w := range words {
		w = strings.Trim(w, ".,!?;:")
		if len(w) > 3 && !isStopWord(w) && !seen[w] {
			tags = append(tags, w)
			seen[w] = true
		}
		if len(tags) >= 5 {
			break
		}
	}
	return tags
}

// ToJSONSkill converts a SkillDef to a formatted JSON string (for display/export).
func ToJSONSkill(skill *SkillDef) string {
	data, _ := json.MarshalIndent(skill, "", "  ")
	return string(data)
}

// ── Helpers ───────────────────────────────────────────────────────────────

func countKeywords(s string, keywords []string) int {
	count := 0
	lower := strings.ToLower(s)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			count++
		}
	}
	return count
}

var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "can": true, "shall": true, "to": true,
	"of": true, "in": true, "for": true, "on": true, "with": true,
	"at": true, "by": true, "from": true, "as": true, "into": true,
	"through": true, "during": true, "before": true, "after": true,
	"above": true, "below": true, "between": true, "and": true, "but": true,
	"or": true, "nor": true, "not": true, "so": true, "yet": true,
	"this": true, "that": true, "these": true, "those": true, "it": true,
	"its": true, "them": true, "they": true, "we": true, "you": true,
	"i": true, "me": true, "my": true, "your": true, "our": true,
}

func isStopWord(w string) bool {
	return stopWords[strings.ToLower(w)]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
