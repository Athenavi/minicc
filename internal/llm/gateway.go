package llm

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Gateway is the unified LLM entry point with routing, failover, and caching.
type Gateway struct {
	mu        sync.RWMutex
	providers []Provider
	cache     *ResponseCache
	breakers  map[string]*CircuitBreaker
	metrics   *GatewayMetrics
}

type GatewayMetrics struct {
	mu        sync.Mutex
	requests  int64
	cacheHits int64
	errors    int64
	failovers int64
	byProvider map[string]*ProviderMetrics
}

type ProviderMetrics struct {
	requests int64
	errors   int64
	latency  time.Duration
}

func NewGateway(cacheTTL time.Duration) *Gateway {
	return &Gateway{
		cache:    NewResponseCache(cacheTTL),
		breakers: make(map[string]*CircuitBreaker),
		metrics: &GatewayMetrics{
			byProvider: make(map[string]*ProviderMetrics),
		},
	}
}

func (g *Gateway) AddProvider(p Provider) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.providers = append(g.providers, p)
	g.breakers[p.Name()] = NewCircuitBreaker(5, 30*time.Second)
	g.metrics.byProvider[p.Name()] = &ProviderMetrics{}
	slog.Info("llm provider added", "name", p.Name(), "available", p.IsAvailable())
}

func (g *Gateway) Chat(ctx context.Context, req *Request) (*Response, error) {
	g.metrics.mu.Lock()
	g.metrics.requests++
	g.metrics.mu.Unlock()

	// 1. Check cache
	if cached := g.cache.Get(req); cached != nil {
		g.metrics.mu.Lock()
		g.metrics.cacheHits++
		g.metrics.mu.Unlock()
		return cached, nil
	}

	g.mu.RLock()
	providers := g.providers
	g.mu.RUnlock()

	if len(providers) == 0 {
		return nil, ErrNoProvider
	}

	// 2. Try providers in order
	var lastErr error
	for i, p := range providers {
		if !p.IsAvailable() {
			continue
		}

		cb := g.breakers[p.Name()]
		if !cb.Allow(p.Name()) {
			slog.Debug("circuit open, skipping", "provider", p.Name())
			continue
		}

		resp, err := p.Chat(ctx, req)
		g.recordMetrics(p.Name(), err)

		if err == nil {
			cb.Success(p.Name())
			g.cache.Set(req, resp)
			return resp, nil
		}

		cb.Fail(p.Name())
		lastErr = err
		slog.Warn("llm provider failed",
			"provider", p.Name(),
			"error", err,
			"remaining", len(providers)-i-1,
		)

		if i < len(providers)-1 {
			g.metrics.mu.Lock()
			g.metrics.failovers++
			g.metrics.mu.Unlock()
		}
	}

	return nil, lastErr
}

func (g *Gateway) recordMetrics(name string, err error) {
	g.metrics.mu.Lock()
	defer g.metrics.mu.Unlock()

	pm := g.metrics.byProvider[name]
	if pm == nil {
		return
	}
	pm.requests++
	if err != nil {
		pm.errors++
		g.metrics.errors++
	}
}

func (g *Gateway) CacheStats() map[string]interface{} {
	return g.cache.Stats()
}

func (g *Gateway) Metrics() map[string]interface{} {
	g.metrics.mu.Lock()
	defer g.metrics.mu.Unlock()

	byProvider := make(map[string]interface{})
	for name, pm := range g.metrics.byProvider {
		byProvider[name] = map[string]interface{}{
			"requests": pm.requests,
			"errors":   pm.errors,
		}
	}

	return map[string]interface{}{
		"total_requests":  g.metrics.requests,
		"cache_hits":      g.metrics.cacheHits,
		"errors":          g.metrics.errors,
		"failovers":       g.metrics.failovers,
		"providers":       len(g.providers),
		"by_provider":     byProvider,
	}
}
