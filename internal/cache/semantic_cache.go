package cache

import (
	"context"
	"math"
	"strings"
	"sync"
)

// Embedder converts text into a numeric vector for similarity comparison.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float64, error)
}

// ── Simple Word-Frequency Embedder ────────────────────────────────────────

// WordCountEmbedder converts text into a sparse word-frequency vector.
// Suitable for short to medium-length texts (< 1000 words).
type WordCountEmbedder struct {
	mu         sync.RWMutex
	vocab      map[string]int // word → index
	nextIdx    int
}

// NewWordCountEmbedder creates an embedder that builds vocabulary on-the-fly.
func NewWordCountEmbedder() *WordCountEmbedder {
	return &WordCountEmbedder{
		vocab:   make(map[string]int),
		nextIdx: 0,
	}
}

// Embed converts text into a frequency vector. Words not in the vocabulary
// are added incrementally (growing the vector dimension).
func (e *WordCountEmbedder) Embed(_ context.Context, text string) ([]float64, error) {
	words := tokenize(text)
	if len(words) == 0 {
		return []float64{}, nil
	}

	// Count word frequencies
	localCounts := make(map[string]int)
	for _, w := range words {
		localCounts[w]++
	}

	// Build vector using atomic vocabulary
	e.mu.Lock()
	// Ensure vocabulary is up-to-date
	for w := range localCounts {
		if _, exists := e.vocab[w]; !exists {
			e.vocab[w] = e.nextIdx
			e.nextIdx++
		}
	}
	dim := e.nextIdx
	e.mu.Unlock()

	// Build vector
	e.mu.RLock()
	vec := make([]float64, dim)
	for w, count := range localCounts {
		if idx, ok := e.vocab[w]; ok && idx < dim {
			vec[idx] = float64(count)
		}
	}
	e.mu.RUnlock()

	return vec, nil
}

// ── Cosine Similarity ─────────────────────────────────────────────────────

// CosineSimilarity computes the cosine similarity between two vectors.
// Returns 0 if either vector is zero-length.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// ── Semantic Cache ────────────────────────────────────────────────────────

// SemanticCache caches responses based on semantic similarity of input queries.
// A cached response is returned if the new query's embedding has cosine similarity
// >= Threshold with a previously cached query.
type SemanticCache struct {
	*Cache                        // underlying Redis/disk cache for the response data
	embedder    Embedder          // converts queries to vectors
	threshold   float64           // minimum cosine similarity to consider a hit (default 0.95)
	mu          sync.RWMutex
	entries     []semanticEntry   // in-memory index of (query, key, vector)
	data        map[string][]byte // in-memory fallback store (used when Cache.rdb is nil)
	cachePrefix string            // prefix for cache keys
}

type semanticEntry struct {
	query  string
	key    string
	vector []float64
}

// NewSemanticCache creates a semantic cache with the given embedder and threshold.
// The underlying Cache stores actual response data; the semantic index is in-memory.
func NewSemanticCache(underlying *Cache, embedder Embedder, threshold float64) *SemanticCache {
	if threshold <= 0 {
		threshold = 0.95
	}
	if threshold > 1 {
		threshold = 1
	}
	if embedder == nil {
		embedder = NewWordCountEmbedder()
	}
	return &SemanticCache{
		Cache:       underlying,
		embedder:    embedder,
		threshold:   threshold,
		entries:     make([]semanticEntry, 0),
		data:        make(map[string][]byte),
		cachePrefix: "semantic:",
	}
}

// Get retrieves a semantically similar cached response.
// Returns (nil, false) if no sufficiently similar query is found.
func (sc *SemanticCache) Get(ctx context.Context, query string) ([]byte, bool) {
	if query == "" {
		return nil, false
	}

	vec, err := sc.embedder.Embed(ctx, query)
	if err != nil || len(vec) == 0 {
		return nil, false
	}

	sc.mu.RLock()
	bestSim := 0.0
	bestKey := ""
	for _, entry := range sc.entries {
		sim := CosineSimilarity(vec, entry.vector)
		if sim > bestSim {
			bestSim = sim
			bestKey = entry.key
		}
	}
	sc.mu.RUnlock()

	if bestSim < sc.threshold || bestKey == "" {
		return nil, false
	}

	// Try underlying cache first, then in-memory fallback
	if sc.Cache != nil {
		if data, ok := sc.Cache.Get(ctx, sc.cachePrefix+bestKey); ok {
			return data, true
		}
	}

	sc.mu.RLock()
	data, ok := sc.data[bestKey]
	sc.mu.RUnlock()
	return data, ok
}

// Set stores a response for a query, indexing it for future semantic lookups.
func (sc *SemanticCache) Set(ctx context.Context, query string, data []byte) {
	if query == "" || data == nil {
		return
	}

	vec, err := sc.embedder.Embed(ctx, query)
	if err != nil || len(vec) == 0 {
		return
	}

	key := hashKey(query)

	sc.mu.Lock()
	// Check for duplicate
	for _, entry := range sc.entries {
		if entry.query == query {
			sc.data[key] = data
			sc.mu.Unlock()
			if sc.Cache != nil {
				sc.Cache.Set(ctx, sc.cachePrefix+key, data)
			}
			return
		}
	}
	// Add new entry
	sc.entries = append(sc.entries, semanticEntry{
		query:  query,
		key:    key,
		vector: vec,
	})

	// Store data: underlying cache first, then in-memory fallback
	if sc.Cache != nil {
		sc.Cache.Set(ctx, sc.cachePrefix+key, data)
	}
	sc.data[key] = data
	sc.mu.Unlock()
}

// Size returns the number of cached entries in the semantic index.
func (sc *SemanticCache) Size() int {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return len(sc.entries)
}

// ── Helpers ───────────────────────────────────────────────────────────────

func tokenize(text string) []string {
	text = strings.ToLower(text)
	// Replace common punctuation with spaces
	replacer := strings.NewReplacer(
		".", " ", ",", " ", "!", " ", "?", " ", ";", " ", ":", " ",
		"(", " ", ")", " ", "[", " ", "]", " ", "{", " ", "}", " ",
		"\"", " ", "'", " ", "\n", " ", "\t", " ", "\r", " ",
		"/", " ", "\\", " ", "-", " ", "_", " ",
	)
	text = replacer.Replace(text)

	words := strings.Fields(text)
	// Filter short words and deduplicate within this call
	seen := make(map[string]bool)
	var result []string
	for _, w := range words {
		if len(w) < 2 {
			continue
		}
		// Normalize digits
		normalized := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return '0'
			}
			return r
		}, w)
		if !seen[normalized] {
			seen[normalized] = true
			result = append(result, normalized)
		}
	}
	return result
}

func hashKey(s string) string {
	// Simple deterministic hash for cache keys
	var h uint64 = 14695981039346656037
	for _, c := range s {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return strings.TrimRight(strings.ReplaceAll(
		strings.ReplaceAll(
			strings.Map(func(r rune) rune {
				if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
					return r
				}
				return -1
			}, s),
			" ", "_"),
		"__", "_"), "_") + "_" + formatHash(h)
}

func formatHash(h uint64) string {
	const hexChars = "0123456789abcdef"
	buf := make([]byte, 16)
	for i := 15; i >= 0; i-- {
		buf[i] = hexChars[h&0xf]
		h >>= 4
	}
	return string(buf)
}
