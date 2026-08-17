package routing

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// ModelConfig represents a model configuration with routing metadata.
type ModelConfig struct {
	ID                string    `json:"id"`
	ModelID           string    `json:"model_id"`
	DisplayName       string    `json:"display_name"`
	Provider          string    `json:"provider"`
	Priority          int       `json:"priority"`
	Weight            int       `json:"weight"`             // Load balancing weight (1-100)
	FallbackChain     []string  `json:"fallback_chain"`     // Fallback models
	MaxRPM            int       `json:"max_rpm"`
	MaxTPM            int       `json:"max_tpm"`
	ConcurrentLimit   int       `json:"concurrent_limit"`
	Status            string    `json:"status"`
	InputCostPer1M    float64   `json:"input_cost_per_1m"`
	OutputCostPer1M   float64   `json:"output_cost_per_1m"`
}

// Request represents an LLM request.
type Request struct {
	UserID      string
	TenantID    string
	Message     string
	MaxTokens   int
	Temperature float64
}

// ModelRouter is the interface for selecting models.
type ModelRouter interface {
	SelectModel(ctx context.Context, req *Request) (*ModelConfig, error)
	Name() string
}

// WeightedRoundRobin selects models based on weighted round robin algorithm.
type WeightedRoundRobin struct {
	mu       sync.RWMutex
	models   []*ModelConfig
	totalWeight int
}

// NewWeightedRoundRobin creates a new weighted round robin router.
func NewWeightedRoundRobin(models []*ModelConfig) *WeightedRoundRobin {
	var totalWeight int
	activeModels := make([]*ModelConfig, 0)
	
	for _, m := range models {
		if m.Status == "active" {
			activeModels = append(activeModels, m)
			totalWeight += m.Weight
		}
	}
	
	return &WeightedRoundRobin{
		models:      activeModels,
		totalWeight: totalWeight,
	}
}

// SelectModel selects a model using weighted round robin.
func (w *WeightedRoundRobin) SelectModel(ctx context.Context, req *Request) (*ModelConfig, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	
	if len(w.models) == 0 {
		return nil, fmt.Errorf("no active models available")
	}
	
	rand := rand.Intn(w.totalWeight)
	cumulative := 0
	
	for _, model := range w.models {
		cumulative += model.Weight
		if rand < cumulative {
			return model, nil
		}
	}
	
	// Fallback to first model
	return w.models[0], nil
}

// Name returns the name of the router.
func (w *WeightedRoundRobin) Name() string {
	return "weighted_round_robin"
}

// FailoverRouter selects models with fallback chain.
type FailoverRouter struct {
	primary  *ModelConfig
	fallback []*ModelConfig
}

// NewFailoverRouter creates a new failover router.
func NewFailoverRouter(models []*ModelConfig) *FailoverRouter {
	var primary *ModelConfig
	var fallbacks []*ModelConfig
	
	// Find highest priority model as primary
	for _, m := range models {
		if m.Status != "active" {
			continue
		}
		if primary == nil || m.Priority > primary.Priority {
			primary = m
		}
	}
	
	// Rest become fallback
	if primary != nil {
		for _, m := range models {
			if m.Status == "active" && m.ID != primary.ID {
				fallbacks = append(fallbacks, m)
			}
		}
	}
	
	return &FailoverRouter{
		primary:  primary,
		fallback: fallbacks,
	}
}

// SelectModel tries primary first, then falls back.
func (f *FailoverRouter) SelectModel(ctx context.Context, req *Request) (*ModelConfig, error) {
	// Try primary
	if f.primary != nil && f.primary.Status == "active" {
		return f.primary, nil
	}
	
	// Try fallbacks
	for _, fb := range f.fallback {
		if fb.Status == "active" {
			return fb, nil
		}
	}
	
	return nil, fmt.Errorf("all models unavailable")
}

// Name returns the name of the router.
func (f *FailoverRouter) Name() string {
	return "failover"
}

// ABTestRouter distributes traffic for A/B testing.
type ABTestRouter struct {
	variants []ABVariant
}

// ABVariant represents a variant in A/B test.
type ABVariant struct {
	Model   *ModelConfig
	Traffic float64 // Traffic percentage (0.0-1.0)
	TestID  string
}

// NewABTestRouter creates a new A/B test router.
func NewABTestRouter(variants []ABVariant) *ABTestRouter {
	return &ABTestRouter{variants: variants}
}

// SelectModel selects a model based on traffic ratio.
func (a *ABTestRouter) SelectModel(ctx context.Context, req *Request) (*ModelConfig, error) {
	if len(a.variants) == 0 {
		return nil, fmt.Errorf("no A/B test variants configured")
	}
	
	rand := rand.Float64()
	cumulative := 0.0
	
	for _, variant := range a.variants {
		cumulative += variant.Traffic
		if rand < cumulative {
			return variant.Model, nil
		}
	}
	
	// Fallback to first variant
	return a.variants[0].Model, nil
}

// Name returns the name of the router.
func (a *ABTestRouter) Name() string {
	return "ab_test"
}

// ModelRegistry manages all model configurations.
type ModelRegistry struct {
	mu       sync.RWMutex
	models   map[string]*ModelConfig // model_id -> ModelConfig
}

// NewModelRegistry creates a new model registry.
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models: make(map[string]*ModelConfig),
	}
}

// Register adds a model to the registry.
func (r *ModelRegistry) Register(model *ModelConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.models[model.ModelID] = model
}

// GetAllActive returns all active models.
func (r *ModelRegistry) GetAllActive() []*ModelConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	active := make([]*ModelConfig, 0)
	for _, m := range r.models {
		if m.Status == "active" {
			active = append(active, m)
		}
	}
	
	return active
}

// Get returns a model by ID.
func (r *ModelRegistry) Get(modelID string) (*ModelConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	model, ok := r.models[modelID]
	return model, ok
}
