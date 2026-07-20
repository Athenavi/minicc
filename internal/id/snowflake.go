// Package id provides a Snowflake-style unique ID generator.
//
// Format (64-bit):
//   1 bit  : unused (sign bit, always 0)
//   41 bits: timestamp in milliseconds since custom epoch
//   10 bits: worker ID (0-1023)
//   12 bits: sequence number (0-4095)
//
// IDs are returned as compact base62 strings (e.g. "7Jq2r3kLx9").
package id

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	epochMillis int64 = 1700000000000 // 2023-11-14T22:13:20Z

	workerBits  uint8 = 10
	seqBits     uint8 = 12
	workerShift       = seqBits
	timeShift         = seqBits + workerBits
	workerMax         = -1 ^ (-1 << workerBits)
	seqMax            = -1 ^ (-1 << seqBits)
)

var (
	defaultGenerator *Generator
	once             sync.Once
)

// Generator is a Snowflake-style ID generator.
type Generator struct {
	mu       sync.Mutex
	workerID int64
	lastTime int64
	seq      int64
}

// New creates a Generator with the given worker ID (0-1023).
func New(workerID int64) (*Generator, error) {
	if workerID < 0 || workerID > workerMax {
		return nil, fmt.Errorf("worker ID must be between 0 and %d", workerMax)
	}
	return &Generator{workerID: workerID}, nil
}

// Next returns a unique base62-encoded ID string.
func (g *Generator) Next() string {
	return base62Encode(g.nextInt64())
}

// nextInt64 generates the next unique int64 ID.
func (g *Generator) nextInt64() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UnixMilli() - epochMillis
	if now < 0 {
		// 时钟早于 epoch：只使用 sequence 保证唯一性，避免忙等死循环
		g.seq = (g.seq + 1) & seqMax
		return (0 << timeShift) | (g.workerID << workerShift) | g.seq
	}
	if now <= g.lastTime {
		// Same millisecond or clock regression — reuse lastTime to guarantee uniqueness.
		g.seq = (g.seq + 1) & seqMax
		if g.seq == 0 {
			// Sequence exhausted — wait for real clock to advance past lastTime.
			for now <= g.lastTime {
				now = time.Now().UnixMilli() - epochMillis
			}
			g.lastTime = now
		}
		return (g.lastTime << timeShift) | (g.workerID << workerShift) | g.seq
	}

	// New millisecond — reset sequence.
	g.seq = 0
	g.lastTime = now
	return (now << timeShift) | (g.workerID << workerShift) | g.seq
}

// NextID returns a unique ID using the package-level default generator.
// The default generator reads WORKER_ID from environment (0-1023), defaulting to 0.
func NextID() string {
	once.Do(func() {
		wid := int64(0)
		if v := os.Getenv("WORKER_ID"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 && n <= workerMax {
				wid = n
			}
		}
		defaultGenerator, _ = New(wid)
	})
	return defaultGenerator.Next()
}

// base62Encode encodes an int64 as a base62 string.
const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func base62Encode(n int64) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 12)
	for n > 0 {
		buf = append(buf, base62Chars[n%62])
		n /= 62
	}
	// Reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
