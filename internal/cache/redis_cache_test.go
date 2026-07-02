package cache

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestCache_New(t *testing.T) {
	c := New(nil, "test:", 5*time.Minute)
	if c.rdb != nil {
		t.Fatal("expected nil rdb")
	}
	if c.prefix != "test:" {
		t.Fatalf("expected prefix 'test:', got %q", c.prefix)
	}
}

func TestCache_Key(t *testing.T) {
	c := New(nil, "prefix:", 0)
	got := c.key("mykey")
	if got != "prefix:mykey" {
		t.Fatalf("expected 'prefix:mykey', got %q", got)
	}
}

func TestCache_GetNil(t *testing.T) {
	c := New(nil, "test:", time.Minute)
	data, ok := c.Get(context.Background(), "key")
	if ok {
		t.Fatal("expected miss for nil redis")
	}
	if data != nil {
		t.Fatal("expected nil data")
	}
}

func TestCache_SetNil(t *testing.T) {
	c := New(nil, "test:", time.Minute)
	// Should not panic
	c.Set(context.Background(), "key", []byte("value"))
}

func TestCache_DeleteNil(t *testing.T) {
	c := New(nil, "test:", time.Minute)
	// Should not panic
	c.Delete(context.Background(), "key")
}

func TestCache_ExistsNil(t *testing.T) {
	c := New(nil, "test:", time.Minute)
	if c.Exists(context.Background(), "key") {
		t.Fatal("expected false for nil redis")
	}
}

func TestCache_GetOrSet_CallsFn(t *testing.T) {
	c := New(nil, "test:", time.Minute)
	called := false
	data, err := c.GetOrSet(context.Background(), "key", func() ([]byte, error) {
		called = true
		return []byte("computed"), nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatal("expected fn to be called")
	}
	if !bytes.Equal(data, []byte("computed")) {
		t.Fatalf("expected 'computed', got %q", string(data))
	}
}

func TestCache_GetOrSet_FnError(t *testing.T) {
	c := New(nil, "test:", time.Minute)
	_, err := c.GetOrSet(context.Background(), "key", func() ([]byte, error) {
		return nil, errors.New("fn error")
	})
	if err == nil {
		t.Fatal("expected error from fn")
	}
}

func TestCache_GetOrSet_FnReturnsNil(t *testing.T) {
	c := New(nil, "test:", time.Minute)
	data, err := c.GetOrSet(context.Background(), "key", func() ([]byte, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if data != nil {
		t.Fatal("expected nil data")
	}
}

func TestCache_JSON(t *testing.T) {
	c := New(nil, "test:", time.Minute)

	type TestStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	// SetJSON should not error with nil redis
	err := c.SetJSON(context.Background(), "key", TestStruct{Name: "test", Value: 42})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// GetJSON should miss
	var dst TestStruct
	ok, err := c.GetJSON(context.Background(), "key", &dst)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ok {
		t.Fatal("expected miss")
	}
}

func TestCache_Stats_Nil(t *testing.T) {
	c := New(nil, "test:", time.Minute)
	stats := c.Stats(context.Background())
	if stats["keys"] != 0 {
		t.Fatalf("expected 0 keys, got %d", stats["keys"])
	}
}

func TestCache_DefaultTTL(t *testing.T) {
	c := New(nil, "test:", 30*time.Second)
	if c.ttl != 30*time.Second {
		t.Fatalf("expected 30s TTL, got %v", c.ttl)
	}
}

func TestCache_SetTTL_Nil(t *testing.T) {
	c := New(nil, "test:", time.Minute)
	// Should not panic
	c.SetTTL(context.Background(), "key", []byte("val"), 5*time.Second)
}

func TestCache_PrefixIsolation(t *testing.T) {
	c1 := New(nil, "app1:", time.Minute)
	c2 := New(nil, "app2:", time.Minute)
	if c1.key("k") == c2.key("k") {
		t.Fatal("expected different keys for different prefixes")
	}
}
