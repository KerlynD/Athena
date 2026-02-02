// Package market provides market data fetching from Alpha Vantage API.
// It handles rate limiting, retries, and data parsing for stock quotes.
package market

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	// Alpha Vantage API rate limits
	rateLimitDelay = 15 * time.Second // 5 requests per minute = 12 seconds, using 15 for safety
	requestTimeout = 10 * time.Second
)

// AlphaVantageQuote represents the API response structure
type AlphaVantageQuote struct {
	GlobalQuote struct {
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
	} `json:"Global Quote"`
}

// MarketData represents parsed market data for storage
type MarketData struct {
	Ticker    string
	Timestamp time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    int64
}

// Fetcher handles market data fetching from Alpha Vantage
type Fetcher struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewFetcher creates a new market data fetcher
func NewFetcher() (*Fetcher, error) {
	apiKey := os.Getenv("ALPHAVANTAGE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ALPHAVANTAGE_API_KEY is not set")
	}

	return &Fetcher{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
		baseURL: "https://www.alphavantage.co/query",
	}, nil
}

// FetchQuote fetches the current quote for a ticker
func (f *Fetcher) FetchQuote(ctx context.Context, ticker string) (*MarketData, error) {
	url := fmt.Sprintf(
		"%s?function=GLOBAL_QUOTE&symbol=%s&apikey=%s",
		f.baseURL, ticker, f.apiKey,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	log.Printf("Fetching quote for %s...", ticker)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch quote: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var quote AlphaVantageQuote
	if err := json.Unmarshal(body, &quote); err != nil {
		return nil, fmt.Errorf("unmarshal quote: %w", err)
	}

	// Validate response
	if quote.GlobalQuote.Symbol == "" {
		return nil, fmt.Errorf("empty response for %s - may have hit rate limit", ticker)
	}

	// Parse and convert to MarketData
	data, err := parseQuote(ticker, &quote)
	if err != nil {
		return nil, fmt.Errorf("parse quote: %w", err)
	}

	log.Printf("Fetched %s: Close=%.2f, Volume=%d", ticker, data.Close, data.Volume)
	return data, nil
}

// parseQuote converts API response to MarketData
func parseQuote(ticker string, quote *AlphaVantageQuote) (*MarketData, error) {
	data := &MarketData{
		Ticker:    ticker,
		Timestamp: time.Now(),
	}

	var err error

	data.Open, err = strconv.ParseFloat(quote.GlobalQuote.Open, 64)
	if err != nil {
		return nil, fmt.Errorf("parse open: %w", err)
	}

	data.High, err = strconv.ParseFloat(quote.GlobalQuote.High, 64)
	if err != nil {
		return nil, fmt.Errorf("parse high: %w", err)
	}

	data.Low, err = strconv.ParseFloat(quote.GlobalQuote.Low, 64)
	if err != nil {
		return nil, fmt.Errorf("parse low: %w", err)
	}

	data.Close, err = strconv.ParseFloat(quote.GlobalQuote.Price, 64)
	if err != nil {
		return nil, fmt.Errorf("parse close: %w", err)
	}

	data.Volume, err = strconv.ParseInt(quote.GlobalQuote.Volume, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse volume: %w", err)
	}

	return data, nil
}

// FetchMultiple fetches quotes for multiple tickers with rate limiting
func (f *Fetcher) FetchMultiple(ctx context.Context, tickers []string) (map[string]*MarketData, []error) {
	results := make(map[string]*MarketData)
	var errors []error

	for i, ticker := range tickers {
		// Check context cancellation
		select {
		case <-ctx.Done():
			errors = append(errors, ctx.Err())
			return results, errors
		default:
		}

		data, err := f.FetchQuote(ctx, ticker)
		if err != nil {
			log.Printf("Error fetching %s: %v", ticker, err)
			errors = append(errors, fmt.Errorf("%s: %w", ticker, err))
		} else {
			results[ticker] = data
		}

		// Rate limit delay (skip after last ticker)
		if i < len(tickers)-1 {
			log.Printf("Rate limiting: waiting %v before next request", rateLimitDelay)
			select {
			case <-time.After(rateLimitDelay):
			case <-ctx.Done():
				errors = append(errors, ctx.Err())
				return results, errors
			}
		}
	}

	return results, errors
}

// RateLimitDelay returns the rate limit delay for external use
func RateLimitDelay() time.Duration {
	return rateLimitDelay
}
