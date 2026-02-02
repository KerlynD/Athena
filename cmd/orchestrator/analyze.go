// Package main provides the analysis pipeline for the orchestrator.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"

	"athena/services/analysis"
	"athena/services/engine"
)

// runFullAnalysis executes the complete analysis pipeline
func runFullAnalysis(ctx context.Context, db *sql.DB) error {
	log.Println("=== Running Full Analysis Pipeline ===")

	// Step 1: Run Python technical indicators (if we have market data)
	log.Println("\n--- Step 1: Technical Indicators ---")
	if err := runPythonIndicators(ctx); err != nil {
		log.Printf("Warning: Technical indicators failed: %v", err)
		log.Println("Continuing without technical indicators...")
	}

	// Step 2: Run Python embeddings (if we have content)
	log.Println("\n--- Step 2: Embeddings Generation ---")
	if err := runPythonEmbeddings(ctx); err != nil {
		log.Printf("Warning: Embeddings generation failed: %v", err)
		log.Println("Continuing without embeddings...")
	}

	// Step 3: Run Claude sentiment analysis on unanalyzed content
	log.Println("\n--- Step 3: Sentiment Analysis ---")
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		if err := runSentimentAnalysis(ctx, db); err != nil {
			log.Printf("Warning: Sentiment analysis failed: %v", err)
		}
	} else {
		log.Println("Skipping sentiment analysis (ANTHROPIC_API_KEY not set)")
	}

	// Step 4: Generate recommendations
	log.Println("\n--- Step 4: Generate Recommendations ---")
	if err := generateRecommendations(ctx, db); err != nil {
		return fmt.Errorf("generate recommendations: %w", err)
	}

	return nil
}

// runPythonIndicators executes the Python technical indicators script
func runPythonIndicators(ctx context.Context) error {
	pythonPath := getPythonPath()
	scriptPath := "services/analysis/indicators.py"

	// Check if script exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("script not found: %s", scriptPath)
	}

	// Check if we have market data
	log.Println("Running technical indicators calculation...")

	cmd := exec.CommandContext(ctx, pythonPath, scriptPath)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run indicators: %w", err)
	}

	log.Println("✓ Technical indicators calculated")
	return nil
}

// runPythonEmbeddings executes the Python embeddings generation script
func runPythonEmbeddings(ctx context.Context) error {
	pythonPath := getPythonPath()
	scriptPath := "services/analysis/embeddings.py"

	// Check if script exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("script not found: %s", scriptPath)
	}

	log.Println("Running embeddings generation...")

	cmd := exec.CommandContext(ctx, pythonPath, scriptPath)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run embeddings: %w", err)
	}

	log.Println("✓ Embeddings generated")
	return nil
}

// runSentimentAnalysis analyzes unanalyzed content using Claude
func runSentimentAnalysis(ctx context.Context, db *sql.DB) error {
	// Get content without sentiment
	rows, err := db.QueryContext(ctx, `
		SELECT id, creator_name, content_text, mentioned_tickers
		FROM creator_content
		WHERE sentiment IS NULL
		ORDER BY created_at DESC
		LIMIT 20
	`)
	if err != nil {
		return fmt.Errorf("query content: %w", err)
	}
	defer rows.Close()

	type contentItem struct {
		ID       int
		Creator  string
		Text     string
		Tickers  []string
	}

	var items []contentItem
	for rows.Next() {
		var item contentItem
		var tickers pq.StringArray
		if err := rows.Scan(&item.ID, &item.Creator, &item.Text, &tickers); err != nil {
			return fmt.Errorf("scan row: %w", err)
		}
		item.Tickers = []string(tickers)
		items = append(items, item)
	}

	if len(items) == 0 {
		log.Println("No content to analyze")
		return nil
	}

	log.Printf("Analyzing %d content items...", len(items))

	// Create sentiment analyzer
	analyzer, err := analysis.NewAnalyzer()
	if err != nil {
		return fmt.Errorf("create analyzer: %w", err)
	}

	// Group content by ticker for analysis
	contentByTicker := make(map[string][]string)
	for _, item := range items {
		for _, ticker := range item.Tickers {
			contentByTicker[ticker] = append(contentByTicker[ticker], item.Text)
		}
	}

	// Analyze each ticker
	for ticker, content := range contentByTicker {
		if len(content) == 0 {
			continue
		}

		// Get market context
		marketContext := getMarketContext(ctx, db, ticker)

		result, err := analyzer.AnalyzeSentiment(ctx, ticker, content, marketContext)
		if err != nil {
			log.Printf("Error analyzing %s: %v", ticker, err)
			continue
		}

		log.Printf("✓ %s: %s (%.0f%% confidence)", ticker, result.Sentiment, result.Confidence*100)

		// Update content with sentiment
		for _, item := range items {
			for _, t := range item.Tickers {
				if t == ticker {
					_, err := db.ExecContext(ctx, `
						UPDATE creator_content
						SET sentiment = $1, confidence_score = $2
						WHERE id = $3
					`, result.Sentiment, result.Confidence, item.ID)
					if err != nil {
						log.Printf("Error updating content %d: %v", item.ID, err)
					}
				}
			}
		}

		// Rate limit between API calls
		time.Sleep(1 * time.Second)
	}

	return nil
}

