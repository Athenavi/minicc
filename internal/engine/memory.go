package engine

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ── Memory Types ──────────────────────────────────────────────────────────

// MemoryItem is a single entry in the agent's memory.
type MemoryItem struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"` // thought, observation, tool_result, plan, summary
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// WorkingMemory is the agent's short-term scratchpad for the current task.
// It stores thoughts, intermediate results, and state across ReAct turns.
type WorkingMemory struct {
	mu       sync.RWMutex
	sessionID string
	task     string
	items    []MemoryItem
	state    map[string]interface{} // key-value state store
}

// NewWorkingMemory creates a new working memory for a task session.
func NewWorkingMemory(sessionID, task string) *WorkingMemory {
	return &WorkingMemory{
		sessionID: sessionID,
		task:      task,
		items:     make([]MemoryItem, 0),
		state:     make(map[string]interface{}),
	}
}

// Add stores a new memory item.
func (wm *WorkingMemory) Add(itemType, content string, metadata map[string]interface{}) *MemoryItem {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	item := MemoryItem{
		ID:        fmt.Sprintf("mem_%d", time.Now().UnixNano()),
		Type:      itemType,
		Content:   content,
		Metadata:  metadata,
		CreatedAt: time.Now(),
	}
	wm.items = append(wm.items, item)
	return &item
}

// AddThought records the agent's reasoning step.
func (wm *WorkingMemory) AddThought(thought string) {
	wm.Add("thought", thought, nil)
}

// AddObservation records a tool's output.
func (wm *WorkingMemory) AddObservation(toolName, output string, success bool) {
	wm.Add("observation", output, map[string]interface{}{
		"tool":    toolName,
		"success": success,
	})
}

// GetRecent returns the n most recent memory items.
func (wm *WorkingMemory) GetRecent(n int) []MemoryItem {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	if n <= 0 || n >= len(wm.items) {
		result := make([]MemoryItem, len(wm.items))
		copy(result, wm.items)
		return result
	}
	return wm.items[len(wm.items)-n:]
}

// SetState stores a key-value pair in working state.
func (wm *WorkingMemory) SetState(key string, value interface{}) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.state[key] = value
}

// GetState retrieves a value from working state.
func (wm *WorkingMemory) GetState(key string) interface{} {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.state[key]
}

// Summarize creates a condensed text summary of the working memory for LLM context.
func (wm *WorkingMemory) Summarize() string {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	var parts []string
	parts = append(parts, fmt.Sprintf("Task: %s", wm.task))
	parts = append(parts, fmt.Sprintf("Steps taken: %d", len(wm.items)))

	// Add last 3 items as recent history
	recent := wm.items
	if len(recent) > 3 {
		recent = recent[len(recent)-3:]
	}
	for _, item := range recent {
		content := item.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		parts = append(parts, fmt.Sprintf("[%s] %s", item.Type, content))
	}

	// Include key state
	if len(wm.state) > 0 {
		stateJSON, _ := json.Marshal(wm.state)
		parts = append(parts, fmt.Sprintf("State: %s", string(stateJSON)))
	}

	return strings.Join(parts, "\n")
}

// ── Episodic Memory (cross-session) ───────────────────────────────────────

// Episode records a completed task for future reference.
type Episode struct {
	ID        string    `json:"id"`
	Task      string    `json:"task"`
	Summary   string    `json:"summary"`
	ToolsUsed []string  `json:"tools_used"`
	Success   bool      `json:"success"`
	Duration  time.Duration `json:"duration"`
	CreatedAt time.Time `json:"created_at"`
}

// EpisodicMemory stores past task episodes for cross-session learning.
// In-memory for now; can be backed by PostgreSQL later.
type EpisodicMemory struct {
	mu       sync.RWMutex
	episodes []Episode
	maxSize  int
}

// NewEpisodicMemory creates an episodic memory store.
func NewEpisodicMemory(maxSize int) *EpisodicMemory {
	if maxSize <= 0 {
		maxSize = 100
	}
	return &EpisodicMemory{
		episodes: make([]Episode, 0, maxSize),
		maxSize:  maxSize,
	}
}

// Record saves an episode.
func (em *EpisodicMemory) Record(ep Episode) {
	em.mu.Lock()
	defer em.mu.Unlock()
	if len(em.episodes) >= em.maxSize {
		em.episodes = em.episodes[1:]
	}
	em.episodes = append(em.episodes, ep)
}

// Recent returns the n most recent episodes.
func (em *EpisodicMemory) Recent(n int) []Episode {
	em.mu.RLock()
	defer em.mu.RUnlock()
	if n <= 0 || n >= len(em.episodes) {
		result := make([]Episode, len(em.episodes))
		copy(result, em.episodes)
		return result
	}
	return em.episodes[len(em.episodes)-n:]
}

// FindByTool returns episodes that used a specific tool.
func (em *EpisodicMemory) FindByTool(toolName string) []Episode {
	em.mu.RLock()
	defer em.mu.RUnlock()
	var result []Episode
	for _, ep := range em.episodes {
		for _, t := range ep.ToolsUsed {
			if t == toolName {
				result = append(result, ep)
				break
			}
		}
	}
	return result
}
