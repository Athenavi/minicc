package api

import (
	"fmt"
	"time"
)

// genID produces a simple nanosecond-based unique ID.
func genID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// nullableStr returns nil for empty strings, useful for nullable DB columns.
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
