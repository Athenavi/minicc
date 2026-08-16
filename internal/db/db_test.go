package db

import (
	"context"
	"testing"
)

func TestConnectPostgres_InvalidDSN(t *testing.T) {
	err := ConnectPostgres(context.Background(), "invalid-dsn", 1, 1)
	if err == nil {
		t.Fatal("expected error for invalid DSN")
	}
}

func TestClosePostgres_NilPool(t *testing.T) {
	// Should not panic
	ClosePostgres()
}

func TestConnectRedis_InvalidAddr(t *testing.T) {
	err := ConnectRedis(context.Background(), "invalid:addr", "", 0)
	if err == nil {
		t.Fatal("expected error for invalid addr")
	}
}

func TestCloseRedis_NilClient(t *testing.T) {
	// Should not panic
	CloseRedis()
}

func TestRunAtlasMigrations_NilPool(t *testing.T) {
	err := RunAtlasMigrations(context.Background(), nil, "migrations")
	if err == nil {
		t.Fatal("expected error for nil pool")
	}
}

func TestGlobalVars_InitiallyNil(t *testing.T) {
	if Pool != nil {
		t.Fatal("expected nil Pool before ConnectPostgres")
	}
	if Redis != nil {
		t.Fatal("expected nil Redis before ConnectRedis")
	}
}
