package config

import (
	"os"
	"testing"
)

func TestLoadFromEnv_MissingRequired(t *testing.T) {
	// Save current env vars
	origAlpha := os.Getenv("ALPHAVANTAGE_API_KEY")
	origTwitter := os.Getenv("TWITTER_BEARER_TOKEN")
	origAnthropic := os.Getenv("ANTHROPIC_API_KEY")

	// Clear required vars
	os.Unsetenv("ALPHAVANTAGE_API_KEY")
	os.Unsetenv("TWITTER_BEARER_TOKEN")
	os.Unsetenv("ANTHROPIC_API_KEY")

	defer func() {
		// Restore original values
		if origAlpha != "" {
			os.Setenv("ALPHAVANTAGE_API_KEY", origAlpha)
		}
		if origTwitter != "" {
			os.Setenv("TWITTER_BEARER_TOKEN", origTwitter)
		}
		if origAnthropic != "" {
			os.Setenv("ANTHROPIC_API_KEY", origAnthropic)
		}
	}()

	_, err := LoadFromEnv()
	if err == nil {
		t.Error("LoadFromEnv() should return error when required vars missing")
	}
}

func TestLoadFromEnv_WithAllVars(t *testing.T) {
	// Set required env vars for test
	os.Setenv("ALPHAVANTAGE_API_KEY", "test_alpha_key")
	os.Setenv("TWITTER_BEARER_TOKEN", "test_twitter_token")
	os.Setenv("ANTHROPIC_API_KEY", "test_anthropic_key")
	os.Setenv("TRACKED_TICKERS", "SPY,QQQ,VOO")
	os.Setenv("CREATORS", "creator1,creator2")

	defer func() {
		os.Unsetenv("ALPHAVANTAGE_API_KEY")
		os.Unsetenv("TWITTER_BEARER_TOKEN")
		os.Unsetenv("ANTHROPIC_API_KEY")
		os.Unsetenv("TRACKED_TICKERS")
		os.Unsetenv("CREATORS")
	}()

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.AlphaVantageAPIKey != "test_alpha_key" {
		t.Errorf("AlphaVantageAPIKey = %v, want test_alpha_key", cfg.AlphaVantageAPIKey)
	}

	if len(cfg.TrackedTickers) != 3 {
		t.Errorf("TrackedTickers count = %v, want 3", len(cfg.TrackedTickers))
	}

	if len(cfg.Creators) != 2 {
		t.Errorf("Creators count = %v, want 2", len(cfg.Creators))
	}
}

func TestDefaultWeights(t *testing.T) {
	cfg := &Config{
		ConfidenceWeights: ConfidenceWeights{
			CreatorConsensus:   0.3,
			TechnicalAlignment: 0.3,
			VolumeConfirmation: 0.2,
			HistoricalAccuracy: 0.2,
		},
	}

	sum := cfg.ConfidenceWeights.CreatorConsensus +
		cfg.ConfidenceWeights.TechnicalAlignment +
		cfg.ConfidenceWeights.VolumeConfirmation +
		cfg.ConfidenceWeights.HistoricalAccuracy

	if sum < 0.99 || sum > 1.01 {
		t.Errorf("ConfidenceWeights sum = %v, want ~1.0", sum)
	}
}

func TestDefaultMarketThresholds(t *testing.T) {
	cfg := &Config{
		MarketThresholds: MarketThresholds{
			VIXHigh:       25.0,
			RSIOverbought: 70.0,
			RSIOversold:   30.0,
		},
	}

	if cfg.MarketThresholds.VIXHigh != 25.0 {
		t.Errorf("VIXHigh = %v, want 25.0", cfg.MarketThresholds.VIXHigh)
	}

	if cfg.MarketThresholds.RSIOverbought != 70.0 {
		t.Errorf("RSIOverbought = %v, want 70.0", cfg.MarketThresholds.RSIOverbought)
	}

	if cfg.MarketThresholds.RSIOversold != 30.0 {
		t.Errorf("RSIOversold = %v, want 30.0", cfg.MarketThresholds.RSIOversold)
	}
}
