// Package engine provides the recommendation engine for the Market Intelligence Aggregator.
// It generates buy/hold/wait recommendations based on market regime and confidence scores.
package engine

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"athena/services/analysis"
)

// MarketRegime represents the current market condition
type MarketRegime string

const (
	RegimeCalm     MarketRegime = "calm"
	RegimeVolatile MarketRegime = "volatile"
	RegimeBullish  MarketRegime = "bullish"
	RegimeBearish  MarketRegime = "bearish"
)

// Recommendation represents an investment recommendation
type Recommendation struct {
	Ticker          string
	Action          string  // buy, hold, wait
	Amount          float64 // Dollar amount
	ConfidenceScore float64
	Reasoning       string
	MarketRegime    MarketRegime
	VIXLevel        float64
}

// Engine handles recommendation generation
type Engine struct {
	db               *sql.DB
	vixHighThreshold float64
	rsiOverbought    float64
	rsiOversold      float64
}

// Config holds engine configuration
type Config struct {
	VIXHighThreshold float64
	RSIOverbought    float64
	RSIOversold      float64
}

// DefaultConfig returns default engine configuration
func DefaultConfig() Config {
	return Config{
		VIXHighThreshold: 25.0,
		RSIOverbought:    70.0,
		RSIOversold:      30.0,
	}
}

// NewEngine creates a new recommendation engine
func NewEngine(db *sql.DB, cfg Config) *Engine {
	return &Engine{
		db:               db,
		vixHighThreshold: cfg.VIXHighThreshold,
		rsiOverbought:    cfg.RSIOverbought,
		rsiOversold:      cfg.RSIOversold,
	}
}

// GenerateRecommendations generates recommendations for the given budget
func (e *Engine) GenerateRecommendations(ctx context.Context, budget float64) ([]Recommendation, error) {
	log.Printf("Generating recommendations for budget: $%.2f", budget)

	// 1. Determine market regime
	regime, vix, err := e.detectMarketRegime(ctx)
	if err != nil {
		log.Printf("Warning: could not detect market regime: %v", err)
		regime = RegimeCalm
		vix = 0
	}
	log.Printf("Market regime: %s (VIX: %.2f)", regime, vix)

	// 2. If high volatility, recommend holding cash
	if regime == RegimeVolatile {
		return []Recommendation{
			{
				Action:       "wait",
				Reasoning:    fmt.Sprintf("High volatility detected (VIX: %.2f). Wait 2-3 days for market to stabilize.", vix),
				VIXLevel:     vix,
				MarketRegime: regime,
			},
		}, nil
	}

	// 3. Fetch tracked tickers
	tickers, err := e.getTrackedTickers(ctx)
	if err != nil {
		return nil, fmt.Errorf("get tracked tickers: %w", err)
	}

	// 4. Generate recommendations for each ticker
	recommendations := make([]Recommendation, 0, len(tickers))

	for _, ticker := range tickers {
		score, err := e.getTickerConfidenceScore(ctx, ticker)
		if err != nil {
			log.Printf("Warning: could not get confidence score for %s: %v", ticker, err)
			continue
		}

		// Calculate allocation
		allocation := e.calculateAllocation(ticker, score, budget, regime)

		recommendations = append(recommendations, Recommendation{
			Ticker:          ticker,
			Action:          allocation.Action,
			Amount:          allocation.Amount,
			ConfidenceScore: score.Overall,
			Reasoning:       allocation.Reasoning,
			MarketRegime:    regime,
			VIXLevel:        vix,
		})
	}

	return recommendations, nil
}

