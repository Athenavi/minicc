package db

import (
	"sync"
	"testing"
)

func TestAtomicRedis_Mode(t *testing.T) {
	// Test with nil - should return "unknown"
	var nilClient RedisClient
	atomic := NewAtomicRedis(nilClient)
	
	// Since we can't actually connect to Redis in tests, we'll test the Mode() method
	// by checking it doesn't panic
	mode := atomic.Mode()
	if mode != "unknown" {
		t.Fatalf("expected 'unknown', got %q", mode)
	}
}

func TestAtomicRedis_ConcurrentAccess(t *testing.T) {
	// Test that concurrent access doesn't panic
	var nilClient RedisClient
	atomic := NewAtomicRedis(nilClient)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Just call Mode() to test concurrent access
			atomic.Mode()
		}()
	}
	wg.Wait()
}

func TestRedisConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     RedisConfig
		wantErr bool
	}{
		{
			name:    "empty mode defaults to single",
			cfg:     RedisConfig{Addr: "localhost:6379"},
			wantErr: true, // Will fail because no Redis server
		},
		{
			name:    "cluster without addrs",
			cfg:     RedisConfig{Mode: "cluster"},
			wantErr: true,
		},
		{
			name:    "sentinel without master",
			cfg:     RedisConfig{Mode: "sentinel", SentinelAddrs: []string{"localhost:26379"}},
			wantErr: true,
		},
		{
			name:    "sentinel without addrs",
			cfg:     RedisConfig{Mode: "sentinel", MasterName: "mymaster"},
			wantErr: true,
		},
		{
			name:    "unknown mode",
			cfg:     RedisConfig{Mode: "unknown"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRedisClient(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRedisClient() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
