// Package main provides manual holdings input functionality for the orchestrator.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// addHolding handles interactive manual holding input
func addHolding(ctx context.Context, db *sql.DB) error {
	log.Println("=== Add Manual Holding ===")
	log.Println("Manually add or update a portfolio holding.")
	log.Println("Use this for Roth IRA positions that can't be fetched automatically.")
	log.Println("")

	reader := bufio.NewReader(os.Stdin)

	// Get ticker
	fmt.Print("Ticker symbol (e.g., AAPL): ")
	ticker, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read ticker: %w", err)
	}
	ticker = strings.TrimSpace(strings.ToUpper(ticker))
	if ticker == "" {
		return fmt.Errorf("ticker cannot be empty")
	}

	// Get quantity
	fmt.Print("Quantity (number of shares): ")
	qtyStr, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read quantity: %w", err)
	}
	quantity, err := strconv.ParseFloat(strings.TrimSpace(qtyStr), 64)
	if err != nil {
		return fmt.Errorf("invalid quantity: %w", err)
	}

	// Get average cost (optional)
	fmt.Print("Average cost per share (or press Enter to skip): ")
	costStr, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read cost: %w", err)
	}
	costStr = strings.TrimSpace(costStr)
	avgCost := 0.0
	if costStr != "" {
		avgCost, err = strconv.ParseFloat(costStr, 64)
		if err != nil {
			return fmt.Errorf("invalid cost: %w", err)
		}
	}

	// Get current price (optional)
	fmt.Print("Current price per share (or press Enter to skip): ")
	priceStr, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read price: %w", err)
	}
	priceStr = strings.TrimSpace(priceStr)
	currentPrice := 0.0
	if priceStr != "" {
		currentPrice, err = strconv.ParseFloat(priceStr, 64)
		if err != nil {
			return fmt.Errorf("invalid price: %w", err)
		}
	}

	// Calculate market value
	marketValue := quantity * currentPrice

	// Store or update in database
	err = upsertHolding(ctx, db, ticker, quantity, avgCost, currentPrice, marketValue)
	if err != nil {
		return fmt.Errorf("store holding: %w", err)
	}

	log.Printf("\n✓ Saved %s: %.4f shares @ $%.2f = $%.2f", ticker, quantity, currentPrice, marketValue)
	log.Println("Run 'orchestrator status' to see all holdings.")

	return nil
}

// upsertHolding inserts or updates a holding in the database
func upsertHolding(ctx context.Context, db *sql.DB, ticker string, quantity, avgCost, currentPrice, marketValue float64) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Use UPSERT - if ticker exists, update it; otherwise insert
	query := `
		INSERT INTO holdings (ticker, quantity, avg_cost, current_price, market_value, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (ticker) 
		DO UPDATE SET 
			quantity = EXCLUDED.quantity,
			avg_cost = EXCLUDED.avg_cost,
			current_price = EXCLUDED.current_price,
			market_value = EXCLUDED.market_value,
			updated_at = NOW()
	`

	_, err := db.ExecContext(ctx, query, ticker, quantity, avgCost, currentPrice, marketValue)
	if err != nil {
		// If ON CONFLICT fails (no unique constraint), try delete + insert
		if strings.Contains(err.Error(), "ON CONFLICT") {
			log.Println("Using fallback insert method...")
			
			// Delete existing
			_, _ = db.ExecContext(ctx, "DELETE FROM holdings WHERE ticker = $1", ticker)
			
			// Insert new
			insertQuery := `
				INSERT INTO holdings (ticker, quantity, avg_cost, current_price, market_value, updated_at)
				VALUES ($1, $2, $3, $4, $5, NOW())
			`
			_, err = db.ExecContext(ctx, insertQuery, ticker, quantity, avgCost, currentPrice, marketValue)
		}
		if err != nil {
			return fmt.Errorf("upsert holding: %w", err)
		}
	}

	return nil
}

// importHoldingsCSV imports holdings from a CSV file
func importHoldingsCSV(ctx context.Context, db *sql.DB, filepath string) error {
	log.Printf("=== Import Holdings from CSV ===")
	log.Printf("Reading from: %s", filepath)

	file, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	imported := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip header and empty lines
		if lineNum == 1 || line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Expected format: ticker,quantity,avg_cost,current_price
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			log.Printf("Skipping line %d: invalid format", lineNum)
			continue
		}

		ticker := strings.TrimSpace(strings.ToUpper(parts[0]))
		quantity, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			log.Printf("Skipping line %d: invalid quantity", lineNum)
			continue
		}

		avgCost := 0.0
		if len(parts) > 2 {
			avgCost, _ = strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		}

		currentPrice := 0.0
		if len(parts) > 3 {
			currentPrice, _ = strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
		}

		marketValue := quantity * currentPrice

		if err := upsertHolding(ctx, db, ticker, quantity, avgCost, currentPrice, marketValue); err != nil {
			log.Printf("Error importing %s: %v", ticker, err)
			continue
		}

		imported++
		log.Printf("  ✓ %s: %.4f shares", ticker, quantity)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	log.Printf("\n=== Imported %d holdings ===", imported)
	return nil
}

// clearHoldings removes all holdings from the database
func clearHoldings(ctx context.Context, db *sql.DB) error {
	log.Println("=== Clear All Holdings ===")
	
	result, err := db.ExecContext(ctx, "DELETE FROM holdings")
	if err != nil {
		return fmt.Errorf("delete holdings: %w", err)
	}

	rows, _ := result.RowsAffected()
	log.Printf("Deleted %d holdings", rows)
	return nil
}
