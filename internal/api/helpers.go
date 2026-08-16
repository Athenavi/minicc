package api

import (
	"github.com/athenavi/minicc/internal/id"
)

// genID produces a snowflake-based unique ID string.
func genID() string {
	return id.NextID()
}

// nullableStr returns nil for empty strings, useful for nullable DB columns.
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
