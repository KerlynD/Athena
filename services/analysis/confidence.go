// Package analysis provides confidence scoring for investment recommendations.
// It combines multiple signals to generate overall confidence scores.
package analysis

import (
	"context"
	"database/sql"
	"fmt"
)

// ConfidenceWeights defines the weights for different signal components
type ConfidenceWeights struct {
	CreatorConsensus   float64
	TechnicalAlignment float64
	VolumeConfirmation float64
	HistoricalAccuracy float64
}

// DefaultWeights returns the default confidence weights
func DefaultWeights() ConfidenceWeights {
	return ConfidenceWeights{
		CreatorConsensus:   0.30,
		TechnicalAlignment: 0.30,
		VolumeConfirmation: 0.20,
		HistoricalAccuracy: 0.20,
	}
}

// ConfidenceInputs holds all inputs needed for confidence calculation
type ConfidenceInputs struct {
	Ticker               string
	CreatorSentiments    map[string]string  // creator -> sentiment (bullish/bearish/neutral)
	TechnicalSignals     []string           // list of signal types (bullish/bearish)
	CurrentVolume        int64
	AvgVolume            int64
	CreatorAccuracyRates map[string]float64 // creator -> historical accuracy (0-1)
}

// ConfidenceScore represents the calculated confidence with breakdown
type ConfidenceScore struct {
	Overall            float64
	CreatorConsensus   float64
	TechnicalAlignment float64
	VolumeConfirmation float64
	HistoricalAccuracy float64
	Direction          string // bullish or bearish based on signals
	Breakdown          string
}

// CalculateConfidence computes the overall confidence score from inputs
func CalculateConfidence(inputs ConfidenceInputs, weights ConfidenceWeights) ConfidenceScore {
	var score ConfidenceScore
	score.Direction = "neutral"

	// 1. Creator Consensus: % of creators with same sentiment
	bullishCount := 0
	bearishCount := 0
	totalCreators := len(inputs.CreatorSentiments)

	for _, sentiment := range inputs.CreatorSentiments {
		switch sentiment {
		case "bullish":
			bullishCount++
		case "bearish":
			bearishCount++
		}
	}

	if totalCreators > 0 {
		if bullishCount > bearishCount {
			score.CreatorConsensus = float64(bullishCount) / float64(totalCreators)
			score.Direction = "bullish"
		} else if bearishCount > bullishCount {
			score.CreatorConsensus = float64(bearishCount) / float64(totalCreators)
			score.Direction = "bearish"
		} else {
			// Split sentiment = low confidence
			score.CreatorConsensus = 0.5
		}
	}

	// 2. Technical Alignment: % of indicators signaling same direction
	bullishSignals := 0
	bearishSignals := 0
	totalSignals := len(inputs.TechnicalSignals)

	for _, signal := range inputs.TechnicalSignals {
		switch signal {
		case "bullish":
			bullishSignals++
		case "bearish":
			bearishSignals++
		}
	}

	if totalSignals > 0 {
		// Alignment is how many signals agree with the dominant direction
		if score.Direction == "bullish" {
			score.TechnicalAlignment = float64(bullishSignals) / float64(totalSignals)
		} else if score.Direction == "bearish" {
			score.TechnicalAlignment = float64(bearishSignals) / float64(totalSignals)
		} else {
			// If direction is neutral, use the higher of the two
			if bullishSignals > bearishSignals {
				score.TechnicalAlignment = float64(bullishSignals) / float64(totalSignals)
			} else {
				score.TechnicalAlignment = float64(bearishSignals) / float64(totalSignals)
			}
		}
	}

	// 3. Volume Confirmation: Current volume vs 20-day average
	score.VolumeConfirmation = 0.5 // Default neutral
	if inputs.AvgVolume > 0 {
		ratio := float64(inputs.CurrentVolume) / float64(inputs.AvgVolume)
		// Normalize: >2x = 1.0, <0.5x = 0.0, linear between
		switch {
		case ratio >= 2.0:
			score.VolumeConfirmation = 1.0
		case ratio <= 0.5:
			score.VolumeConfirmation = 0.0
		default:
			score.VolumeConfirmation = (ratio - 0.5) / 1.5
		}
	}

	// 4. Historical Accuracy: Average accuracy of creators making predictions
	if len(inputs.CreatorAccuracyRates) > 0 {
		sum := 0.0
		for _, accuracy := range inputs.CreatorAccuracyRates {
			sum += accuracy
		}
		score.HistoricalAccuracy = sum / float64(len(inputs.CreatorAccuracyRates))
	} else {
		score.HistoricalAccuracy = 0.5 // Default for unknown creators
	}

	// Calculate weighted overall score
	score.Overall = (score.CreatorConsensus * weights.CreatorConsensus) +
		(score.TechnicalAlignment * weights.TechnicalAlignment) +
		(score.VolumeConfirmation * weights.VolumeConfirmation) +
		(score.HistoricalAccuracy * weights.HistoricalAccuracy)

	// Build breakdown string
	score.Breakdown = fmt.Sprintf(
		"Creator: %.0f%% | Technical: %.0f%% | Volume: %.0f%% | History: %.0f%%",
		score.CreatorConsensus*100,
		score.TechnicalAlignment*100,
		score.VolumeConfirmation*100,
		score.HistoricalAccuracy*100,
	)

	return score
}

// FetchCreatorAccuracy retrieves historical accuracy rates from database
func FetchCreatorAccuracy(ctx context.Context, db *sql.DB, creators []string) (map[string]float64, error) {
	if len(creators) == 0 {
		return make(map[string]float64), nil
	}

	// Build query with placeholders
	query := `
		SELECT creator_name, 
			   COALESCE(AVG(CASE WHEN was_accurate THEN 1.0 ELSE 0.0 END), 0.5) as accuracy
		FROM creator_accuracy
		WHERE creator_name = ANY($1)
		GROUP BY creator_name
	`

	rows, err := db.QueryContext(ctx, query, creators)
	if err != nil {
		return nil, fmt.Errorf("query accuracy: %w", err)
	}
	defer rows.Close()

	rates := make(map[string]float64)
	for rows.Next() {
		var name string
		var accuracy float64
		if err := rows.Scan(&name, &accuracy); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		rates[name] = accuracy
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	// Default 0.5 for creators without history
	for _, creator := range creators {
		if _, exists := rates[creator]; !exists {
			rates[creator] = 0.5
		}
	}

	return rates, nil
}

// GetTechnicalSignals interprets technical indicators to generate signals
func GetTechnicalSignals(rsi float64, sma50 float64, sma200 float64, macd float64, macdSignal float64, currentPrice float64) []string {
	var signals []string

	// RSI signal
	if rsi > 0 {
		switch {
		case rsi < 30:
			signals = append(signals, "bullish") // Oversold
		case rsi > 70:
			signals = append(signals, "bearish") // Overbought
		default:
			signals = append(signals, "neutral")
		}
	}

	// Golden/Death Cross (SMA50 vs SMA200)
	if sma50 > 0 && sma200 > 0 {
		if sma50 > sma200 {
			signals = append(signals, "bullish") // Golden cross
		} else {
			signals = append(signals, "bearish") // Death cross
		}
	}

	// Price vs SMA200 (long-term trend)
	if sma200 > 0 && currentPrice > 0 {
		if currentPrice > sma200 {
			signals = append(signals, "bullish")
		} else {
			signals = append(signals, "bearish")
		}
	}

	// MACD signal
	if macd != 0 && macdSignal != 0 {
		if macd > macdSignal {
			signals = append(signals, "bullish")
		} else {
			signals = append(signals, "bearish")
		}
	}

	return signals
}
