package engine

import (
	"testing"

	"athena/services/analysis"
)

func TestCalculateAllocation(t *testing.T) {
	engine := &Engine{
		db:               nil, // Not needed for allocation tests
		vixHighThreshold: 25.0,
		rsiOverbought:    70.0,
		rsiOversold:      30.0,
	}

	tests := []struct {
		name           string
		ticker         string
		score          *analysis.ConfidenceScore
		budget         float64
		regime         MarketRegime
		expectedAction string
		minAmount      float64
		maxAmount      float64
	}{
		{
			name:   "high confidence core holding",
			ticker: "SPY",
			score: &analysis.ConfidenceScore{
				Overall:   0.85,
				Direction: "bullish",
			},
			budget:         1000.0,
			regime:         RegimeCalm,
			expectedAction: "buy",
			minAmount:      400.0, // 40% base allocation
			maxAmount:      450.0,
		},
		{
			name:   "low confidence reduces allocation",
			ticker: "SPY",
			score: &analysis.ConfidenceScore{
				Overall:   0.45,
				Direction: "bullish",
			},
			budget:         1000.0,
			regime:         RegimeCalm,
			expectedAction: "buy",
			minAmount:      150.0, // Reduced
			maxAmount:      250.0,
		},
		{
			name:   "very low confidence",
			ticker: "SPY",
			score: &analysis.ConfidenceScore{
				Overall:   0.30,
				Direction: "neutral",
			},
			budget:         1000.0,
			regime:         RegimeCalm,
			expectedAction: "buy",
			minAmount:      50.0,
			maxAmount:      150.0,
		},
		{
			name:   "bearish regime reduces allocation",
			ticker: "QQQ",
			score: &analysis.ConfidenceScore{
				Overall:   0.75,
				Direction: "bullish",
			},
			budget:         1000.0,
			regime:         RegimeBearish,
			expectedAction: "buy",
			minAmount:      200.0, // 30% base * 0.75 = 225
			maxAmount:      250.0,
		},
		{
			name:   "non-core low confidence waits",
			ticker: "PLTR",
			score: &analysis.ConfidenceScore{
				Overall:   0.50,
				Direction: "neutral",
			},
			budget:         1000.0,
			regime:         RegimeCalm,
			expectedAction: "wait",
			minAmount:      0,
			maxAmount:      0,
		},
		{
			name:   "non-core high confidence buys",
			ticker: "PLTR",
			score: &analysis.ConfidenceScore{
				Overall:   0.75,
				Direction: "bullish",
			},
			budget:         1000.0,
			regime:         RegimeCalm,
			expectedAction: "buy",
			minAmount:      90.0, // 10% risk allocation
			maxAmount:      110.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.calculateAllocation(tt.ticker, tt.score, tt.budget, tt.regime)

			if result.Action != tt.expectedAction {
				t.Errorf("calculateAllocation() action = %v, want %v", result.Action, tt.expectedAction)
			}

			if result.Amount < tt.minAmount || result.Amount > tt.maxAmount {
				t.Errorf("calculateAllocation() amount = %v, want between %v and %v",
					result.Amount, tt.minAmount, tt.maxAmount)
			}

			if result.Reasoning == "" {
				t.Error("calculateAllocation() reasoning is empty")
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.VIXHighThreshold != 25.0 {
		t.Errorf("DefaultConfig() VIXHighThreshold = %v, want 25.0", cfg.VIXHighThreshold)
	}

	if cfg.RSIOverbought != 70.0 {
		t.Errorf("DefaultConfig() RSIOverbought = %v, want 70.0", cfg.RSIOverbought)
	}

	if cfg.RSIOversold != 30.0 {
		t.Errorf("DefaultConfig() RSIOversold = %v, want 30.0", cfg.RSIOversold)
	}
}

func TestMarketRegimeConstants(t *testing.T) {
	// Verify regime constants are defined correctly
	if RegimeCalm != "calm" {
		t.Errorf("RegimeCalm = %v, want 'calm'", RegimeCalm)
	}

	if RegimeVolatile != "volatile" {
		t.Errorf("RegimeVolatile = %v, want 'volatile'", RegimeVolatile)
	}

	if RegimeBullish != "bullish" {
		t.Errorf("RegimeBullish = %v, want 'bullish'", RegimeBullish)
	}

	if RegimeBearish != "bearish" {
		t.Errorf("RegimeBearish = %v, want 'bearish'", RegimeBearish)
	}
}

func TestCoreHoldingsAllocation(t *testing.T) {
	// Verify core holdings allocation percentages sum to 100%
	total := 0.0
	for _, allocation := range coreHoldings {
		total += allocation
	}

	if total < 0.99 || total > 1.01 {
		t.Errorf("coreHoldings total = %v, want ~1.0", total)
	}
}
