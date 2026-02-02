// Package main provides the daily orchestrator for the Market Intelligence Aggregator.
// This is the main entry point that coordinates all data fetching, analysis, and recommendation generation.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"athena/services/market"
	"athena/services/social"
)

func main() {
	// Setup logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Setup context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutdown signal received, cleaning up...")
		cancel()
	}()

	// Validate required environment variables
	if err := validateEnv(); err != nil {
		log.Fatalf("Environment validation failed: %v", err)
	}

	// Connect to database
	db, err := connectDB()
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	// Parse command
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "fetch-portfolio":
		if err := fetchPortfolio(ctx); err != nil {
			log.Fatalf("fetch-portfolio failed: %v", err)
		}
	case "fetch-market":
		if err := fetchMarketData(ctx, db); err != nil {
			log.Fatalf("fetch-market failed: %v", err)
		}
	case "fetch-social":
		if err := fetchSocialContent(ctx, db); err != nil {
			log.Fatalf("fetch-social failed: %v", err)
		}
	case "add-content":
		if err := addContent(ctx, db); err != nil {
			log.Fatalf("add-content failed: %v", err)
		}
	case "add-batch":
		if err := addContentBatch(ctx, db); err != nil {
			log.Fatalf("add-batch failed: %v", err)
		}
	case "list-content":
		if err := listContent(ctx, db, 20); err != nil {
			log.Fatalf("list-content failed: %v", err)
		}
	case "analyze":
		if err := runAnalysis(ctx, db); err != nil {
			log.Fatalf("analyze failed: %v", err)
		}
	case "run-all":
		if err := runAll(ctx, db); err != nil {
			log.Fatalf("run-all failed: %v", err)
		}
	case "status":
		if err := showStatus(ctx, db); err != nil {
			log.Fatalf("status failed: %v", err)
		}
	case "add-holding":
		if err := addHolding(ctx, db); err != nil {
			log.Fatalf("add-holding failed: %v", err)
		}
	case "import-holdings":
		if len(os.Args) < 3 {
			log.Fatalf("Usage: orchestrator import-holdings <csv-file>")
		}
		if err := importHoldingsCSV(ctx, db, os.Args[2]); err != nil {
			log.Fatalf("import-holdings failed: %v", err)
		}
	case "clear-holdings":
		if err := clearHoldings(ctx, db); err != nil {
			log.Fatalf("clear-holdings failed: %v", err)
		}
	case "show-portfolio":
		if err := showPortfolio(ctx, db); err != nil {
			log.Fatalf("show-portfolio failed: %v", err)
		}
	default:
		log.Printf("Unknown command: %s", command)
		printUsage()
		os.Exit(1)
	}

	log.Println("Command completed successfully")
}

func printUsage() {
	fmt.Println(`Usage: orchestrator <command>

Commands:
  fetch-portfolio      Fetch portfolio holdings from Robinhood
  fetch-market         Fetch market data from Alpha Vantage
  
  add-holding          Manually add/update a portfolio holding
  import-holdings      Import holdings from CSV file
  clear-holdings       Remove all holdings from database
  show-portfolio       Display current portfolio

  add-content          Manually add creator content for analysis
  add-batch            Add multiple pieces of content at once
  list-content         Show recent creator content

  analyze              Run analysis and generate recommendations
  run-all              Execute complete daily workflow
  status               Show database status and counts`)
}

func validateEnv() error {
	required := []string{
		"DATABASE_URL",
	}

	// These are optional for testing, but required for actual API calls
	optional := []string{
		"ALPHAVANTAGE_API_KEY",
		"TWITTER_BEARER_TOKEN",
		"ANTHROPIC_API_KEY",
	}

	for _, env := range required {
		if os.Getenv(env) == "" {
			return fmt.Errorf("%s environment variable is not set", env)
		}
	}

	// Log warnings for optional vars
	for _, env := range optional {
		if os.Getenv(env) == "" {
			log.Printf("Warning: %s not set - some features may not work", env)
		}
	}

	return nil
}

func connectDB() (*sql.DB, error) {
	dbURL := os.Getenv("DATABASE_URL")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	log.Println("Database connection established")
	return db, nil
}

func fetchMarketData(ctx context.Context, db *sql.DB) error {
	log.Println("=== Fetching Market Data ===")

	// Check if API key is set
	if os.Getenv("ALPHAVANTAGE_API_KEY") == "" {
		return fmt.Errorf("ALPHAVANTAGE_API_KEY is not set")
	}

	// Create fetcher and store
	fetcher, err := market.NewFetcher()
	if err != nil {
		return fmt.Errorf("create fetcher: %w", err)
	}

	store := market.NewStore(db)

	// Get tickers to fetch
	tickers := getTrackedTickers()
	log.Printf("Fetching data for %d tickers: %v", len(tickers), tickers)

	// Fetch and store data
	dataMap, fetchErrors := fetcher.FetchMultiple(ctx, tickers)

	// Log any fetch errors
	for _, err := range fetchErrors {
		log.Printf("Fetch error: %v", err)
	}

	// Store successful fetches
	saved, storeErrors := store.SaveMultiple(ctx, dataMap)

	// Log any store errors
	for _, err := range storeErrors {
		log.Printf("Store error: %v", err)
	}

	log.Printf("Fetched %d tickers, saved %d to database", len(dataMap), saved)

	if saved == 0 && len(tickers) > 0 {
		return fmt.Errorf("failed to save any market data")
	}

	return nil
}