// getMarketContext builds market context string for sentiment analysis
func getMarketContext(ctx context.Context, db *sql.DB, ticker string) string {
	var close, rsi sql.NullFloat64
	var sma50, sma200 sql.NullFloat64

	// Get latest price
	db.QueryRowContext(ctx, `
		SELECT close FROM market_data
		WHERE ticker = $1
		ORDER BY timestamp DESC LIMIT 1
	`, ticker).Scan(&close)

	// Get technical indicators
	db.QueryRowContext(ctx, `
		SELECT rsi_14, sma_50, sma_200 FROM technical_indicators
		WHERE ticker = $1
		ORDER BY timestamp DESC LIMIT 1
	`, ticker).Scan(&rsi, &sma50, &sma200)

	context := fmt.Sprintf("Current price: $%.2f", close.Float64)

	if rsi.Valid {
		context += fmt.Sprintf(", RSI(14): %.1f", rsi.Float64)
		if rsi.Float64 > 70 {
			context += " (overbought)"
		} else if rsi.Float64 < 30 {
			context += " (oversold)"
		}
	}

	if sma50.Valid && sma200.Valid {
		if sma50.Float64 > sma200.Float64 {
			context += ", Golden cross (bullish trend)"
		} else {
			context += ", Death cross (bearish trend)"
		}
	}

	return context
}

// generateRecommendations creates investment recommendations using Claude AI
func generateRecommendations(ctx context.Context, db *sql.DB) error {
	// Get contribution budget from config or environment
	budget := 1000.0 // Default monthly contribution
	
	// Try to get from environment variable
	if budgetStr := os.Getenv("MONTHLY_CONTRIBUTION"); budgetStr != "" {
		if b, err := strconv.ParseFloat(budgetStr, 64); err == nil {
			budget = b
		}
	}

	log.Printf("Generating Claude-powered recommendations for $%.2f budget...", budget)

	// Create Claude-powered recommendation engine
	claudeEng, err := engine.NewClaudeEngine(db)
	if err != nil {
		// Fall back to basic engine if Claude isn't available
		log.Printf("Claude engine unavailable (%v), using basic engine", err)
		return generateBasicRecommendations(ctx, db, budget)
	}

	recommendations, err := claudeEng.GenerateRecommendations(ctx, budget)
	if err != nil {
		log.Printf("Claude recommendation failed (%v), falling back to basic", err)
		return generateBasicRecommendations(ctx, db, budget)
	}

	// Display Claude's recommendations
	log.Println("\n" + strings.Repeat("=", 60))
	log.Println("CLAUDE AI RECOMMENDATIONS")
	log.Println(strings.Repeat("=", 60))
	
	log.Printf("\n📊 Market Assessment: %s\n", recommendations.MarketAssessment)
	log.Printf("📋 Strategy: %s\n", recommendations.OverallStrategy)
	
	log.Println("\n" + strings.Repeat("-", 60))
	log.Println("ALLOCATIONS:")
	log.Println(strings.Repeat("-", 60))

	for _, rec := range recommendations.Recommendations {
		emoji := "📈"
		switch rec.Action {
		case "hold":
			emoji = "⏸️"
		case "sell":
			emoji = "📉"
		case "wait":
			emoji = "⏳"
		}

		log.Printf("%s [Priority %d] %s: %s $%.2f (%.0f%% confidence)",
			emoji, rec.Priority, rec.Ticker, strings.ToUpper(rec.Action), rec.Amount, rec.Confidence*100)
		log.Printf("   → %s", rec.Reasoning)
	}

	log.Println(strings.Repeat("-", 60))
	log.Printf("💰 Total Allocated: $%.2f", recommendations.TotalAllocated)
	if recommendations.CashToHold > 0 {
		log.Printf("💵 Cash to Hold: $%.2f", recommendations.CashToHold)
	}
	log.Println(strings.Repeat("=", 60))

	// Store recommendations
	if err := claudeEng.StoreRecommendations(ctx, recommendations); err != nil {
		log.Printf("Warning: could not store recommendations: %v", err)
	}

	return nil
}

// generateBasicRecommendations falls back to the basic engine without Claude
func generateBasicRecommendations(ctx context.Context, db *sql.DB, budget float64) error {
	eng := engine.NewEngine(db, engine.DefaultConfig())

	recommendations, err := eng.GenerateRecommendations(ctx, budget)
	if err != nil {
		return fmt.Errorf("generate recommendations: %w", err)
	}

	log.Println("\n=== BASIC RECOMMENDATIONS (Claude unavailable) ===")
	for _, rec := range recommendations {
		if rec.Action == "wait" {
			log.Printf("⏳ WAIT: %s", rec.Reasoning)
			continue
		}

		log.Printf("📈 %s: %s $%.2f (%.0f%% confidence)",
			rec.Ticker, rec.Action, rec.Amount, rec.ConfidenceScore*100)
		log.Printf("   Reason: %s", rec.Reasoning)

		if err := eng.StoreRecommendation(ctx, rec); err != nil {
			log.Printf("Error storing recommendation: %v", err)
		}
	}

	return nil
}

// getPythonPath returns the appropriate python command
func getPythonPath() string {
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