// detectMarketRegime determines the current market condition
func (e *Engine) detectMarketRegime(ctx context.Context) (MarketRegime, float64, error) {
	// Fetch VIX data
	var vix sql.NullFloat64
	err := e.db.QueryRowContext(ctx, `
		SELECT close FROM market_data
		WHERE ticker = 'VIX' OR ticker = '^VIX'
		ORDER BY timestamp DESC
		LIMIT 1
	`).Scan(&vix)

	if err != nil && err != sql.ErrNoRows {
		return RegimeCalm, 0, fmt.Errorf("query VIX: %w", err)
	}

	vixLevel := 0.0
	if vix.Valid {
		vixLevel = vix.Float64
	}

	// High VIX = volatile market
	if vixLevel > e.vixHighThreshold {
		return RegimeVolatile, vixLevel, nil
	}

	// Check SPY RSI for trend
	var rsi sql.NullFloat64
	err = e.db.QueryRowContext(ctx, `
		SELECT rsi_14 FROM technical_indicators
		WHERE ticker = 'SPY'
		ORDER BY timestamp DESC
		LIMIT 1
	`).Scan(&rsi)

	if err == nil && rsi.Valid {
		if rsi.Float64 > e.rsiOverbought {
			return RegimeBearish, vixLevel, nil
		} else if rsi.Float64 < e.rsiOversold {
			return RegimeBullish, vixLevel, nil
		}
	}

	return RegimeCalm, vixLevel, nil
}

// getTrackedTickers retrieves the list of tracked tickers from config
func (e *Engine) getTrackedTickers(ctx context.Context) ([]string, error) {
	// Default tickers
	defaultTickers := []string{"SPY", "QQQ", "VOO", "VTI"}

	var tickersJSON string
	err := e.db.QueryRowContext(ctx, `
		SELECT value FROM config WHERE key = 'tracked_tickers'
	`).Scan(&tickersJSON)

	if err != nil {
		if err == sql.ErrNoRows {
			return defaultTickers, nil
		}
		return nil, fmt.Errorf("query config: %w", err)
	}

	// Parse JSON array (simple parsing without full JSON library)
	// For proper implementation, use encoding/json
	return defaultTickers, nil
}

// getTickerConfidenceScore retrieves or calculates the confidence score for a ticker
func (e *Engine) getTickerConfidenceScore(ctx context.Context, ticker string) (*analysis.ConfidenceScore, error) {
	// 1. Fetch recent creator sentiments for the ticker
	creatorSentiments := make(map[string]string)
	rows, err := e.db.QueryContext(ctx, `
		SELECT DISTINCT creator_name, sentiment
		FROM creator_content
		WHERE $1 = ANY(mentioned_tickers)
			AND sentiment IS NOT NULL
			AND posted_at >= NOW() - INTERVAL '7 days'
	`, ticker)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var creator, sentiment string
			if rows.Scan(&creator, &sentiment) == nil {
				creatorSentiments[creator] = sentiment
			}
		}
	}

	// 2. Fetch technical indicators
	var rsi, sma50, sma200, macd, macdSignal sql.NullFloat64
	var currentPrice float64
	var currentVolume, avgVolume sql.NullInt64

	e.db.QueryRowContext(ctx, `
		SELECT rsi_14, sma_50, sma_200, macd, macd_signal
		FROM technical_indicators
		WHERE ticker = $1
		ORDER BY timestamp DESC LIMIT 1
	`, ticker).Scan(&rsi, &sma50, &sma200, &macd, &macdSignal)

	e.db.QueryRowContext(ctx, `
		SELECT close, volume FROM market_data
		WHERE ticker = $1
		ORDER BY timestamp DESC LIMIT 1
	`, ticker).Scan(&currentPrice, &currentVolume)

	e.db.QueryRowContext(ctx, `
		SELECT volume_avg_20 FROM technical_indicators
		WHERE ticker = $1 AND volume_avg_20 IS NOT NULL
		ORDER BY timestamp DESC LIMIT 1
	`, ticker).Scan(&avgVolume)

	// 3. Generate technical signals
	var technicalSignals []string
	if rsi.Valid {
		technicalSignals = analysis.GetTechnicalSignals(
			rsi.Float64,
			sma50.Float64,
			sma200.Float64,
			macd.Float64,
			macdSignal.Float64,
			currentPrice,
		)
	}

	// 4. Get creator accuracy rates
	var creators []string
	for creator := range creatorSentiments {
		creators = append(creators, creator)
	}
	accuracyRates, _ := analysis.FetchCreatorAccuracy(ctx, e.db, creators)

	// 5. Build inputs and calculate confidence
	inputs := analysis.ConfidenceInputs{
		Ticker:               ticker,
		CreatorSentiments:    creatorSentiments,
		TechnicalSignals:     technicalSignals,
		CurrentVolume:        currentVolume.Int64,
		AvgVolume:            avgVolume.Int64,
		CreatorAccuracyRates: accuracyRates,
	}

	score := analysis.CalculateConfidence(inputs, analysis.DefaultWeights())
	return &score, nil
}

