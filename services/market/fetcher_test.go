package market

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseQuote(t *testing.T) {
	tests := []struct {
		name    string
		ticker  string
		quote   *AlphaVantageQuote
		wantErr bool
	}{
		{
			name:   "valid quote",
			ticker: "SPY",
			quote: &AlphaVantageQuote{
				GlobalQuote: struct {
					Symbol           string `json:"01. symbol"`
					Open             string `json:"02. open"`
					High             string `json:"03. high"`
					Low              string `json:"04. low"`
					Price            string `json:"05. price"`
					Volume           string `json:"06. volume"`
					LatestTradingDay string `json:"07. latest trading day"`
					PreviousClose    string `json:"08. previous close"`
					Change           string `json:"09. change"`
					ChangePercent    string `json:"10. change percent"`
				}{
					Symbol:           "SPY",
					Open:             "450.00",
					High:             "455.00",
					Low:              "449.00",
					Price:            "453.50",
					Volume:           "50000000",
					LatestTradingDay: "2024-01-30",
					PreviousClose:    "451.00",
					Change:           "2.50",
					ChangePercent:    "0.55%",
				},
			},
			wantErr: false,
		},
		{
			name:   "invalid open price",
			ticker: "SPY",
			quote: &AlphaVantageQuote{
				GlobalQuote: struct {
					Symbol           string `json:"01. symbol"`
					Open             string `json:"02. open"`
					High             string `json:"03. high"`
					Low              string `json:"04. low"`
					Price            string `json:"05. price"`
					Volume           string `json:"06. volume"`
					LatestTradingDay string `json:"07. latest trading day"`
					PreviousClose    string `json:"08. previous close"`
					Change           string `json:"09. change"`
					ChangePercent    string `json:"10. change percent"`
				}{
					Symbol: "SPY",
					Open:   "invalid",
					High:   "455.00",
					Low:    "449.00",
					Price:  "453.50",
					Volume: "50000000",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := parseQuote(tt.ticker, tt.quote)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseQuote() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && data.Ticker != tt.ticker {
				t.Errorf("parseQuote() ticker = %v, want %v", data.Ticker, tt.ticker)
			}
		})
	}
}

func TestFetchQuote_MockServer(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := AlphaVantageQuote{
			GlobalQuote: struct {
				Symbol           string `json:"01. symbol"`
				Open             string `json:"02. open"`
				High             string `json:"03. high"`
				Low              string `json:"04. low"`
				Price            string `json:"05. price"`
				Volume           string `json:"06. volume"`
				LatestTradingDay string `json:"07. latest trading day"`
				PreviousClose    string `json:"08. previous close"`
				Change           string `json:"09. change"`
				ChangePercent    string `json:"10. change percent"`
			}{
				Symbol: "SPY",
				Open:   "450.00",
				High:   "455.00",
				Low:    "449.00",
				Price:  "453.50",
				Volume: "50000000",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create fetcher with mock server
	fetcher := &Fetcher{
		apiKey:  "test_key",
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := fetcher.FetchQuote(ctx, "SPY")
	if err != nil {
		t.Fatalf("FetchQuote() error = %v", err)
	}

	if data.Ticker != "SPY" {
		t.Errorf("FetchQuote() ticker = %v, want SPY", data.Ticker)
	}

	if data.Close != 453.50 {
		t.Errorf("FetchQuote() close = %v, want 453.50", data.Close)
	}

	if data.Volume != 50000000 {
		t.Errorf("FetchQuote() volume = %v, want 50000000", data.Volume)
	}
}

func TestRateLimitDelay(t *testing.T) {
	delay := RateLimitDelay()
	if delay != 15*time.Second {
		t.Errorf("RateLimitDelay() = %v, want 15s", delay)
	}
}
