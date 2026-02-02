// Package main provides portfolio fetching functionality for the orchestrator.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// PortfolioResult represents the output from the Python portfolio fetcher
type PortfolioResult struct {
	Status        string    `json:"status"`
	HoldingsCount int       `json:"holdings_count"`
	TotalValue    float64   `json:"total_value"`
	TotalCost     float64   `json:"total_cost"`
	TotalGain     float64   `json:"total_gain"`
	GainPercent   float64   `json:"gain_percent"`
	Holdings      []Holding `json:"holdings"`
	Timestamp     string    `json:"timestamp"`
	Message       string    `json:"message,omitempty"`
}

// Holding represents a portfolio position
type Holding struct {
	Ticker       string  `json:"ticker"`
	Quantity     float64 `json:"quantity"`
	AvgCost      float64 `json:"avg_cost"`
	CurrentPrice float64 `json:"current_price"`
	MarketValue  float64 `json:"market_value"`
}

// fetchPortfolio runs the Python script to fetch Robinhood portfolio
func fetchPortfolio(ctx context.Context) error {
	log.Println("=== Fetching Portfolio from Robinhood ===")

	// Check for required environment variables
	if os.Getenv("ROBINHOOD_USERNAME") == "" || os.Getenv("ROBINHOOD_PASSWORD") == "" {
		return fmt.Errorf("ROBINHOOD_USERNAME and ROBINHOOD_PASSWORD must be set")
	}

	pythonPath := getPythonPath()
	scriptPath := "services/robinhood/fetch_portfolio.py"

	// Check if script exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("script not found: %s", scriptPath)
	}

	log.Printf("Running: %s %s", pythonPath, scriptPath)

	// Create command with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, pythonPath, scriptPath)
	cmd.Env = os.Environ()

	output, err := cmd.Output()
	if err != nil {
		// Try to get stderr for better error message
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("portfolio fetch failed: %s\nStderr: %s", err, string(exitErr.Stderr))
		}
		return fmt.Errorf("portfolio fetch failed: %w", err)
	}

	// Parse the JSON output
	var result PortfolioResult
	if err := json.Unmarshal(output, &result); err != nil {
		log.Printf("Raw output: %s", string(output))
		return fmt.Errorf("parse portfolio result: %w", err)
	}

	if result.Status == "error" {
		return fmt.Errorf("portfolio fetch error: %s", result.Message)
	}

	// Display results
	log.Printf("✓ Fetched %d holdings", result.HoldingsCount)
	log.Printf("  Total Value: $%.2f", result.TotalValue)
	log.Printf("  Total Cost:  $%.2f", result.TotalCost)
	log.Printf("  Total Gain:  $%.2f (%.2f%%)", result.TotalGain, result.GainPercent)
	log.Println("")
	log.Println("Holdings:")
	for _, h := range result.Holdings {
		gainPct := 0.0
		if h.AvgCost > 0 {
			gainPct = (h.CurrentPrice - h.AvgCost) / h.AvgCost * 100
		}
		log.Printf("  %s: %.4f shares @ $%.2f (cost: $%.2f, gain: %.2f%%)",
			h.Ticker, h.Quantity, h.CurrentPrice, h.AvgCost, gainPct)
	}

	return nil
}

// showPortfolio displays current portfolio from database
func showPortfolio(ctx context.Context, db *sql.DB) error {
	log.Println("=== Current Portfolio ===")

	rows, err := db.QueryContext(ctx, `
		SELECT ticker, quantity, avg_cost, current_price, market_value, updated_at
		FROM holdings
		ORDER BY market_value DESC
	`)
	if err != nil {
		return fmt.Errorf("query holdings: %w", err)
	}
	defer rows.Close()

	var totalValue, totalCost float64
	count := 0

	fmt.Println("")
	fmt.Printf("%-8s %12s %12s %12s %12s %10s\n",
		"Ticker", "Quantity", "Avg Cost", "Price", "Value", "Gain %")
	fmt.Println("------------------------------------------------------------------------")

	for rows.Next() {
		var ticker string
		var quantity, avgCost, currentPrice, marketValue float64
		var updatedAt time.Time

		if err := rows.Scan(&ticker, &quantity, &avgCost, &currentPrice, &marketValue, &updatedAt); err != nil {
			return fmt.Errorf("scan row: %w", err)
		}

		gainPct := 0.0
		if avgCost > 0 {
			gainPct = (currentPrice - avgCost) / avgCost * 100
		}

		fmt.Printf("%-8s %12.4f %12.2f %12.2f %12.2f %9.2f%%\n",
			ticker, quantity, avgCost, currentPrice, marketValue, gainPct)

		totalValue += marketValue
		totalCost += avgCost * quantity
		count++
	}

	if count == 0 {
		fmt.Println("No holdings found. Run 'orchestrator fetch-portfolio' to sync.")
		return nil
	}

	fmt.Println("------------------------------------------------------------------------")
	totalGain := totalValue - totalCost
	totalGainPct := 0.0
	if totalCost > 0 {
		totalGainPct = totalGain / totalCost * 100
	}
	fmt.Printf("%-8s %12s %12.2f %12s %12.2f %9.2f%%\n",
		"TOTAL", "", totalCost, "", totalValue, totalGainPct)

	return nil
}

// getPythonPath returns the appropriate python command (shared with analyze.go)
func getPythonPathPortfolio() string {
	// Try venv first
	if runtime.GOOS == "windows" {
		if _, err := os.Stat("venv/Scripts/python.exe"); err == nil {
			return "venv/Scripts/python.exe"
		}
	} else {
		if _, err := os.Stat("venv/bin/python"); err == nil {
			return "venv/bin/python"
		}
	}

	// Fall back to system python
	if runtime.GOOS == "windows" {
		return "python"
	}
	return "python3"
}
