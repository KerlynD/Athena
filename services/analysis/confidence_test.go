package analysis

import (
	"testing"
)

func TestCalculateConfidence(t *testing.T) {
	weights := DefaultWeights()

	tests := []struct {
		name           string
		inputs         ConfidenceInputs
		expectedDir    string
		minOverall     float64
		maxOverall     float64
	}{
		{
			name: "all bullish signals",
			inputs: ConfidenceInputs{
				Ticker: "SPY",
				CreatorSentiments: map[string]string{
					"creator1": "bullish",
					"creator2": "bullish",
					"creator3": "bullish",
				},
				TechnicalSignals: []string{"bullish", "bullish", "bullish", "bullish"},
				CurrentVolume:    100000000,
				AvgVolume:        50000000, // 2x average = high volume
				CreatorAccuracyRates: map[string]float64{
					"creator1": 0.8,
					"creator2": 0.7,
					"creator3": 0.9,
				},
			},
			expectedDir: "bullish",
			minOverall:  0.8,
			maxOverall:  1.0,
		},
		{
			name: "all bearish signals",
			inputs: ConfidenceInputs{
				Ticker: "SPY",
				CreatorSentiments: map[string]string{
					"creator1": "bearish",
					"creator2": "bearish",
				},
				TechnicalSignals: []string{"bearish", "bearish", "bearish"},
				CurrentVolume:    100000000,
				AvgVolume:        50000000,
				CreatorAccuracyRates: map[string]float64{
					"creator1": 0.8,
					"creator2": 0.7,
				},
			},
			expectedDir: "bearish",
			minOverall:  0.8,
			maxOverall:  1.0,
		},
		{
			name: "mixed signals low confidence",
			inputs: ConfidenceInputs{
				Ticker: "SPY",
				CreatorSentiments: map[string]string{
					"creator1": "bullish",
					"creator2": "bearish",
				},
				TechnicalSignals: []string{"bullish", "bearish"},
				CurrentVolume:    30000000,
				AvgVolume:        50000000, // Below average
				CreatorAccuracyRates: map[string]float64{
					"creator1": 0.5,
					"creator2": 0.5,
				},
			},
			expectedDir: "neutral",
			minOverall:  0.3,
			maxOverall:  0.6,
		},
		{
			name: "empty inputs",
			inputs: ConfidenceInputs{
				Ticker:               "SPY",
				CreatorSentiments:    map[string]string{},
				TechnicalSignals:     []string{},
				CurrentVolume:        0,
				AvgVolume:            0,
				CreatorAccuracyRates: map[string]float64{},
			},
			expectedDir: "neutral",
			minOverall:  0.0,
			maxOverall:  0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := CalculateConfidence(tt.inputs, weights)

			if score.Direction != tt.expectedDir {
				t.Errorf("CalculateConfidence() direction = %v, want %v", score.Direction, tt.expectedDir)
			}

			if score.Overall < tt.minOverall || score.Overall > tt.maxOverall {
				t.Errorf("CalculateConfidence() overall = %v, want between %v and %v",
					score.Overall, tt.minOverall, tt.maxOverall)
			}

			// Verify breakdown string is not empty
			if score.Breakdown == "" {
				t.Error("CalculateConfidence() breakdown is empty")
			}
		})
	}
}

func TestDefaultWeights(t *testing.T) {
	weights := DefaultWeights()

	// Verify weights sum to approximately 1.0
	sum := weights.CreatorConsensus + weights.TechnicalAlignment +
		weights.VolumeConfirmation + weights.HistoricalAccuracy

	if sum < 0.99 || sum > 1.01 {
		t.Errorf("DefaultWeights() sum = %v, want ~1.0", sum)
	}
}

func TestGetTechnicalSignals(t *testing.T) {
	tests := []struct {
		name          string
		rsi           float64
		sma50         float64
		sma200        float64
		macd          float64
		macdSignal    float64
		currentPrice  float64
		expectedCount int
	}{
		{
			name:          "all indicators available",
			rsi:           45.0,
			sma50:         450.0,
			sma200:        440.0,
			macd:          2.5,
			macdSignal:    1.5,
			currentPrice:  455.0,
			expectedCount: 4, // RSI, SMA cross, price vs SMA200, MACD
		},
		{
			name:          "oversold RSI",
			rsi:           25.0,
			sma50:         0,
			sma200:        0,
			macd:          0,
			macdSignal:    0,
			currentPrice:  0,
			expectedCount: 1, // Only RSI signal
		},
		{
			name:          "overbought RSI",
			rsi:           75.0,
			sma50:         0,
			sma200:        0,
			macd:          0,
			macdSignal:    0,
			currentPrice:  0,
			expectedCount: 1, // Only RSI signal
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := GetTechnicalSignals(tt.rsi, tt.sma50, tt.sma200, tt.macd, tt.macdSignal, tt.currentPrice)

			if len(signals) != tt.expectedCount {
				t.Errorf("GetTechnicalSignals() returned %d signals, want %d", len(signals), tt.expectedCount)
			}
		})
	}
}
