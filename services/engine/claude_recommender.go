// Package engine provides Claude-powered recommendations.
// This sends all available context to Claude for intelligent investment recommendations.
package engine

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	claudeAPIURL   = "https://api.anthropic.com/v1/messages"
	claudeModel    = "claude-sonnet-4-20250514"
	claudeVersion  = "2023-06-01"
	maxTokens      = 4000
	requestTimeout = 60 * time.Second
)

// ClaudeRecommendation represents a single recommendation from Claude
type ClaudeRecommendation struct {
	Ticker     string  `json:"ticker"`
	Action     string  `json:"action"` // buy, hold, sell, wait
	Amount     float64 `json:"amount"` // Dollar amount to allocate
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
	Priority   int     `json:"priority"` // 1 = highest priority
}

// ClaudeRecommendations represents the full response
type ClaudeRecommendations struct {
	MarketAssessment string                 `json:"market_assessment"`
	Recommendations  []ClaudeRecommendation `json:"recommendations"`
	TotalAllocated   float64                `json:"total_allocated"`
	CashToHold       float64                `json:"cash_to_hold"`
	OverallStrategy  string                 `json:"overall_strategy"`
}

// PortfolioHolding represents a current holding
type PortfolioHolding struct {
	Ticker       string
	Quantity     float64
	AvgCost      float64
	CurrentPrice float64
	MarketValue  float64
	GainPercent  float64
}

// MarketDataPoint represents current market data
type MarketDataPoint struct {
	Ticker string
	Price  float64
	Volume int64
}

// TechnicalData represents technical indicators
type TechnicalData struct {
	Ticker     string
	RSI        float64
	SMA50      float64
	SMA200     float64
	MACD       float64
	MACDSignal float64
}

// ContentItem represents creator content
type ContentItem struct {
	Creator   string
	Content   string
	Sentiment string
	Tickers   []string
}

// ClaudeEngine uses Claude for intelligent recommendations
type ClaudeEngine struct {
	db         *sql.DB
	apiKey     string
	httpClient *http.Client
}

// NewClaudeEngine creates a new Claude-powered engine
func NewClaudeEngine(db *sql.DB) (*ClaudeEngine, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set")
	}

	return &ClaudeEngine{
		db:     db,
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
	}, nil
}

// GenerateRecommendations gathers all context and asks Claude for recommendations
func (e *ClaudeEngine) GenerateRecommendations(ctx context.Context, budget float64) (*ClaudeRecommendations, error) {
	log.Println("Gathering context for Claude analysis...")

	// 1. Get current portfolio holdings
	holdings, err := e.getPortfolioHoldings(ctx)
	if err != nil {
		log.Printf("Warning: could not get holdings: %v", err)
	}
	log.Printf("Found %d current holdings", len(holdings))

	// 2. Get recent market data
	marketData, err := e.getMarketData(ctx)
	if err != nil {
		log.Printf("Warning: could not get market data: %v", err)
	}
	log.Printf("Found market data for %d tickers", len(marketData))

	// 3. Get technical indicators
	technicals, err := e.getTechnicalIndicators(ctx)
	if err != nil {
		log.Printf("Warning: could not get technical indicators: %v", err)
	}
	log.Printf("Found technical data for %d tickers", len(technicals))

	// 4. Get creator content
	content, err := e.getCreatorContent(ctx)
	if err != nil {
		log.Printf("Warning: could not get creator content: %v", err)
	}
	log.Printf("Found %d content items from creators", len(content))

	// 5. Build prompt and call Claude
	prompt := e.buildPrompt(holdings, marketData, technicals, content, budget)
	
	recommendations, err := e.callClaude(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("call Claude: %w", err)
	}

	return recommendations, nil
}

func (e *ClaudeEngine) getPortfolioHoldings(ctx context.Context) ([]PortfolioHolding, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT ticker, quantity, avg_cost, current_price, market_value
		FROM holdings
		ORDER BY market_value DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var holdings []PortfolioHolding
	for rows.Next() {
		var h PortfolioHolding
		if err := rows.Scan(&h.Ticker, &h.Quantity, &h.AvgCost, &h.CurrentPrice, &h.MarketValue); err != nil {
			continue
		}
		if h.AvgCost > 0 {
			h.GainPercent = (h.CurrentPrice - h.AvgCost) / h.AvgCost * 100
		}
		holdings = append(holdings, h)
	}
	return holdings, nil
}

