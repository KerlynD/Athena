// Package config provides configuration loading for the Market Intelligence Aggregator.
// It loads settings from both environment variables and the database config table.
package config

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Config holds all application configuration
type Config struct {
	// API Keys (from environment)
	AlphaVantageAPIKey string
	TwitterBearerToken string
	AnthropicAPIKey    string

	// Robinhood credentials (from environment)
	RobinhoodUsername string
	RobinhoodPassword string
	RobinhoodTOTP     string

	// Tracking configuration
	TrackedTickers []string
	Creators       []Creator

	// Scoring weights (from database)
	ConfidenceWeights ConfidenceWeights

	// Market thresholds (from database)
	MarketThresholds MarketThresholds

	// Contribution targets (from database)
	ContributionTarget ContributionTarget
}

// Creator represents a social media creator to track
type Creator struct {
	Name          string `json:"name"`
	TwitterHandle string `json:"twitter_handle"`
}

// ConfidenceWeights for scoring calculations
type ConfidenceWeights struct {
	CreatorConsensus   float64 `json:"creator_consensus"`
	TechnicalAlignment float64 `json:"technical_alignment"`
	VolumeConfirmation float64 `json:"volume_confirmation"`
	HistoricalAccuracy float64 `json:"historical_accuracy"`
}

// MarketThresholds for regime detection
type MarketThresholds struct {
	VIXHigh       float64 `json:"vix_high"`
	RSIOverbought float64 `json:"rsi_overbought"`
	RSIOversold   float64 `json:"rsi_oversold"`
}

// ContributionTarget for investment tracking
type ContributionTarget struct {
	Monthly float64 `json:"monthly"`
	Annual  float64 `json:"annual"`
	Current float64 `json:"current"`
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() (*Config, error) {
	cfg := &Config{}

	// Load API keys (required)
	cfg.AlphaVantageAPIKey = os.Getenv("ALPHAVANTAGE_API_KEY")
	if cfg.AlphaVantageAPIKey == "" {
		return nil, fmt.Errorf("ALPHAVANTAGE_API_KEY is not set")
	}

	cfg.TwitterBearerToken = os.Getenv("TWITTER_BEARER_TOKEN")
	if cfg.TwitterBearerToken == "" {
		return nil, fmt.Errorf("TWITTER_BEARER_TOKEN is not set")
	}

	cfg.AnthropicAPIKey = os.Getenv("ANTHROPIC_API_KEY")
	if cfg.AnthropicAPIKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set")
	}

	// Load Robinhood credentials (optional - Python uses these directly)
	cfg.RobinhoodUsername = os.Getenv("ROBINHOOD_USERNAME")
	cfg.RobinhoodPassword = os.Getenv("ROBINHOOD_PASSWORD")
	cfg.RobinhoodTOTP = os.Getenv("ROBINHOOD_TOTP")

	// Load tracking configuration
	tickersStr := os.Getenv("TRACKED_TICKERS")
	if tickersStr == "" {
		tickersStr = "SPY,QQQ,VOO,VTI" // Default
	}
	cfg.TrackedTickers = strings.Split(tickersStr, ",")
	for i := range cfg.TrackedTickers {
		cfg.TrackedTickers[i] = strings.TrimSpace(cfg.TrackedTickers[i])
	}

	creatorsStr := os.Getenv("CREATORS")
	if creatorsStr == "" {
		creatorsStr = "mobyinvest,carbonfinance" // Default
	}
	creatorHandles := strings.Split(creatorsStr, ",")
	cfg.Creators = make([]Creator, len(creatorHandles))
	for i, handle := range creatorHandles {
		handle = strings.TrimSpace(handle)
		cfg.Creators[i] = Creator{
			Name:          handle,
			TwitterHandle: handle,
		}
	}

	// Set default weights and thresholds
	cfg.ConfidenceWeights = ConfidenceWeights{
		CreatorConsensus:   0.3,
		TechnicalAlignment: 0.3,
		VolumeConfirmation: 0.2,
		HistoricalAccuracy: 0.2,
	}

	cfg.MarketThresholds = MarketThresholds{
		VIXHigh:       25.0,
		RSIOverbought: 70.0,
		RSIOversold:   30.0,
	}

	cfg.ContributionTarget = ContributionTarget{
		Monthly: 1000.0,
		Annual:  7000.0,
		Current: 600.0,
	}

	return cfg, nil
}

// LoadFromDB loads additional configuration from the database config table
func LoadFromDB(ctx context.Context, db *sql.DB, cfg *Config) error {
	// Load tracked tickers
	var tickersJSON string
	err := db.QueryRowContext(ctx, "SELECT value FROM config WHERE key = 'tracked_tickers'").Scan(&tickersJSON)
	if err == nil {
		var tickers []string
		if jsonErr := json.Unmarshal([]byte(tickersJSON), &tickers); jsonErr == nil {
			cfg.TrackedTickers = tickers
		}
	}

	// Load creators
	var creatorsJSON string
	err = db.QueryRowContext(ctx, "SELECT value FROM config WHERE key = 'creators'").Scan(&creatorsJSON)
	if err == nil {
		var creators []Creator
		if jsonErr := json.Unmarshal([]byte(creatorsJSON), &creators); jsonErr == nil {
			cfg.Creators = creators
		}
	}

	// Load confidence weights
	var weightsJSON string
	err = db.QueryRowContext(ctx, "SELECT value FROM config WHERE key = 'confidence_weights'").Scan(&weightsJSON)
	if err == nil {
		if jsonErr := json.Unmarshal([]byte(weightsJSON), &cfg.ConfidenceWeights); jsonErr != nil {
			return fmt.Errorf("parse confidence weights: %w", jsonErr)
		}
	}

	// Load market thresholds
	var thresholdsJSON string
	err = db.QueryRowContext(ctx, "SELECT value FROM config WHERE key = 'market_regime_thresholds'").Scan(&thresholdsJSON)
	if err == nil {
		if jsonErr := json.Unmarshal([]byte(thresholdsJSON), &cfg.MarketThresholds); jsonErr != nil {
			return fmt.Errorf("parse market thresholds: %w", jsonErr)
		}
	}

	// Load contribution target
	var targetJSON string
	err = db.QueryRowContext(ctx, "SELECT value FROM config WHERE key = 'contribution_target'").Scan(&targetJSON)
	if err == nil {
		if jsonErr := json.Unmarshal([]byte(targetJSON), &cfg.ContributionTarget); jsonErr != nil {
			return fmt.Errorf("parse contribution target: %w", jsonErr)
		}
	}

	return nil
}

// Load combines environment and database configuration
func Load(ctx context.Context, db *sql.DB) (*Config, error) {
	cfg, err := LoadFromEnv()
	if err != nil {
		return nil, fmt.Errorf("load from env: %w", err)
	}

	if err := LoadFromDB(ctx, db, cfg); err != nil {
		return nil, fmt.Errorf("load from db: %w", err)
	}

	return cfg, nil
}
