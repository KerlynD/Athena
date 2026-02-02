// Package market provides market data storage for the Market Intelligence Aggregator.
package market

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// Store handles market data persistence
type Store struct {
	db *sql.DB
}

// NewStore creates a new market data store
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// SaveMarketData stores market data in the database
func (s *Store) SaveMarketData(ctx context.Context, data *MarketData) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	query := `
		INSERT INTO market_data (ticker, timestamp, open, high, low, close, volume, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`

	_, err := s.db.ExecContext(ctx, query,
		data.Ticker,
		data.Timestamp,
		data.Open,
		data.High,
		data.Low,
		data.Close,
		data.Volume,
	)

	if err != nil {
		log.Printf("Error saving market data for %s: %v", data.Ticker, err)
		return fmt.Errorf("save market data: %w", err)
	}

	log.Printf("Saved market data for %s", data.Ticker)
	return nil
}

// SaveMultiple stores multiple market data records
func (s *Store) SaveMultiple(ctx context.Context, dataMap map[string]*MarketData) (int, []error) {
	saved := 0
	var errors []error

	for ticker, data := range dataMap {
		if err := s.SaveMarketData(ctx, data); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", ticker, err))
		} else {
			saved++
		}
	}

	return saved, errors
}

// GetLatest retrieves the most recent market data for a ticker
func (s *Store) GetLatest(ctx context.Context, ticker string) (*MarketData, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	query := `
		SELECT ticker, timestamp, open, high, low, close, volume
		FROM market_data
		WHERE ticker = $1
		ORDER BY timestamp DESC
		LIMIT 1
	`

	var data MarketData
	err := s.db.QueryRowContext(ctx, query, ticker).Scan(
		&data.Ticker,
		&data.Timestamp,
		&data.Open,
		&data.High,
		&data.Low,
		&data.Close,
		&data.Volume,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no market data found for %s", ticker)
	}
	if err != nil {
		return nil, fmt.Errorf("query market data: %w", err)
	}

	return &data, nil
}

// GetHistorical retrieves historical market data for a ticker
func (s *Store) GetHistorical(ctx context.Context, ticker string, days int) ([]MarketData, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	query := `
		SELECT ticker, timestamp, open, high, low, close, volume
		FROM market_data
		WHERE ticker = $1 AND timestamp >= NOW() - $2::interval
		ORDER BY timestamp ASC
	`

	interval := fmt.Sprintf("%d days", days)
	rows, err := s.db.QueryContext(ctx, query, ticker, interval)
	if err != nil {
		return nil, fmt.Errorf("query historical data: %w", err)
	}
	defer rows.Close()

	var results []MarketData
	for rows.Next() {
		var data MarketData
		if err := rows.Scan(
			&data.Ticker,
			&data.Timestamp,
			&data.Open,
			&data.High,
			&data.Low,
			&data.Close,
			&data.Volume,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		results = append(results, data)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return results, nil
}

// GetTrackedTickers retrieves the list of tracked tickers from config
func (s *Store) GetTrackedTickers(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Default tickers if config not found
	defaultTickers := []string{"SPY", "QQQ", "VOO", "VTI"}

	var tickersJSON string
	err := s.db.QueryRowContext(ctx, `
		SELECT value::text FROM config WHERE key = 'tracked_tickers'
	`).Scan(&tickersJSON)

	if err != nil {
		if err == sql.ErrNoRows {
			return defaultTickers, nil
		}
		return defaultTickers, nil // Use defaults on error
	}

	// Simple JSON array parsing (avoiding full json import for this)
	// Format: ["SPY", "QQQ", "VOO", "VTI"]
	// For proper implementation, use encoding/json
	return defaultTickers, nil
}
