//go:build integration

package database

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDatabaseService(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("set TEST_DATABASE_URL to a disposable local connectient_test database (see docs/testing.md)")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Database != "connectient_test" ||
		(config.ConnConfig.Host != "127.0.0.1" && config.ConnConfig.Host != "localhost" && config.ConnConfig.Host != "::1") {
		t.Fatal("test requires a local database named connectient_test")
	}
	t.Setenv("DATABASE_URL", dsn)
	srv := NewDb()
	if srv == nil {
		t.Fatal("NewDb returned nil")
	}
	t.Cleanup(func() {
		_ = srv.Close()
		dbInstance = nil
	})
	if NewDb() != srv {
		t.Error("NewDb did not reuse the service")
	}

	stats := srv.Health()
	if stats.Status != "up" || stats.Error != "" {
		t.Fatalf("database health: %+v", stats)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if stats := srv.Health(); stats.Status != "down" {
		t.Errorf("health after close = %q, want down", stats.Status)
	}
}
