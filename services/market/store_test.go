package market

import (
	"testing"
	"time"
)

func TestMarketDataStruct(t *testing.T) {
	data := MarketData{
		Ticker:    "SPY",
		Timestamp: time.Now(),
		Open:      450.00,
		High:      455.00,
		Low:       449.00,
		Close:     453.50,
		Volume:    50000000,
	}

	if data.Ticker != "SPY" {
		t.Errorf("Ticker = %v, want SPY", data.Ticker)
	}

	if data.Close != 453.50 {
		t.Errorf("Close = %v, want 453.50", data.Close)
	}
}

func TestNewStore(t *testing.T) {
	// NewStore should work with nil db (for testing struct creation)
	store := NewStore(nil)
	if store == nil {
		t.Error("NewStore returned nil")
	}
}