func fetchSocialContent(ctx context.Context, db *sql.DB) error {
	log.Println("=== Fetching Social Content ===")

	// Check if API key is set
	if os.Getenv("TWITTER_BEARER_TOKEN") == "" {
		return fmt.Errorf("TWITTER_BEARER_TOKEN is not set")
	}

	// Create client and store
	client, err := social.NewClient()
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	store := social.NewStore(db)

	// Get creators to fetch
	creators := getCreators()
	log.Printf("Fetching tweets from %d creators: %v", len(creators), creators)

	totalSaved := 0

	for _, creator := range creators {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Fetch tweets
		tweets, err := client.FetchRecentTweets(ctx, creator, 10)
		if err != nil {
			log.Printf("Error fetching from @%s: %v", creator, err)
			continue
		}

		// Store tweets
		saved, storeErrors := store.SaveTweets(ctx, creator, tweets)
		for _, err := range storeErrors {
			log.Printf("Store error for @%s: %v", creator, err)
		}

		totalSaved += saved
		log.Printf("Saved %d tweets from @%s", saved, creator)

		// Rate limit between creators
		if creator != creators[len(creators)-1] {
			time.Sleep(social.RateLimitDelay())
		}
	}

	log.Printf("Total: saved %d tweets from %d creators", totalSaved, len(creators))
	return nil
}

func runAnalysis(ctx context.Context, db *sql.DB) error {
	return runFullAnalysis(ctx, db)
}

func runAll(ctx context.Context, db *sql.DB) error {
	log.Println("=== Market Intelligence Daily Run ===")
	log.Printf("Started at: %s", time.Now().Format(time.RFC3339))

	// Step 1: Fetch portfolio from Robinhood
	log.Println("\n--- Step 1/3: Fetching portfolio ---")
	if os.Getenv("ROBINHOOD_USERNAME") != "" {
		if err := fetchPortfolio(ctx); err != nil {
			log.Printf("Warning: portfolio fetch failed: %v", err)
			// Continue anyway - other steps may still work
		}
	} else {
		log.Println("Skipping portfolio fetch (ROBINHOOD_USERNAME not set)")
	}

	// Step 2: Fetch market data
	log.Println("\n--- Step 2/3: Fetching market data ---")
	if err := fetchMarketData(ctx, db); err != nil {
		log.Printf("Warning: market data fetch failed: %v", err)
		// Continue anyway - other steps may still work
	}

	// Step 3: Run analysis (social content is added manually via add-content)
	log.Println("\n--- Step 3/3: Running analysis ---")
	if err := runAnalysis(ctx, db); err != nil {
		return fmt.Errorf("run analysis: %w", err)
	}

	log.Printf("\nCompleted at: %s", time.Now().Format(time.RFC3339))
	return nil
}

func showStatus(ctx context.Context, db *sql.DB) error {
	log.Println("=== Database Status ===")

	// Holdings
	var holdingsCount int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM holdings").Scan(&holdingsCount)
	log.Printf("Holdings: %d", holdingsCount)

	// Market data
	var marketCount int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM market_data").Scan(&marketCount)
	log.Printf("Market data records: %d", marketCount)

	// Latest market data per ticker
	rows, err := db.QueryContext(ctx, `
		SELECT ticker, MAX(timestamp) as latest
		FROM market_data
		GROUP BY ticker
		ORDER BY ticker
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var ticker string
			var latest time.Time
			rows.Scan(&ticker, &latest)
			log.Printf("  %s: %s", ticker, latest.Format("2006-01-02 15:04"))
		}
	}

	// Creator content
	var contentCount int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM creator_content").Scan(&contentCount)
	log.Printf("Creator content: %d", contentCount)

	// Content with embeddings
	var embeddingCount int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM creator_content WHERE embedding IS NOT NULL").Scan(&embeddingCount)
	log.Printf("Content with embeddings: %d", embeddingCount)

	// Technical indicators
	var indicatorCount int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM technical_indicators").Scan(&indicatorCount)
	log.Printf("Technical indicators: %d", indicatorCount)

	// Signals
	var signalCount int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM signals").Scan(&signalCount)
	log.Printf("Signals: %d", signalCount)

	return nil
}

func getTrackedTickers() []string {
	tickersStr := os.Getenv("TRACKED_TICKERS")
	if tickersStr == "" {
		return []string{"SPY", "QQQ", "VOO", "VTI"}
	}

	tickers := strings.Split(tickersStr, ",")
	for i := range tickers {
		tickers[i] = strings.TrimSpace(tickers[i])
	}
	return tickers
}

func getCreators() []string {
	creatorsStr := os.Getenv("CREATORS")
	if creatorsStr == "" {
		return []string{"mobyinvest", "carbonfinance"}
	}

	creators := strings.Split(creatorsStr, ",")
	for i := range creators {
		creators[i] = strings.TrimSpace(creators[i])
	}
	return creators
}