func (e *ClaudeEngine) getMarketData(ctx context.Context) ([]MarketDataPoint, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT DISTINCT ON (ticker) ticker, close, volume
		FROM market_data
		ORDER BY ticker, timestamp DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []MarketDataPoint
	for rows.Next() {
		var d MarketDataPoint
		if err := rows.Scan(&d.Ticker, &d.Price, &d.Volume); err != nil {
			continue
		}
		data = append(data, d)
	}
	return data, nil
}

func (e *ClaudeEngine) getTechnicalIndicators(ctx context.Context) ([]TechnicalData, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT DISTINCT ON (ticker) ticker, 
			COALESCE(rsi_14, 0), COALESCE(sma_50, 0), COALESCE(sma_200, 0),
			COALESCE(macd, 0), COALESCE(macd_signal, 0)
		FROM technical_indicators
		ORDER BY ticker, timestamp DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []TechnicalData
	for rows.Next() {
		var d TechnicalData
		if err := rows.Scan(&d.Ticker, &d.RSI, &d.SMA50, &d.SMA200, &d.MACD, &d.MACDSignal); err != nil {
			continue
		}
		data = append(data, d)
	}
	return data, nil
}

func (e *ClaudeEngine) getCreatorContent(ctx context.Context) ([]ContentItem, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT creator_name, content_text, COALESCE(sentiment, 'unknown'), mentioned_tickers
		FROM creator_content
		WHERE created_at >= NOW() - INTERVAL '7 days'
		ORDER BY created_at DESC
		LIMIT 20
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ContentItem
	for rows.Next() {
		var item ContentItem
		var tickers []string
		if err := rows.Scan(&item.Creator, &item.Content, &item.Sentiment, &tickers); err != nil {
			continue
		}
		item.Tickers = tickers
		items = append(items, item)
	}
	return items, nil
}

