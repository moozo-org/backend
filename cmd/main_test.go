package main

import (
	"os"
	"testing"

	"moozo/internal/config"

	"github.com/stretchr/testify/require"
)

// t.Setenv registers the restore; os.Unsetenv does the actual clearing.
func clearEnv(t *testing.T) {
	for _, k := range []string{"PORT", "PRODUCTION", "ENV_PRODUCTION", "MONGODB_URI"} {
		t.Setenv(k, "")
		err := os.Unsetenv(k)
		require.NoError(t, err)
	}
}

func TestConfigDefaults(t *testing.T) {
	clearEnv(t)

	cfg, err := config.Load[Config]()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.Production {
		t.Error("Production should default to false")
	}
	if cfg.MongoURI != "mongodb://localhost:27017" {
		t.Errorf("MongoURI = %q", cfg.MongoURI)
	}
}

func TestConfigReadsUppercaseNames(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("PRODUCTION", "true")
	t.Setenv("MONGODB_URI", "mongodb://mongo:27017")

	cfg, err := config.Load[Config]()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
	if !cfg.Production {
		t.Error("Production = false, want true")
	}
	if cfg.MongoURI != "mongodb://mongo:27017" {
		t.Errorf("MongoURI = %q", cfg.MongoURI)
	}
}

// A named Env field would let ENV_PRODUCTION override PRODUCTION.
func TestProductionIsThePrimaryKey(t *testing.T) {
	clearEnv(t)
	t.Setenv("ENV_PRODUCTION", "true")

	cfg, err := config.Load[Config]()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Production {
		t.Error("ENV_PRODUCTION must not set Production")
	}

	t.Setenv("PRODUCTION", "true")
	cfg, _ = config.Load[Config]()
	if !cfg.Production {
		t.Error("PRODUCTION must set Production")
	}
}
