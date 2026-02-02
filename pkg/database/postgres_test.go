package database

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	// Note: This test doesn't require DATABASE_URL to be set
	cfg := DefaultConfig()

	if cfg.MaxOpenConns != 25 {
		t.Errorf("DefaultConfig() MaxOpenConns = %v, want 25", cfg.MaxOpenConns)
	}

	if cfg.MaxIdleConns != 5 {
		t.Errorf("DefaultConfig() MaxIdleConns = %v, want 5", cfg.MaxIdleConns)
	}

	if cfg.ConnMaxLifetime != 5*time.Minute {
		t.Errorf("DefaultConfig() ConnMaxLifetime = %v, want 5m", cfg.ConnMaxLifetime)
	}
}

func TestNew_MissingURL(t *testing.T) {
	cfg := Config{
		URL:             "", // Empty URL
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("New() should return error for empty URL")
	}
}