func (e *ClaudeEngine) buildPrompt(holdings []PortfolioHolding, marketData []MarketDataPoint, technicals []TechnicalData, content []ContentItem, budget float64) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`You are an expert investment advisor helping manage a Roth IRA portfolio. 
I have $%.2f to allocate this month.

## MY INVESTMENT GOALS:
- Long-term growth in a Roth IRA (tax-free growth)
- Focus on quality companies with strong fundamentals
- Mix of ETFs for stability and individual stocks for growth
- I'm interested in tech, growth stocks, and market leaders
- I want data-driven recommendations based on the context I provide

## MY CURRENT PORTFOLIO:
`, budget))

	if len(holdings) == 0 {
		sb.WriteString("No current holdings on file.\n")
	} else {
		sb.WriteString("| Ticker | Shares | Avg Cost | Current Price | Value | Gain % |\n")
		sb.WriteString("|--------|--------|----------|---------------|-------|--------|\n")
		for _, h := range holdings {
			sb.WriteString(fmt.Sprintf("| %s | %.4f | $%.2f | $%.2f | $%.2f | %.2f%% |\n",
				h.Ticker, h.Quantity, h.AvgCost, h.CurrentPrice, h.MarketValue, h.GainPercent))
		}
	}

	sb.WriteString("\n## MARKET DATA (Recent Prices):\n")
	if len(marketData) == 0 {
		sb.WriteString("No market data available.\n")
	} else {
		for _, d := range marketData {
			sb.WriteString(fmt.Sprintf("- %s: $%.2f (Vol: %d)\n", d.Ticker, d.Price, d.Volume))
		}
	}

	sb.WriteString("\n## TECHNICAL INDICATORS:\n")
	if len(technicals) == 0 {
		sb.WriteString("No technical data available (need more historical data).\n")
	} else {
		hasData := false
		for _, t := range technicals {
			if t.RSI > 0 || t.SMA50 > 0 {
				hasData = true
				sb.WriteString(fmt.Sprintf("- %s: RSI=%.1f, SMA50=%.2f, SMA200=%.2f, MACD=%.4f\n",
					t.Ticker, t.RSI, t.SMA50, t.SMA200, t.MACD))
			}
		}
		if !hasData {
			sb.WriteString("Technical indicators require more historical data (14+ days for RSI).\n")
		}
	}

	sb.WriteString("\n## CREATOR INSIGHTS (from market analysts I follow):\n")
	if len(content) == 0 {
		sb.WriteString("No recent creator content available.\n")
	} else {
		for i, c := range content {
			text := c.Content
			if len(text) > 300 {
				text = text[:300] + "..."
			}
			sb.WriteString(fmt.Sprintf("%d. @%s [%s]: %s\n", i+1, c.Creator, c.Sentiment, text))
			if len(c.Tickers) > 0 {
				sb.WriteString(fmt.Sprintf("   Mentioned: %v\n", c.Tickers))
			}
		}
	}

	sb.WriteString(fmt.Sprintf(`
## YOUR TASK:
Based on ALL the above context, provide investment recommendations for my $%.2f monthly contribution.

Consider:
1. My current portfolio composition and any gaps
2. Current market conditions and valuations
3. Technical indicators (if available)
4. Creator sentiment and insights
5. General market knowledge and fundamentals

You can recommend ANY ticker - not just ones I already own. Focus on:
- ETFs like SPY, QQQ, VOO for core stability
- Quality tech stocks (AAPL, MSFT, GOOG, NVDA, etc.)
- Any other opportunities you see fit

## RESPONSE FORMAT:
Respond with ONLY valid JSON (no markdown code blocks, no explanation outside JSON):
{
    "market_assessment": "Brief 1-2 sentence assessment of current market conditions",
    "recommendations": [
        {
            "ticker": "SYMBOL",
            "action": "buy|hold|sell|wait",
            "amount": 123.45,
            "confidence": 0.85,
            "reasoning": "Why this recommendation",
            "priority": 1
        }
    ],
    "total_allocated": 1000.00,
    "cash_to_hold": 0.00,
    "overall_strategy": "Brief strategy summary"
}

Important: Recommendations should add up to the budget ($%.2f) unless you recommend holding some cash.
`, budget, budget))

	return sb.String()
}

func (e *ClaudeEngine) callClaude(ctx context.Context, prompt string) (*ClaudeRecommendations, error) {
	log.Println("Calling Claude for investment recommendations...")

	reqBody := struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{
		Model:     claudeModel,
		MaxTokens: maxTokens,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "user", Content: prompt},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", claudeAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", e.apiKey)
	req.Header.Set("anthropic-version", claudeVersion)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var claudeResp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if claudeResp.Error != nil {
		return nil, fmt.Errorf("Claude error: %s", claudeResp.Error.Message)
	}

	if len(claudeResp.Content) == 0 {
		return nil, fmt.Errorf("empty response from Claude")
	}

	// Parse the JSON response
	responseText := strings.TrimSpace(claudeResp.Content[0].Text)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	var recommendations ClaudeRecommendations
	if err := json.Unmarshal([]byte(responseText), &recommendations); err != nil {
		log.Printf("Claude response: %s", responseText)
		return nil, fmt.Errorf("parse recommendations: %w", err)
	}

	return &recommendations, nil
}

// StoreRecommendations saves Claude's recommendations to the database
func (e *ClaudeEngine) StoreRecommendations(ctx context.Context, recs *ClaudeRecommendations) error {
	for _, rec := range recs.Recommendations {
		_, err := e.db.ExecContext(ctx, `
			INSERT INTO signals 
			(ticker, signal_type, recommendation_amount, confidence_score, reasoning, market_regime, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, NOW())
		`, rec.Ticker, rec.Action, rec.Amount, rec.Confidence, rec.Reasoning, recs.MarketAssessment)

		if err != nil {
			log.Printf("Error storing recommendation for %s: %v", rec.Ticker, err)
		}
	}
	return nil
}
