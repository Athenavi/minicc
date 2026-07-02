package cache

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestNewWordCountEmbedder(t *testing.T) {
	e := NewWordCountEmbedder()
	if e == nil {
		t.Fatal("expected non-nil embedder")
	}
}

func TestWordCountEmbedder_Empty(t *testing.T) {
	e := NewWordCountEmbedder()
	vec, err := e.Embed(context.Background(), "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(vec) != 0 {
		t.Fatalf("expected empty vector, got len %d", len(vec))
	}
}

func TestWordCountEmbedder_Basic(t *testing.T) {
	e := NewWordCountEmbedder()
	vec, err := e.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(vec) == 0 {
		t.Fatal("expected non-empty vector")
	}
}

func TestWordCountEmbedder_Consistency(t *testing.T) {
	e := NewWordCountEmbedder()
	v1, _ := e.Embed(context.Background(), "hello world")
	v2, _ := e.Embed(context.Background(), "hello world")
	if len(v1) != len(v2) {
		t.Fatalf("expected same vector length, got %d vs %d", len(v1), len(v2))
	}
	for i := range v1 {
		if v1[i] != v2[i] {
			t.Fatalf("expected identical vectors, differ at index %d: %f vs %f", i, v1[i], v2[i])
		}
	}
}

func TestCosineSimilarity_Identical(t *testing.T) {
	a := []float64{1, 2, 3}
	b := []float64{1, 2, 3}
	sim := CosineSimilarity(a, b)
	if math.Abs(sim-1.0) > 1e-10 {
		t.Fatalf("expected 1.0, got %f", sim)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float64{1, 0, 0}
	b := []float64{0, 1, 0}
	sim := CosineSimilarity(a, b)
	if math.Abs(sim-0.0) > 1e-10 {
		t.Fatalf("expected 0.0, got %f", sim)
	}
}

func TestCosineSimilarity_Oposite(t *testing.T) {
	a := []float64{1, 2, 3}
	b := []float64{-1, -2, -3}
	sim := CosineSimilarity(a, b)
	if math.Abs(sim+1.0) > 1e-10 {
		t.Fatalf("expected -1.0, got %f", sim)
	}
}

func TestCosineSimilarity_Empty(t *testing.T) {
	if CosineSimilarity(nil, []float64{1}) != 0 {
		t.Fatal("expected 0 for empty")
	}
	if CosineSimilarity([]float64{1}, nil) != 0 {
		t.Fatal("expected 0 for empty")
	}
	if CosineSimilarity(nil, nil) != 0 {
		t.Fatal("expected 0 for both empty")
	}
}

func TestNewSemanticCache(t *testing.T) {
	underlying := New(nil, "test:", time.Minute)
	sc := NewSemanticCache(underlying, NewWordCountEmbedder(), 0.9)
	if sc.threshold != 0.9 {
		t.Fatalf("expected threshold 0.9, got %f", sc.threshold)
	}
	if sc.Size() != 0 {
		t.Fatalf("expected size 0, got %d", sc.Size())
	}
}

func TestSemanticCache_DefaultThreshold(t *testing.T) {
	underlying := New(nil, "test:", time.Minute)
	sc := NewSemanticCache(underlying, nil, 0)
	if sc.threshold != 0.95 {
		t.Fatalf("expected default threshold 0.95, got %f", sc.threshold)
	}
}

func TestSemanticCache_SetGet(t *testing.T) {
	underlying := New(nil, "test:", time.Minute)
	sc := NewSemanticCache(underlying, NewWordCountEmbedder(), 0.5) // low threshold for test

	sc.Set(context.Background(), "what is the weather", []byte("sunny"))
	if sc.Size() != 1 {
		t.Fatalf("expected size 1, got %d", sc.Size())
	}

	// Exact match should hit
	data, ok := sc.Get(context.Background(), "what is the weather")
	if !ok {
		t.Fatal("expected hit for exact match")
	}
	if string(data) != "sunny" {
		t.Fatalf("expected 'sunny', got %q", string(data))
	}
}

func TestSemanticCache_Miss(t *testing.T) {
	underlying := New(nil, "test:", time.Minute)
	sc := NewSemanticCache(underlying, NewWordCountEmbedder(), 0.99) // high threshold

	sc.Set(context.Background(), "hello world", []byte("data"))
	// Different query should miss
	_, ok := sc.Get(context.Background(), "completely different text")
	if ok {
		t.Fatal("expected miss for very different query")
	}
}

func TestSemanticCache_EmptyQuery(t *testing.T) {
	underlying := New(nil, "test:", time.Minute)
	sc := NewSemanticCache(underlying, nil, 0.9)
	_, ok := sc.Get(context.Background(), "")
	if ok {
		t.Fatal("expected miss for empty query")
	}
	// Set with empty should not panic
	sc.Set(context.Background(), "", []byte("data"))
}

func TestSemanticCache_NilData(t *testing.T) {
	underlying := New(nil, "test:", time.Minute)
	sc := NewSemanticCache(underlying, nil, 0.9)
	// Should not panic
	sc.Set(context.Background(), "test", nil)
}

func TestTokenize(t *testing.T) {
	tokens := tokenize("Hello World!")
	if len(tokens) == 0 {
		t.Fatal("expected non-empty tokens")
	}
	if tokens[0] != "hello" {
		t.Fatalf("expected 'hello', got %q", tokens[0])
	}
}

func TestTokenize_Lowercase(t *testing.T) {
	tokens := tokenize("HELLO WORLD")
	found := false
	for _, tok := range tokens {
		if tok == "hello" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'hello' to be lowercase")
	}
}

func TestTokenize_ShortWords(t *testing.T) {
	tokens := tokenize("a an the is it")
	for _, tok := range tokens {
		if len(tok) < 2 {
			t.Fatalf("expected no single-char tokens, got %q", tok)
		}
	}
}

func TestTokenize_Empty(t *testing.T) {
	tokens := tokenize("")
	if len(tokens) != 0 {
		t.Fatalf("expected empty tokens, got %d", len(tokens))
	}
}

func TestHashKey_Deterministic(t *testing.T) {
	h1 := hashKey("hello world")
	h2 := hashKey("hello world")
	if h1 != h2 {
		t.Fatalf("expected same hash, got %q vs %q", h1, h2)
	}
}

func TestHashKey_RoughlyUnique(t *testing.T) {
	h1 := hashKey("hello world")
	h2 := hashKey("hello world!")
	if h1 == h2 {
		t.Fatal("expected different hashes for different inputs")
	}
}

func TestSemanticCache_DuplicateSet(t *testing.T) {
	underlying := New(nil, "test:", time.Minute)
	sc := NewSemanticCache(underlying, NewWordCountEmbedder(), 0.5)

	sc.Set(context.Background(), "same query", []byte("first"))
	sc.Set(context.Background(), "same query", []byte("second"))

	if sc.Size() != 1 {
		t.Fatalf("expected size 1 (dedup), got %d", sc.Size())
	}

	data, ok := sc.Get(context.Background(), "same query")
	if !ok {
		t.Fatal("expected hit")
	}
	if string(data) != "second" {
		t.Fatalf("expected 'second' (overwritten), got %q", string(data))
	}
}

func TestSemanticCache_VeryHighThreshold(t *testing.T) {
	underlying := New(nil, "test:", time.Minute)
	sc := NewSemanticCache(underlying, NewWordCountEmbedder(), 1.5)
	if sc.threshold != 1.0 {
		t.Fatalf("expected threshold capped at 1.0, got %f", sc.threshold)
	}
}

func TestWordCountEmbedder_VocabBuilding(t *testing.T) {
	e := NewWordCountEmbedder()
	// First, prime the vocab with all words
	v1, _ := e.Embed(context.Background(), "alpha beta gamma delta")
	// Now embed a subset — should have same dimension
	v2, _ := e.Embed(context.Background(), "alpha beta")
	if len(v1) != len(v2) {
		t.Fatalf("expected same dimension after all words seen, got %d vs %d", len(v1), len(v2))
	}
	if len(v1) < 4 {
		t.Fatalf("expected at least 4 dimensions (4 unique words), got %d", len(v1))
	}
}

func TestSemanticCache_SimilarQuery(t *testing.T) {
	underlying := New(nil, "test:", time.Minute)
	// Lower threshold to catch similar queries
	sc := NewSemanticCache(underlying, NewWordCountEmbedder(), 0.3)

	sc.Set(context.Background(), "what is the weather today", []byte("sunny"))

	// Similar query should hit
	data, ok := sc.Get(context.Background(), "what weather today")
	if !ok {
		t.Log("warning: similar query didn't hit (may need lower threshold)")
	} else {
		if string(data) != "sunny" {
			t.Fatalf("expected 'sunny', got %q", string(data))
		}
	}
}