// AllocationResult holds the calculated allocation
type AllocationResult struct {
	Action    string
	Amount    float64
	Reasoning string
}

// Core holdings allocation strategy
var coreHoldings = map[string]float64{
	"SPY": 0.40, // 40% of budget
	"QQQ": 0.30, // 30% of budget
	"VOO": 0.20, // 20% of budget
	"VTI": 0.10, // 10% of budget
}

// calculateAllocation determines how much to allocate to a ticker
func (e *Engine) calculateAllocation(ticker string, score *analysis.ConfidenceScore, budget float64, regime MarketRegime) AllocationResult {
	baseAllocation, isCore := coreHoldings[ticker]

	if !isCore {
		// Risk allocation for non-core holdings
		if score.Overall < 0.6 {
			return AllocationResult{
				Action:    "wait",
				Amount:    0,
				Reasoning: fmt.Sprintf("Confidence too low (%.0f%%) for risk allocation", score.Overall*100),
			}
		}

		// Allocate 10% of budget to risk positions with high confidence
		return AllocationResult{
			Action:    "buy",
			Amount:    budget * 0.10,
			Reasoning: fmt.Sprintf("Risk allocation approved (%.0f%% confidence)", score.Overall*100),
		}
	}

	// Core holding allocation
	amount := budget * baseAllocation

	// Adjust based on confidence and market regime
	switch {
	case score.Overall < 0.4:
		// Very low confidence - significantly reduce
		amount *= 0.25
		return AllocationResult{
			Action:    "buy",
			Amount:    amount,
			Reasoning: fmt.Sprintf("Reduced allocation due to low confidence (%.0f%%)", score.Overall*100),
		}
	case score.Overall < 0.6:
		// Moderate confidence - reduce by 50%
		amount *= 0.5
		return AllocationResult{
			Action:    "buy",
			Amount:    amount,
			Reasoning: fmt.Sprintf("Reduced allocation due to moderate confidence (%.0f%%)", score.Overall*100),
		}
	case regime == RegimeBearish:
		// Bearish market - reduce exposure
		amount *= 0.75
		return AllocationResult{
			Action:    "buy",
			Amount:    amount,
			Reasoning: fmt.Sprintf("Bearish regime - conservative allocation (%.0f%% confidence)", score.Overall*100),
		}
	case regime == RegimeBullish && score.Overall > 0.8:
		// Bullish market with high confidence - increase slightly
		amount *= 1.1
		if amount > budget*baseAllocation*1.2 {
			amount = budget * baseAllocation * 1.2 // Cap at 120% of base
		}
		return AllocationResult{
			Action:    "buy",
			Amount:    amount,
			Reasoning: fmt.Sprintf("Bullish regime with high confidence (%.0f%%) - increased allocation", score.Overall*100),
		}
	default:
		// Standard allocation
		return AllocationResult{
			Action:    "buy",
			Amount:    amount,
			Reasoning: fmt.Sprintf("Standard allocation (%.0f%% confidence)", score.Overall*100),
		}
	}
}

// StoreRecommendation saves a recommendation to the database
func (e *Engine) StoreRecommendation(ctx context.Context, rec Recommendation) error {
	_, err := e.db.ExecContext(ctx, `
		INSERT INTO signals 
		(ticker, signal_type, recommendation_amount, confidence_score, reasoning, market_regime, vix_level, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`, rec.Ticker, rec.Action, rec.Amount, rec.ConfidenceScore, rec.Reasoning, rec.MarketRegime, rec.VIXLevel)

	if err != nil {
		return fmt.Errorf("insert signal: %w", err)
	}

	return nil
}
