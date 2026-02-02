package social

import (
	"testing"
	"time"
)

func TestExtractTickers(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "dollar sign ticker",
			text:     "I'm bullish on $SPY today",
			expected: []string{"SPY"},
		},
		{
			name:     "multiple dollar sign tickers",
			text:     "$AAPL and $MSFT are looking strong. Also watching $GOOGL",
			expected: []string{"AAPL", "MSFT", "GOOGL"},
		},
		{
			name:     "known ticker without dollar sign",
			text:     "SPY is at all time highs, QQQ following",
			expected: []string{"SPY", "QQQ"},
		},
		{
			name:     "mixed format",
			text:     "$NVDA looking good, TSLA not so much",
			expected: []string{"NVDA", "TSLA"},
		},
		{
			name:     "no tickers",
			text:     "The market is looking uncertain today",
			expected: []string{},
		},
		{
			name:     "duplicate tickers",
			text:     "$SPY is up, SPY looking strong, $SPY bullish",
			expected: []string{"SPY"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractTickers(tt.text)

			// Check that all expected tickers are present
			resultMap := make(map[string]bool)
			for _, ticker := range result {
				resultMap[ticker] = true
			}

			for _, expected := range tt.expected {
				if !resultMap[expected] {
					t.Errorf("ExtractTickers() missing expected ticker %s", expected)
				}
			}

			// Check no unexpected tickers
			if len(result) != len(tt.expected) {
				t.Errorf("ExtractTickers() returned %d tickers, expected %d", len(result), len(tt.expected))
			}
		})
	}
}

func TestRateLimitDelay(t *testing.T) {
	delay := RateLimitDelay()
	if delay != 5*time.Second {
		t.Errorf("RateLimitDelay() = %v, want 5s", delay)
	}
}

func TestKnownTickers(t *testing.T) {
	// Verify key tickers are in the known list
	expectedTickers := []string{"SPY", "QQQ", "VOO", "VTI", "PLTR"}
	
	for _, ticker := range expectedTickers {
		if !knownTickers[ticker] {
			t.Errorf("knownTickers missing expected ticker: %s", ticker)
		}
	}
}
