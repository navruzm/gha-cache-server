package config

import (
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("API_BASE_URL", "http://localhost:3000")
	cfg, err := Load(envFunc(map[string]string{
		"API_BASE_URL": "http://localhost:3000",
	}))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.StorageDriver != "filesystem" {
		t.Errorf("default storage driver = %q, want filesystem", cfg.StorageDriver)
	}
	if cfg.DBDriver != "sqlite" {
		t.Errorf("default DB driver = %q, want sqlite", cfg.DBDriver)
	}
	if cfg.StorageFilesystemPath != ".data/storage/filesystem" {
		t.Errorf("default fs path = %q", cfg.StorageFilesystemPath)
	}
	if cfg.DBSQLitePath != ".data/sqlite.db" {
		t.Errorf("default sqlite path = %q", cfg.DBSQLitePath)
	}
	if cfg.CacheCleanupOlderThanDays != 90 {
		t.Errorf("default cleanup days = %d", cfg.CacheCleanupOlderThanDays)
	}
	if cfg.DefaultActionsResultsURL == "" {
		t.Error("expected DefaultActionsResultsURL default")
	}
}

func TestLoad_RequiresAPIBaseURL(t *testing.T) {
	_, err := Load(envFunc(map[string]string{}))
	if err == nil {
		t.Fatal("expected error when API_BASE_URL missing")
	}
}

func TestLoad_S3Variant(t *testing.T) {
	cfg, err := Load(envFunc(map[string]string{
		"API_BASE_URL":      "http://localhost:3000",
		"STORAGE_DRIVER":    "s3",
		"STORAGE_S3_BUCKET": "test-bucket",
		"AWS_REGION":        "eu-west-1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.StorageDriver != "s3" || cfg.S3Bucket != "test-bucket" || cfg.AWSRegion != "eu-west-1" {
		t.Errorf("got %+v", cfg)
	}
}

func TestLoad_PostgresFromURL(t *testing.T) {
	cfg, err := Load(envFunc(map[string]string{
		"API_BASE_URL":    "http://localhost:3000",
		"DB_DRIVER":       "postgres",
		"DB_POSTGRES_URL": "postgres://u:p@h:5432/db",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DBDriver != "postgres" || cfg.PostgresURL != "postgres://u:p@h:5432/db" {
		t.Errorf("got %+v", cfg)
	}
}

func TestLoad_PORTBeatsListenAddr(t *testing.T) {
	cfg, err := Load(envFunc(map[string]string{
		"API_BASE_URL": "http://localhost:3000",
		"PORT":         "8080",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want :8080", cfg.ListenAddr)
	}
}

func TestLoad_NITRO_PORTAlsoWorks(t *testing.T) {
	cfg, _ := Load(envFunc(map[string]string{
		"API_BASE_URL": "http://localhost:3000",
		"NITRO_PORT":   "9090",
	}))
	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want :9090", cfg.ListenAddr)
	}
}

func envFunc(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}
