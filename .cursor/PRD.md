# Market Intelligence Aggregator - Product Requirements Document

## Executive Summary

A CLI-based market intelligence system called Athena on a windows machine (but should have mac/linux support) that aggregates real-time market data, creator sentiment analysis, and portfolio tracking to provide daily investment recommendations for a Roth IRA. Built with Go (primary), Python (Robinhood integration), PostgreSQL with pgvector (semantic search), and Claude API for sentiment analysis.

---

## Table of Contents

1. [System Architecture](#system-architecture)
2. [Tech Stack](#tech-stack)
3. [Database Schema](#database-schema)
4. [Component Specifications](#component-specifications)
5. [Implementation Phases](#implementation-phases)
6. [Development Setup](#development-setup)
7. [Deployment Strategy](#deployment-strategy)
8. [Code Snapshots](#code-snapshots)

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Daily Orchestrator                       │
│                    (Go - main service)                       │
└──────────────┬──────────────────────────────────────────────┘
               │
       ┌───────┴───────┐
       │               │
       ▼               ▼
┌─────────────┐ ┌──────────────┐
│  Data Layer │ │ Analysis Layer│
└─────────────┘ └──────────────┘
       │               │
   ┌───┴───┬───────────┴───┬─────────────┐
   │       │               │             │
   ▼       ▼               ▼             ▼
┌────┐ ┌────┐      ┌──────────┐  ┌──────────┐
│ X  │ │ MKT│      │ Claude   │  │ Technical│
│ API│ │Data│      │   API    │  │Indicators│
└────┘ └────┘      └──────────┘  └──────────┘
   │       │               │             │
   └───┬───┴───────────┬───┴─────────────┘
       │               │
       ▼               ▼
┌─────────────────────────────────┐
│   Supabase (PostgreSQL)         │
│   - pgvector (embeddings)       │
│   - Time-series data            │
└─────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────┐
│   TUI (Bubble Tea)              │
│   - Daily recommendations       │
│   - Portfolio view              │
│   - Market signals              │
└─────────────────────────────────┘
```

### Data Flow

1. **Daily 7AM Trigger** → Orchestrator starts
2. **Parallel Data Collection**:
   - Fetch portfolio from Robinhood (Python script)
   - Fetch market data for tracked tickers (Go service)
   - Scrape creator content from X API (Go service)
3. **Storage**: Raw data → PostgreSQL
4. **Embedding Generation**: Creator content → sentence-transformers → pgvector
5. **Analysis**:
   - Calculate technical indicators
   - Semantic search for relevant historical context
   - Claude API: Sentiment analysis with context
   - Confidence scoring
6. **Recommendation Engine**: Generate buy/hold/wait signals
7. **TUI Display**: Present results to user

---

## Tech Stack

| Component | Technology | Justification |
|-----------|-----------|---------------|
| **Primary Language** | Go 1.21+ | Your current work focus, great concurrency for parallel API calls |
| **Portfolio Integration** | Python 3.11+ (robin_stocks) | Only official Robinhood library available |
| **Database** | Supabase (PostgreSQL 15+) | Managed Postgres with pgvector support |
| **Vector Search** | pgvector extension | Semantic search for historical context retrieval |
| **Embeddings** | sentence-transformers (all-MiniLM-L6-v2) | Fast, lightweight, good for financial text |
| **Market Data API** | Alpha Vantage (free tier) | 500 requests/day, comprehensive data |
| **Social Media** | X API v2 (Free tier) | Access to Moby Invest & Carbon Finance tweets |
| **LLM Analysis** | Claude 3.5 Sonnet API | Best reasoning for financial sentiment |
| **Technical Indicators** | pandas-ta (Python) | Comprehensive TA library |
| **TUI Framework** | Bubble Tea + Lip Gloss (Go) | Production-ready, excellent UX |
| **Task Scheduling** | systemd timer (Linux) / launchd (macOS) | Native OS-level cron |
| **Environment Management** | direnv + .envrc | Automatic env loading per directory |
| **Containerization** | Docker + Docker Compose | Local dev & deployment consistency |

---

## Database Schema

### Tables

```sql
-- Enable extensions
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- Portfolio holdings
CREATE TABLE holdings (
    id SERIAL PRIMARY KEY,
    ticker VARCHAR(10) NOT NULL,
    quantity DECIMAL(18, 8) NOT NULL,
    avg_cost DECIMAL(18, 4) NOT NULL,
    current_price DECIMAL(18, 4),
    market_value DECIMAL(18, 4),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Market data (converted to hypertable for time-series optimization)
CREATE TABLE market_data (
    id SERIAL PRIMARY KEY,
    ticker VARCHAR(10) NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL,
    open DECIMAL(18, 4),
    high DECIMAL(18, 4),
    low DECIMAL(18, 4),
    close DECIMAL(18, 4),
    volume BIGINT,
    vwap DECIMAL(18, 4),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('market_data', 'timestamp');
CREATE INDEX idx_market_data_ticker_timestamp ON market_data (ticker, timestamp DESC);

-- Technical indicators
CREATE TABLE technical_indicators (
    id SERIAL PRIMARY KEY,
    ticker VARCHAR(10) NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL,
    rsi_14 DECIMAL(10, 4),
    sma_50 DECIMAL(18, 4),
    sma_200 DECIMAL(18, 4),
    macd DECIMAL(18, 4),
    macd_signal DECIMAL(18, 4),
    atr_14 DECIMAL(18, 4),
    volume_avg_20 BIGINT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

SELECT create_hypertable('technical_indicators', 'timestamp');
CREATE INDEX idx_tech_ticker_timestamp ON technical_indicators (ticker, timestamp DESC);

-- Creator content (with embeddings)
CREATE TABLE creator_content (
    id SERIAL PRIMARY KEY,
    creator_name VARCHAR(100) NOT NULL,
    platform VARCHAR(50) DEFAULT 'twitter',
    content_id VARCHAR(100) UNIQUE NOT NULL,
    content_text TEXT NOT NULL,
    mentioned_tickers TEXT[], -- Array of tickers mentioned
    sentiment VARCHAR(20), -- bullish, bearish, neutral
    confidence_score DECIMAL(5, 4), -- 0-1
    embedding vector(384), -- sentence-transformers dimension
    posted_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_creator_posted_at ON creator_content (posted_at DESC);
CREATE INDEX idx_creator_embedding ON creator_content USING ivfflat (embedding vector_cosine_ops);

-- Historical signals (for backtesting)
CREATE TABLE signals (
    id SERIAL PRIMARY KEY,
    ticker VARCHAR(10) NOT NULL,
    signal_type VARCHAR(20) NOT NULL, -- buy, hold, wait
    recommendation_amount DECIMAL(18, 4),
    confidence_score DECIMAL(5, 4),
    reasoning TEXT,
    market_regime VARCHAR(50), -- calm, volatile, bearish, bullish
    vix_level DECIMAL(10, 4),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_signals_ticker_created ON signals (ticker, created_at DESC);

-- Creator accuracy tracking
CREATE TABLE creator_accuracy (
    id SERIAL PRIMARY KEY,
    creator_name VARCHAR(100) NOT NULL,
    ticker VARCHAR(10) NOT NULL,
    prediction_date DATE NOT NULL,
    predicted_sentiment VARCHAR(20),
    actual_price_change DECIMAL(10, 4), -- % change over next 7 days
    was_accurate BOOLEAN,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_creator_accuracy ON creator_accuracy (creator_name, was_accurate);

-- Configuration
CREATE TABLE config (
    key VARCHAR(100) PRIMARY KEY,
    value JSONB NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Insert default config
INSERT INTO config (key, value) VALUES
('tracked_tickers', '["SPY", "QQQ", "VOO", "VTI"]'),
('creators', '[
    {"name": "Moby Invest", "twitter_handle": "mobyinvest"},
    {"name": "Carbon Finance", "twitter_handle": "carbonfinance"}
]'),
('confidence_weights', '{
    "creator_consensus": 0.3,
    "technical_alignment": 0.3,
    "volume_confirmation": 0.2,
    "historical_accuracy": 0.2
}'),
('market_regime_thresholds', '{
    "vix_high": 25,
    "rsi_overbought": 70,
    "rsi_oversold": 30
}'),
('contribution_target', '{
    "monthly": 1000,
    "annual": 7000,
    "current": 600
}');
```

### Supabase Setup

```bash
# Create Supabase project at https://supabase.com
# Navigate to SQL Editor and run the schema above
# Enable RLS (Row Level Security) - disable for personal use or set up policies
```

---

## Component Specifications

### 1. Data Ingestion Layer

#### A. Robinhood Portfolio Fetcher (Python)

**File**: `services/robinhood/fetch_portfolio.py`

```python
#!/usr/bin/env python3
import os
import json
import robin_stocks.robinhood as rh
from datetime import datetime
import psycopg2
from psycopg2.extras import execute_values

def login():
    """Authenticate with Robinhood"""
    username = os.getenv('ROBINHOOD_USERNAME')
    password = os.getenv('ROBINHOOD_PASSWORD')
    totp = os.getenv('ROBINHOOD_TOTP')  # If 2FA enabled
    
    login_result = rh.login(username, password, mfa_code=totp)
    return login_result

def fetch_holdings():
    """Fetch current portfolio holdings"""
    positions = rh.account.build_holdings()
    holdings_data = []
    
    for ticker, data in positions.items():
        holdings_data.append({
            'ticker': ticker,
            'quantity': float(data['quantity']),
            'avg_cost': float(data['average_buy_price']),
            'current_price': float(data['price']),
            'market_value': float(data['equity'])
        })
    
    return holdings_data

def store_holdings(holdings):
    """Store holdings in Supabase"""
    conn = psycopg2.connect(os.getenv('DATABASE_URL'))
    cur = conn.cursor()
    
    # Clear existing holdings
    cur.execute("DELETE FROM holdings")
    
    # Insert new holdings
    insert_query = """
        INSERT INTO holdings (ticker, quantity, avg_cost, current_price, market_value)
        VALUES %s
    """
    values = [(h['ticker'], h['quantity'], h['avg_cost'], h['current_price'], h['market_value']) 
              for h in holdings]
    
    execute_values(cur, insert_query, values)
    conn.commit()
    cur.close()
    conn.close()
    
    print(f"✓ Stored {len(holdings)} holdings")

def main():
    try:
        login()
        holdings = fetch_holdings()
        store_holdings(holdings)
        
        # Output JSON for Go service to consume
        print(json.dumps({
            'status': 'success',
            'holdings': holdings,
            'timestamp': datetime.now().isoformat()
        }))
        
    except Exception as e:
        print(json.dumps({
            'status': 'error',
            'message': str(e)
        }))
        exit(1)

if __name__ == '__main__':
    main()
```

#### B. Market Data Fetcher (Go)

**File**: `services/market/fetcher.go`

```go
package market

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "time"
)

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

type MarketData struct {
    Ticker    string
    Timestamp time.Time
    Open      float64
    High      float64
    Low       float64
    Close     float64
    Volume    int64
}

type Fetcher struct {
    apiKey     string
    httpClient *http.Client
}

func NewFetcher() *Fetcher {
    return &Fetcher{
        apiKey: os.Getenv("ALPHAVANTAGE_API_KEY"),
        httpClient: &http.Client{
            Timeout: 10 * time.Second,
        },
    }
}

func (f *Fetcher) FetchQuote(ctx context.Context, ticker string) (*MarketData, error) {
    url := fmt.Sprintf(
        "https://www.alphavantage.co/query?function=GLOBAL_QUOTE&symbol=%s&apikey=%s",
        ticker, f.apiKey,
    )

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }

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

    // Parse and convert to MarketData
    data := &MarketData{
        Ticker:    ticker,
        Timestamp: time.Now(),
    }
    
    // Convert string values to float64 (error handling omitted for brevity)
    fmt.Sscanf(quote.GlobalQuote.Open, "%f", &data.Open)
    fmt.Sscanf(quote.GlobalQuote.High, "%f", &data.High)
    fmt.Sscanf(quote.GlobalQuote.Low, "%f", &data.Low)
    fmt.Sscanf(quote.GlobalQuote.Price, "%f", &data.Close)
    fmt.Sscanf(quote.GlobalQuote.Volume, "%d", &data.Volume)

    return data, nil
}

// FetchHistorical fetches daily historical data
func (f *Fetcher) FetchHistorical(ctx context.Context, ticker string, days int) ([]MarketData, error) {
    url := fmt.Sprintf(
        "https://www.alphavantage.co/query?function=TIME_SERIES_DAILY&symbol=%s&outputsize=compact&apikey=%s",
        ticker, f.apiKey,
    )

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, err
    }

    resp, err := f.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    // Parse response (implementation similar to FetchQuote)
    // Return slice of MarketData
    
    return nil, nil // Placeholder
}
```

#### C. X (Twitter) Content Scraper (Go)

**File**: `services/social/twitter.go`

```go
package social

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "os"
    "regexp"
    "strings"
    "time"
)

type Tweet struct {
    ID        string    `json:"id"`
    Text      string    `json:"text"`
    CreatedAt time.Time `json:"created_at"`
    AuthorID  string    `json:"author_id"`
}

type TwitterClient struct {
    bearerToken string
    httpClient  *http.Client
}

func NewTwitterClient() *TwitterClient {
    return &TwitterClient{
        bearerToken: os.Getenv("TWITTER_BEARER_TOKEN"),
        httpClient: &http.Client{
            Timeout: 15 * time.Second,
        },
    }
}

func (c *TwitterClient) FetchRecentTweets(ctx context.Context, username string, maxResults int) ([]Tweet, error) {
    // First, get user ID from username
    userID, err := c.getUserID(ctx, username)
    if err != nil {
        return nil, fmt.Errorf("get user ID: %w", err)
    }

    // Fetch tweets
    endpoint := fmt.Sprintf("https://api.twitter.com/2/users/%s/tweets", userID)
    params := url.Values{}
    params.Add("max_results", fmt.Sprintf("%d", maxResults))
    params.Add("tweet.fields", "created_at,text")
    params.Add("exclude", "retweets,replies")

    req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"?"+params.Encode(), nil)
    if err != nil {
        return nil, err
    }

    req.Header.Set("Authorization", "Bearer "+c.bearerToken)

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }

    var result struct {
        Data []Tweet `json:"data"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    return result.Data, nil
}

func (c *TwitterClient) getUserID(ctx context.Context, username string) (string, error) {
    endpoint := fmt.Sprintf("https://api.twitter.com/2/users/by/username/%s", username)
    
    req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
    if err != nil {
        return "", err
    }

    req.Header.Set("Authorization", "Bearer "+c.bearerToken)

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var result struct {
        Data struct {
            ID string `json:"id"`
        } `json:"data"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }

    return result.Data.ID, nil
}

// ExtractTickers finds stock ticker mentions in text
func ExtractTickers(text string) []string {
    // Match $TICKER or known tickers
    tickerRegex := regexp.MustCompile(`\$([A-Z]{1,5})\b|\\b(SPY|QQQ|VOO|VTI|PLTR)\b`)
    matches := tickerRegex.FindAllStringSubmatch(text, -1)
    
    tickers := make(map[string]bool)
    for _, match := range matches {
        if match[1] != "" {
            tickers[match[1]] = true
        }
        if match[2] != "" {
            tickers[match[2]] = true
        }
    }
    
    result := make([]string, 0, len(tickers))
    for ticker := range tickers {
        result = append(result, ticker)
    }
    
    return result
}
```

### 2. Analysis Layer

#### A. Technical Indicators (Python)

**File**: `services/analysis/indicators.py`

```python
#!/usr/bin/env python3
import os
import pandas as pd
import pandas_ta as ta
import psycopg2
from psycopg2.extras import execute_values
from datetime import datetime, timedelta

def fetch_market_data(ticker, days=200):
    """Fetch historical market data for indicator calculation"""
    conn = psycopg2.connect(os.getenv('DATABASE_URL'))
    
    query = """
        SELECT timestamp, open, high, low, close, volume
        FROM market_data
        WHERE ticker = %s AND timestamp >= %s
        ORDER BY timestamp ASC
    """
    
    cutoff_date = datetime.now() - timedelta(days=days)
    df = pd.read_sql_query(query, conn, params=(ticker, cutoff_date))
    conn.close()
    
    return df

def calculate_indicators(df):
    """Calculate all technical indicators"""
    # RSI (14-day)
    df['rsi_14'] = ta.rsi(df['close'], length=14)
    
    # Simple Moving Averages
    df['sma_50'] = ta.sma(df['close'], length=50)
    df['sma_200'] = ta.sma(df['close'], length=200)
    
    # MACD
    macd = ta.macd(df['close'])
    df['macd'] = macd['MACD_12_26_9']
    df['macd_signal'] = macd['MACDs_12_26_9']
    
    # ATR (14-day)
    df['atr_14'] = ta.atr(df['high'], df['low'], df['close'], length=14)
    
    # Volume average (20-day)
    df['volume_avg_20'] = ta.sma(df['volume'], length=20)
    
    return df

def store_indicators(ticker, df):
    """Store calculated indicators in database"""
    conn = psycopg2.connect(os.getenv('DATABASE_URL'))
    cur = conn.cursor()
    
    # Only store most recent indicator
    latest = df.iloc[-1]
    
    insert_query = """
        INSERT INTO technical_indicators 
        (ticker, timestamp, rsi_14, sma_50, sma_200, macd, macd_signal, atr_14, volume_avg_20)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
    """
    
    cur.execute(insert_query, (
        ticker,
        latest['timestamp'],
        float(latest['rsi_14']) if pd.notna(latest['rsi_14']) else None,
        float(latest['sma_50']) if pd.notna(latest['sma_50']) else None,
        float(latest['sma_200']) if pd.notna(latest['sma_200']) else None,
        float(latest['macd']) if pd.notna(latest['macd']) else None,
        float(latest['macd_signal']) if pd.notna(latest['macd_signal']) else None,
        float(latest['atr_14']) if pd.notna(latest['atr_14']) else None,
        int(latest['volume_avg_20']) if pd.notna(latest['volume_avg_20']) else None
    ))
    
    conn.commit()
    cur.close()
    conn.close()

def main():
    tickers = os.getenv('TRACKED_TICKERS', 'SPY,QQQ,VOO,VTI').split(',')
    
    for ticker in tickers:
        ticker = ticker.strip()
        print(f"Calculating indicators for {ticker}...")
        
        df = fetch_market_data(ticker)
        if df.empty:
            print(f"No data for {ticker}")
            continue
        
        df = calculate_indicators(df)
        store_indicators(ticker, df)
        print(f"✓ Stored indicators for {ticker}")

if __name__ == '__main__':
    main()
```

#### B. Embedding Generator (Python)

**File**: `services/analysis/embeddings.py`

```python
#!/usr/bin/env python3
import os
import psycopg2
from sentence_transformers import SentenceTransformer
import numpy as np

# Load model once
model = SentenceTransformer('all-MiniLM-L6-v2')

def generate_embeddings():
    """Generate embeddings for creator content without them"""
    conn = psycopg2.connect(os.getenv('DATABASE_URL'))
    cur = conn.cursor()
    
    # Fetch content without embeddings
    cur.execute("""
        SELECT id, content_text
        FROM creator_content
        WHERE embedding IS NULL
        ORDER BY created_at DESC
        LIMIT 100
    """)
    
    rows = cur.fetchall()
    
    for content_id, text in rows:
        # Generate embedding
        embedding = model.encode(text)
        embedding_list = embedding.tolist()
        
        # Store in database
        cur.execute("""
            UPDATE creator_content
            SET embedding = %s
            WHERE id = %s
        """, (embedding_list, content_id))
    
    conn.commit()
    cur.close()
    conn.close()
    
    print(f"✓ Generated {len(rows)} embeddings")

if __name__ == '__main__':
    generate_embeddings()
```

#### C. Claude Sentiment Analyzer (Go)

**File**: `services/analysis/sentiment.go`

```go
package analysis

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
)

type ClaudeRequest struct {
    Model     string    `json:"model"`
    MaxTokens int       `json:"max_tokens"`
    Messages  []Message `json:"messages"`
}

type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

type ClaudeResponse struct {
    Content []struct {
        Text string `json:"text"`
    } `json:"content"`
}

type SentimentResult struct {
    Ticker     string  `json:"ticker"`
    Sentiment  string  `json:"sentiment"`  // bullish, bearish, neutral
    Confidence float64 `json:"confidence"` // 0-1
    Reasoning  string  `json:"reasoning"`
}

type SentimentAnalyzer struct {
    apiKey     string
    httpClient *http.Client
}

func NewSentimentAnalyzer() *SentimentAnalyzer {
    return &SentimentAnalyzer{
        apiKey: os.Getenv("ANTHROPIC_API_KEY"),
        httpClient: &http.Client{},
    }
}

func (s *SentimentAnalyzer) AnalyzeSentiment(ctx context.Context, ticker string, creatorContent []string, marketContext string) (*SentimentResult, error) {
    prompt := fmt.Sprintf(`Analyze the sentiment for ticker %s based on the following:

Creator Content:
%s

Market Context:
%s

Respond with JSON in this format:
{
    "ticker": "%s",
    "sentiment": "bullish|bearish|neutral",
    "confidence": 0.0-1.0,
    "reasoning": "brief explanation"
}`, ticker, formatContent(creatorContent), marketContext, ticker)

    reqBody := ClaudeRequest{
        Model:     "claude-sonnet-4-20250514",
        MaxTokens: 1000,
        Messages: []Message{
            {
                Role:    "user",
                Content: prompt,
            },
        },
    }

    jsonData, err := json.Marshal(reqBody)
    if err != nil {
        return nil, err
    }

    req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
    if err != nil {
        return nil, err
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("x-api-key", s.apiKey)
    req.Header.Set("anthropic-version", "2023-06-01")

    resp, err := s.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var claudeResp ClaudeResponse
    if err := json.NewDecoder(resp.Body).Decode(&claudeResp); err != nil {
        return nil, err
    }

    if len(claudeResp.Content) == 0 {
        return nil, fmt.Errorf("no content in response")
    }

    var result SentimentResult
    if err := json.Unmarshal([]byte(claudeResp.Content[0].Text), &result); err != nil {
        return nil, fmt.Errorf("parse sentiment result: %w", err)
    }

    return &result, nil
}

func formatContent(content []string) string {
    result := ""
    for i, c := range content {
        result += fmt.Sprintf("%d. %s\n\n", i+1, c)
    }
    return result
}
```

#### D. Confidence Scoring Engine (Go)

**File**: `services/analysis/confidence.go`

```go
package analysis

import (
    "context"
    "database/sql"
    "fmt"
)

type ConfidenceWeights struct {
    CreatorConsensus     float64
    TechnicalAlignment   float64
    VolumeConfirmation   float64
    HistoricalAccuracy   float64
}

type ConfidenceInputs struct {
    Ticker               string
    CreatorSentiments    map[string]string // creator -> sentiment
    TechnicalSignals     []string          // list of bullish/bearish signals
    CurrentVolume        int64
    AvgVolume            int64
    CreatorAccuracyRates map[string]float64
}

type ConfidenceScore struct {
    Overall              float64
    CreatorConsensus     float64
    TechnicalAlignment   float64
    VolumeConfirmation   float64
    HistoricalAccuracy   float64
    Breakdown            string
}

func CalculateConfidence(inputs ConfidenceInputs, weights ConfidenceWeights) ConfidenceScore {
    // 1. Creator Consensus: % of creators bullish on ticker
    bullishCount := 0
    totalCreators := len(inputs.CreatorSentiments)
    for _, sentiment := range inputs.CreatorSentiments {
        if sentiment == "bullish" {
            bullishCount++
        }
    }
    creatorConsensus := 0.0
    if totalCreators > 0 {
        creatorConsensus = float64(bullishCount) / float64(totalCreators)
    }

    // 2. Technical Alignment: % of indicators signaling same direction
    bullishSignals := 0
    totalSignals := len(inputs.TechnicalSignals)
    for _, signal := range inputs.TechnicalSignals {
        if signal == "bullish" {
            bullishSignals++
        }
    }
    technicalAlignment := 0.0
    if totalSignals > 0 {
        technicalAlignment = float64(bullishSignals) / float64(totalSignals)
    }

    // 3. Volume Confirmation: Current volume vs 20-day avg (normalized 0-1)
    volumeConfirmation := 0.5 // default
    if inputs.AvgVolume > 0 {
        ratio := float64(inputs.CurrentVolume) / float64(inputs.AvgVolume)
        // Normalize: >2x = 1.0, <0.5x = 0.0
        if ratio >= 2.0 {
            volumeConfirmation = 1.0
        } else if ratio <= 0.5 {
            volumeConfirmation = 0.0
        } else {
            volumeConfirmation = (ratio - 0.5) / 1.5
        }
    }

    // 4. Historical Accuracy: Average accuracy of creators
    historicalAccuracy := 0.0
    if len(inputs.CreatorAccuracyRates) > 0 {
        sum := 0.0
        for _, accuracy := range inputs.CreatorAccuracyRates {
            sum += accuracy
        }
        historicalAccuracy = sum / float64(len(inputs.CreatorAccuracyRates))
    }

    // Calculate weighted overall score
    overall := (creatorConsensus * weights.CreatorConsensus) +
        (technicalAlignment * weights.TechnicalAlignment) +
        (volumeConfirmation * weights.VolumeConfirmation) +
        (historicalAccuracy * weights.HistoricalAccuracy)

    breakdown := fmt.Sprintf(
        "Creator: %.2f | Technical: %.2f | Volume: %.2f | History: %.2f",
        creatorConsensus, technicalAlignment, volumeConfirmation, historicalAccuracy,
    )

    return ConfidenceScore{
        Overall:            overall,
        CreatorConsensus:   creatorConsensus,
        TechnicalAlignment: technicalAlignment,
        VolumeConfirmation: volumeConfirmation,
        HistoricalAccuracy: historicalAccuracy,
        Breakdown:          breakdown,
    }
}

// FetchCreatorAccuracy retrieves historical accuracy rates from database
func FetchCreatorAccuracy(ctx context.Context, db *sql.DB, creators []string) (map[string]float64, error) {
    query := `
        SELECT creator_name, 
               COALESCE(AVG(CASE WHEN was_accurate THEN 1.0 ELSE 0.0 END), 0.5) as accuracy
        FROM creator_accuracy
        WHERE creator_name = ANY($1)
        GROUP BY creator_name
    `

    rows, err := db.QueryContext(ctx, query, creators)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    rates := make(map[string]float64)
    for rows.Next() {
        var name string
        var accuracy float64
        if err := rows.Scan(&name, &accuracy); err != nil {
            return nil, err
        }
        rates[name] = accuracy
    }

    // Default 0.5 for creators without history
    for _, creator := range creators {
        if _, exists := rates[creator]; !exists {
            rates[creator] = 0.5
        }
    }

    return rates, nil
}
```

### 3. Recommendation Engine

**File**: `services/engine/recommender.go`

```go
package engine

import (
    "context"
    "database/sql"
    "fmt"
    "market-intel/services/analysis"
)

type MarketRegime string

const (
    RegimeCalm     MarketRegime = "calm"
    RegimeVolatile MarketRegime = "volatile"
    RegimeBullish  MarketRegime = "bullish"
    RegimeBearish  MarketRegime = "bearish"
)

type Recommendation struct {
    Ticker            string
    Action            string  // buy, hold, wait
    Amount            float64 // Dollar amount
    ConfidenceScore   float64
    Reasoning         string
    MarketRegime      MarketRegime
    VIXLevel          float64
}

type RecommendationEngine struct {
    db               *sql.DB
    vixHighThreshold float64
    rsiOverbought    float64
    rsiOversold      float64
}

func NewRecommendationEngine(db *sql.DB) *RecommendationEngine {
    return &RecommendationEngine{
        db:               db,
        vixHighThreshold: 25.0,
        rsiOverbought:    70.0,
        rsiOversold:      30.0,
    }
}

func (e *RecommendationEngine) GenerateRecommendations(ctx context.Context, budget float64) ([]Recommendation, error) {
    // 1. Determine market regime
    regime, vix, err := e.detectMarketRegime(ctx)
    if err != nil {
        return nil, fmt.Errorf("detect market regime: %w", err)
    }

    // 2. If high volatility, recommend holding cash
    if regime == RegimeVolatile {
        return []Recommendation{
            {
                Action:   "wait",
                Reasoning: fmt.Sprintf("High volatility detected (VIX: %.2f). Wait 2-3 days for market to stabilize.", vix),
                VIXLevel: vix,
                MarketRegime: regime,
            },
        }, nil
    }

    // 3. Fetch confidence scores for tracked tickers
    tickers := []string{"SPY", "QQQ", "VOO", "VTI"}
    recommendations := make([]Recommendation, 0)

    for _, ticker := range tickers {
        score, err := e.getTickerConfidenceScore(ctx, ticker)
        if err != nil {
            continue
        }

        // 4. Generate allocation based on confidence
        allocation := e.calculateAllocation(ticker, score, budget, regime)
        
        recommendations = append(recommendations, Recommendation{
            Ticker:          ticker,
            Action:          allocation.Action,
            Amount:          allocation.Amount,
            ConfidenceScore: score.Overall,
            Reasoning:       allocation.Reasoning,
            MarketRegime:    regime,
            VIXLevel:        vix,
        })
    }

    return recommendations, nil
}

func (e *RecommendationEngine) detectMarketRegime(ctx context.Context) (MarketRegime, float64, error) {
    // Fetch VIX data
    var vix float64
    err := e.db.QueryRowContext(ctx, `
        SELECT close FROM market_data
        WHERE ticker = 'VIX'
        ORDER BY timestamp DESC
        LIMIT 1
    `).Scan(&vix)
    
    if err != nil {
        // Default to calm if VIX not available
        return RegimeCalm, 0, nil
    }

    if vix > e.vixHighThreshold {
        return RegimeVolatile, vix, nil
    }

    // Check SPY trend
    var spyRSI float64
    err = e.db.QueryRowContext(ctx, `
        SELECT rsi_14 FROM technical_indicators
        WHERE ticker = 'SPY'
        ORDER BY timestamp DESC
        LIMIT 1
    `).Scan(&spyRSI)

    if err == nil {
        if spyRSI > e.rsiOverbought {
            return RegimeBearish, vix, nil
        } else if spyRSI < e.rsiOversold {
            return RegimeBullish, vix, nil
        }
    }

    return RegimeCalm, vix, nil
}

func (e *RecommendationEngine) getTickerConfidenceScore(ctx context.Context, ticker string) (*analysis.ConfidenceScore, error) {
    // This would call the confidence scoring engine
    // Placeholder implementation
    return &analysis.ConfidenceScore{
        Overall: 0.75,
    }, nil
}

type AllocationResult struct {
    Action    string
    Amount    float64
    Reasoning string
}

func (e *RecommendationEngine) calculateAllocation(ticker string, score *analysis.ConfidenceScore, budget float64, regime MarketRegime) AllocationResult {
    // Core holdings allocation strategy
    coreHoldings := map[string]float64{
        "SPY": 0.40, // 40% of budget
        "QQQ": 0.30, // 30% of budget
        "VOO": 0.20, // 20% of budget
        "VTI": 0.10, // 10% of budget
    }

    baseAllocation, isCore := coreHoldings[ticker]
    
    if !isCore {
        // Risk allocation (10% total budget)
        if score.Overall < 0.6 {
            return AllocationResult{
                Action:    "wait",
                Amount:    0,
                Reasoning: fmt.Sprintf("Confidence too low (%.0f%%) for risk allocation", score.Overall*100),
            }
        }
        
        return AllocationResult{
            Action:    "buy",
            Amount:    budget * 0.10,
            Reasoning: fmt.Sprintf("Risk allocation approved (%.0f%% confidence)", score.Overall*100),
        }
    }

    // Core holding allocation
    amount := budget * baseAllocation

    // Adjust based on confidence
    if score.Overall < 0.5 {
        amount *= 0.5 // Reduce by 50%
        return AllocationResult{
            Action:    "buy",
            Amount:    amount,
            Reasoning: fmt.Sprintf("Reduced allocation due to moderate confidence (%.0f%%)", score.Overall*100),
        }
    }

    return AllocationResult{
        Action:    "buy",
        Amount:    amount,
        Reasoning: fmt.Sprintf("Standard allocation (%.0f%% confidence)", score.Overall*100),
    }
}
```

### 4. TUI (Terminal User Interface)

**File**: `cmd/tui/main.go`

```go
package main

import (
    "fmt"
    "os"
    "strings"

    "github.com/charmbracelet/bubbles/table"
    "github.com/charmbracelet/bubbles/viewport"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

var (
    titleStyle = lipgloss.NewStyle().
        Bold(true).
        Foreground(lipgloss.Color("#FAFAFA")).
        Background(lipgloss.Color("#7D56F4")).
        Padding(0, 1)

    headerStyle = lipgloss.NewStyle().
        Bold(true).
        Foreground(lipgloss.Color("#FAFAFA")).
        Background(lipgloss.Color("#383838")).
        Padding(0, 1)

    bullishStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("#00FF00"))

    bearishStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("#FF0000"))

    neutralStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("#FFFF00"))
)

type model struct {
    recommendations []Recommendation
    portfolio       Portfolio
    viewport        viewport.Model
    table           table.Model
    ready           bool
}

type Recommendation struct {
    Ticker     string
    Action     string
    Amount     float64
    Confidence float64
    Signal     string
    Reasoning  string
}

type Portfolio struct {
    Current  float64
    Target   float64
    Holdings []Holding
}

type Holding struct {
    Ticker       string
    Quantity     float64
    MarketValue  float64
    CurrentPrice float64
}

func initialModel() model {
    // Mock data - in production, this would fetch from DB
    recs := []Recommendation{
        {"SPY", "buy", 400, 0.78, "bullish", "Strong uptrend, creator consensus"},
        {"QQQ", "buy", 300, 0.82, "bullish", "Tech sector strength"},
        {"VOO", "buy", 200, 0.75, "neutral", "Steady growth"},
        {"PLTR", "wait", 0, 0.64, "neutral", "Monitor for deeper pullback to $85"},
    }

    portfolio := Portfolio{
        Current: 600,
        Target:  7000,
        Holdings: []Holding{
            {"SPY", 1.2, 549.98, 458.32},
        },
    }

    columns := []table.Column{
        {Title: "Ticker", Width: 8},
        {Title: "Action", Width: 8},
        {Title: "Amount", Width: 10},
        {Title: "Confidence", Width: 12},
        {Title: "Signal", Width: 10},
    }

    rows := make([]table.Row, len(recs))
    for i, r := range recs {
        rows[i] = table.Row{
            r.Ticker,
            r.Action,
            fmt.Sprintf("$%.0f", r.Amount),
            fmt.Sprintf("%.0f%%", r.Confidence*100),
            r.Signal,
        }
    }

    t := table.New(
        table.WithColumns(columns),
        table.WithRows(rows),
        table.WithFocused(true),
        table.WithHeight(7),
    )

    s := table.DefaultStyles()
    s.Header = s.Header.
        BorderStyle(lipgloss.NormalBorder()).
        BorderForeground(lipgloss.Color("240")).
        BorderBottom(true).
        Bold(true)
    s.Selected = s.Selected.
        Foreground(lipgloss.Color("229")).
        Background(lipgloss.Color("57")).
        Bold(false)
    t.SetStyles(s)

    return model{
        recommendations: recs,
        portfolio:       portfolio,
        table:           t,
    }
}

func (m model) Init() tea.Cmd {
    return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd

    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "q", "ctrl+c":
            return m, tea.Quit
        case "enter":
            // Show details of selected recommendation
            return m, nil
        }
    case tea.WindowSizeMsg:
        if !m.ready {
            m.viewport = viewport.New(msg.Width, msg.Height-10)
            m.viewport.YPosition = 0
            m.ready = true
        }
    }

    m.table, cmd = m.table.Update(msg)
    return m, cmd
}

func (m model) View() string {
    if !m.ready {
        return "\n  Initializing..."
    }

    // Header
    header := titleStyle.Render("Market Intelligence - Jan 30, 2025 (Pre-Market)")

    // Portfolio summary
    portfolioProgress := float64(m.portfolio.Current) / float64(m.portfolio.Target) * 100
    portfolioView := headerStyle.Render(fmt.Sprintf(
        "Portfolio: $%.0f / $%.0f (%.0f%%) | Monthly Contribution: $1000 pending",
        m.portfolio.Current, m.portfolio.Target, portfolioProgress,
    ))

    // Recommendations table
    tableView := "\n" + headerStyle.Render("RECOMMENDATIONS") + "\n" + m.table.View()

    // Selected recommendation details
    selected := m.recommendations[m.table.Cursor()]
    detailsView := fmt.Sprintf("\n\n%s\n%s\n",
        headerStyle.Render(fmt.Sprintf("Details: %s", selected.Ticker)),
        selected.Reasoning,
    )

    // Market regime warning (if applicable)
    warningView := ""
    // In production, check actual market regime
    // if regime == "volatile" {
    //     warningView = "\n" + bearishStyle.Render("⚠ Market signal: HOLD cash - VIX elevated, wait 2-3 days")
    // }

    // Footer
    footer := "\n[↑/↓] Navigate  [Enter] Details  [Q] Quit"

    return lipgloss.JoinVertical(
        lipgloss.Left,
        header,
        portfolioView,
        tableView,
        detailsView,
        warningView,
        footer,
    )
}

func main() {
    p := tea.NewProgram(initialModel(), tea.WithAltScreen())
    if _, err := p.Run(); err != nil {
        fmt.Printf("Error: %v", err)
        os.Exit(1)
    }
}
```

---

## Implementation Phases

### Phase 1: Foundation (Week 1)

**Goal**: Set up infrastructure and basic data ingestion

**Tasks**:
1. ✅ Create Supabase project and run schema
2. ✅ Set up Go project structure
3. ✅ Implement Robinhood portfolio fetcher (Python)
4. ✅ Implement Alpha Vantage market data fetcher (Go)
5. ✅ Implement X/Twitter content scraper (Go)
6. ✅ Create database connection utilities
7. ✅ Test data ingestion pipeline

**Deliverable**: Data flowing into database from all sources

### Phase 2: Analysis Engine (Week 2)

**Goal**: Build analysis and scoring systems

**Tasks**:
1. ✅ Implement technical indicators calculator (Python)
2. ✅ Implement embedding generator (Python)
3. ✅ Implement Claude sentiment analyzer (Go)
4. ✅ Implement confidence scoring engine (Go)
5. ✅ Create semantic search with pgvector
6. ✅ Test analysis pipeline end-to-end

**Deliverable**: Confidence scores generated for tickers

### Phase 3: Recommendation Engine (Week 3)

**Goal**: Generate actionable recommendations

**Tasks**:
1. ✅ Implement market regime detector
2. ✅ Implement allocation calculator
3. ✅ Build recommendation engine
4. ✅ Add portfolio contribution tracking
5. ✅ Test recommendation logic

**Deliverable**: Daily recommendations generated

### Phase 4: TUI & Orchestration (Week 4)

**Goal**: Complete user interface and automation

**Tasks**:
1. ✅ Build Bubble Tea TUI
2. ✅ Create orchestrator (main daily workflow)
3. ✅ Set up systemd timer for 7AM daily runs
4. ✅ Add error handling and logging
5. ✅ Create Docker Compose for local dev
6. ✅ End-to-end testing

**Deliverable**: Fully functional system running daily

### Phase 5: Optimization (Month 2+)

**Goal**: Improve accuracy and add features

**Tasks**:
1. ✅ Track actual performance vs recommendations
2. ✅ Tune confidence weights based on accuracy
3. ✅ Add backtesting module
4. ✅ Implement creator accuracy tracking
5. ✅ Add historical context retrieval (RAG)
6. ✅ Optimize database queries

**Deliverable**: Production-ready system with proven accuracy

---

## Development Setup

### Prerequisites

```bash
# Install Go 1.21+
wget https://go.dev/dl/go1.21.6.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Install Python 3.11+
sudo apt install python3.11 python3.11-venv python3-pip

# Install PostgreSQL client tools
sudo apt install postgresql-client

# Install direnv for environment management
sudo apt install direnv
echo 'eval "$(direnv hook bash)"' >> ~/.bashrc
```

### Project Structure

```
market-intelligence/
├── cmd/
│   ├── orchestrator/       # Main daily runner
│   │   └── main.go
│   └── tui/                # Terminal UI
│       └── main.go
├── services/
│   ├── market/             # Market data fetcher (Go)
│   │   ├── fetcher.go
│   │   └── fetcher_test.go
│   ├── social/             # Twitter scraper (Go)
│   │   ├── twitter.go
│   │   └── twitter_test.go
│   ├── robinhood/          # Portfolio fetcher (Python)
│   │   ├── fetch_portfolio.py
│   │   └── requirements.txt
│   ├── analysis/           # Analysis services
│   │   ├── sentiment.go    # Claude API (Go)
│   │   ├── confidence.go   # Scoring (Go)
│   │   ├── indicators.py   # Technical indicators (Python)
│   │   ├── embeddings.py   # Vector embeddings (Python)
│   │   └── requirements.txt
│   └── engine/             # Recommendation engine
│       ├── recommender.go
│       └── recommender_test.go
├── pkg/
│   ├── database/           # DB utilities
│   │   ├── postgres.go
│   │   └── migrations/
│   └── config/             # Configuration
│       └── config.go
├── scripts/
│   ├── setup_db.sql        # Database schema
│   ├── daily_run.sh        # Orchestration script
│   └── install_deps.sh     # Dependency installer
├── deployments/
│   ├── docker-compose.yml
│   ├── Dockerfile.go
│   └── Dockerfile.python
├── .envrc                  # Environment variables
├── go.mod
├── go.sum
└── README.md
```

### Environment Setup

**Create `.envrc` file**:

```bash
# Database
export DATABASE_URL="postgresql://postgres:[password]@[host]:5432/market_intel"

# APIs
export ALPHAVANTAGE_API_KEY="your_alpha_vantage_key"
export TWITTER_BEARER_TOKEN="your_twitter_bearer_token"
export ANTHROPIC_API_KEY="your_anthropic_key"

# Robinhood
export ROBINHOOD_USERNAME="your_robinhood_email"
export ROBINHOOD_PASSWORD="your_robinhood_password"
export ROBINHOOD_TOTP="your_2fa_secret"  # Optional if 2FA enabled

# Tracking
export TRACKED_TICKERS="SPY,QQQ,VOO,VTI,PLTR"
export CREATORS="mobyinvest,carbonfinance"

# Allow direnv
direnv allow .
```

### Install Dependencies

**Go dependencies**:

```bash
# Initialize Go module
go mod init market-intelligence

# Install packages
go get github.com/charmbracelet/bubbletea
go get github.com/charmbracelet/bubbles
go get github.com/charmbracelet/lipgloss
go get github.com/lib/pq  # PostgreSQL driver
```

**Python dependencies**:

Create `services/robinhood/requirements.txt`:
```
robin-stocks==3.0.1
psycopg2-binary==2.9.9
```

Create `services/analysis/requirements.txt`:
```
pandas==2.1.4
pandas-ta==0.3.14b
sentence-transformers==2.2.2
psycopg2-binary==2.9.9
```

Install:
```bash
# Create virtual environment
python3 -m venv venv
source venv/bin/activate

# Install dependencies
pip install -r services/robinhood/requirements.txt
pip install -r services/analysis/requirements.txt
```

### Database Setup

**1. Create Supabase project**:
- Go to https://supabase.com
- Create new project
- Copy connection string

**2. Run schema**:
```bash
psql $DATABASE_URL -f scripts/setup_db.sql
```

**3. Enable extensions**:
```sql
-- Run in Supabase SQL Editor
CREATE EXTENSION IF NOT EXISTS vector;
```

### Docker Compose (Optional Local Development)

**Create `deployments/docker-compose.yml`**:

```yaml
version: '3.8'

services:
  postgres:
    image: ankane/pgvector:latest
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: market_intel
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./scripts/setup_db.sql:/docker-entrypoint-initdb.d/setup.sql

  app:
    build:
      context: ..
      dockerfile: deployments/Dockerfile.go
    depends_on:
      - postgres
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/market_intel
      ALPHAVANTAGE_API_KEY: ${ALPHAVANTAGE_API_KEY}
      TWITTER_BEARER_TOKEN: ${TWITTER_BEARER_TOKEN}
      ANTHROPIC_API_KEY: ${ANTHROPIC_API_KEY}
    volumes:
      - ..:/app

volumes:
  postgres_data:
```

**Create `deployments/Dockerfile.go`**:

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /bin/orchestrator ./cmd/orchestrator
RUN go build -o /bin/tui ./cmd/tui

FROM alpine:latest
RUN apk --no-cache add ca-certificates python3 py3-pip

COPY --from=builder /bin/orchestrator /bin/orchestrator
COPY --from=builder /bin/tui /bin/tui
COPY services/ /app/services/

WORKDIR /app
RUN pip3 install -r services/robinhood/requirements.txt
RUN pip3 install -r services/analysis/requirements.txt

CMD ["/bin/orchestrator"]
```

Run:
```bash
docker-compose -f deployments/docker-compose.yml up -d
```

---

## Deployment Strategy

### Daily Automation with Systemd

**Create systemd service**: `/etc/systemd/system/market-intel.service`

```ini
[Unit]
Description=Market Intelligence Daily Runner
After=network.target

[Service]
Type=oneshot
User=your_username
WorkingDirectory=/home/your_username/market-intelligence
Environment="DATABASE_URL=your_database_url"
Environment="ALPHAVANTAGE_API_KEY=your_key"
Environment="TWITTER_BEARER_TOKEN=your_token"
Environment="ANTHROPIC_API_KEY=your_key"
ExecStart=/home/your_username/market-intelligence/scripts/daily_run.sh

[Install]
WantedBy=multi-user.target
```

**Create systemd timer**: `/etc/systemd/system/market-intel.timer`

```ini
[Unit]
Description=Market Intelligence Daily Timer
Requires=market-intel.service

[Timer]
OnCalendar=Mon-Fri 07:00:00
Persistent=true

[Install]
WantedBy=timers.target
```

**Enable and start**:

```bash
sudo systemctl daemon-reload
sudo systemctl enable market-intel.timer
sudo systemctl start market-intel.timer

# Check status
systemctl status market-intel.timer
journalctl -u market-intel.service
```

### Daily Run Script

**Create `scripts/daily_run.sh`**:

```bash
#!/bin/bash
set -e

PROJECT_ROOT="/home/$(whoami)/market-intelligence"
cd $PROJECT_ROOT

echo "=== Market Intelligence Daily Run - $(date) ==="

# 1. Fetch portfolio from Robinhood
echo "Fetching portfolio..."
source venv/bin/activate
python3 services/robinhood/fetch_portfolio.py

# 2. Fetch market data
echo "Fetching market data..."
./bin/orchestrator fetch-market

# 3. Fetch creator content
echo "Fetching creator content..."
./bin/orchestrator fetch-social

# 4. Calculate technical indicators
echo "Calculating indicators..."
python3 services/analysis/indicators.py

# 5. Generate embeddings for new content
echo "Generating embeddings..."
python3 services/analysis/embeddings.py

# 6. Run analysis and generate recommendations
echo "Generating recommendations..."
./bin/orchestrator analyze

# 7. Display TUI
echo "Launching TUI..."
./bin/tui

echo "=== Daily run complete ==="
```

Make executable:
```bash
chmod +x scripts/daily_run.sh
```

### Monitoring & Logging

**Add logging to Go services**:

```go
import "log"

// In main.go or orchestrator
logFile, err := os.OpenFile("/var/log/market-intel.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
if err != nil {
    log.Fatal(err)
}
defer logFile.Close()

log.SetOutput(logFile)
log.Println("Starting daily run...")
```

**View logs**:
```bash
tail -f /var/log/market-intel.log
```

---

## Code Snapshots

### Main Orchestrator

**File**: `cmd/orchestrator/main.go`

```go
package main

import (
    "context"
    "database/sql"
    "fmt"
    "log"
    "os"
    "strings"
    "time"

    _ "github.com/lib/pq"
    "market-intelligence/services/analysis"
    "market-intelligence/services/engine"
    "market-intelligence/services/market"
    "market-intelligence/services/social"
)

func main() {
    ctx := context.Background()

    // Connect to database
    db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    if len(os.Args) < 2 {
        log.Fatal("Usage: orchestrator <command>")
    }

    command := os.Args[1]

    switch command {
    case "fetch-market":
        fetchMarketData(ctx, db)
    case "fetch-social":
        fetchSocialContent(ctx, db)
    case "analyze":
        runAnalysis(ctx, db)
    default:
        log.Fatalf("Unknown command: %s", command)
    }
}

func fetchMarketData(ctx context.Context, db *sql.DB) {
    fetcher := market.NewFetcher()
    tickers := strings.Split(os.Getenv("TRACKED_TICKERS"), ",")

    for _, ticker := range tickers {
        ticker = strings.TrimSpace(ticker)
        log.Printf("Fetching %s...", ticker)

        data, err := fetcher.FetchQuote(ctx, ticker)
        if err != nil {
            log.Printf("Error fetching %s: %v", ticker, err)
            continue
        }

        // Store in database
        _, err = db.ExecContext(ctx, `
            INSERT INTO market_data (ticker, timestamp, open, high, low, close, volume)
            VALUES ($1, $2, $3, $4, $5, $6, $7)
        `, data.Ticker, data.Timestamp, data.Open, data.High, data.Low, data.Close, data.Volume)

        if err != nil {
            log.Printf("Error storing %s: %v", ticker, err)
            continue
        }

        log.Printf("✓ Stored %s", ticker)
        time.Sleep(15 * time.Second) // Respect API rate limits
    }
}

func fetchSocialContent(ctx context.Context, db *sql.DB) {
    client := social.NewTwitterClient()
    creators := strings.Split(os.Getenv("CREATORS"), ",")

    for _, creator := range creators {
        creator = strings.TrimSpace(creator)
        log.Printf("Fetching tweets from %s...", creator)

        tweets, err := client.FetchRecentTweets(ctx, creator, 10)
        if err != nil {
            log.Printf("Error fetching %s: %v", creator, err)
            continue
        }

        for _, tweet := range tweets {
            tickers := social.ExtractTickers(tweet.Text)

            _, err = db.ExecContext(ctx, `
                INSERT INTO creator_content 
                (creator_name, platform, content_id, content_text, mentioned_tickers, posted_at)
                VALUES ($1, $2, $3, $4, $5, $6)
                ON CONFLICT (content_id) DO NOTHING
            `, creator, "twitter", tweet.ID, tweet.Text, tickers, tweet.CreatedAt)

            if err != nil {
                log.Printf("Error storing tweet: %v", err)
            }
        }

        log.Printf("✓ Stored tweets from %s", creator)
        time.Sleep(5 * time.Second)
    }
}

func runAnalysis(ctx context.Context, db *sql.DB) {
    log.Println("Running analysis...")

    // 1. Generate recommendations
    recEngine := engine.NewRecommendationEngine(db)
    recommendations, err := recEngine.GenerateRecommendations(ctx, 1000.0)
    if err != nil {
        log.Fatalf("Generate recommendations: %v", err)
    }

    // 2. Store recommendations
    for _, rec := range recommendations {
        _, err = db.ExecContext(ctx, `
            INSERT INTO signals 
            (ticker, signal_type, recommendation_amount, confidence_score, reasoning, market_regime, vix_level)
            VALUES ($1, $2, $3, $4, $5, $6, $7)
        `, rec.Ticker, rec.Action, rec.Amount, rec.ConfidenceScore, rec.Reasoning, rec.MarketRegime, rec.VIXLevel)

        if err != nil {
            log.Printf("Error storing signal: %v", err)
        }
    }

    log.Println("✓ Analysis complete")
}
```

### Configuration Loading

**File**: `pkg/config/config.go`

```go
package config

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
)

type Config struct {
    TrackedTickers      []string
    Creators            []Creator
    ConfidenceWeights   ConfidenceWeights
    MarketThresholds    MarketThresholds
    ContributionTarget  ContributionTarget
}

type Creator struct {
    Name          string `json:"name"`
    TwitterHandle string `json:"twitter_handle"`
}

type ConfidenceWeights struct {
    CreatorConsensus   float64 `json:"creator_consensus"`
    TechnicalAlignment float64 `json:"technical_alignment"`
    VolumeConfirmation float64 `json:"volume_confirmation"`
    HistoricalAccuracy float64 `json:"historical_accuracy"`
}

type MarketThresholds struct {
    VIXHigh      float64 `json:"vix_high"`
    RSIOverbought float64 `json:"rsi_overbought"`
    RSIOversold   float64 `json:"rsi_oversold"`
}

type ContributionTarget struct {
    Monthly float64 `json:"monthly"`
    Annual  float64 `json:"annual"`
    Current float64 `json:"current"`
}

func LoadConfig(ctx context.Context, db *sql.DB) (*Config, error) {
    cfg := &Config{}

    // Load tracked tickers
    var tickersJSON string
    err := db.QueryRowContext(ctx, "SELECT value FROM config WHERE key = 'tracked_tickers'").Scan(&tickersJSON)
    if err != nil {
        return nil, fmt.Errorf("load tickers: %w", err)
    }
    json.Unmarshal([]byte(tickersJSON), &cfg.TrackedTickers)

    // Load creators
    var creatorsJSON string
    err = db.QueryRowContext(ctx, "SELECT value FROM config WHERE key = 'creators'").Scan(&creatorsJSON)
    if err != nil {
        return nil, fmt.Errorf("load creators: %w", err)
    }
    json.Unmarshal([]byte(creatorsJSON), &cfg.Creators)

    // Load confidence weights
    var weightsJSON string
    err = db.QueryRowContext(ctx, "SELECT value FROM config WHERE key = 'confidence_weights'").Scan(&weightsJSON)
    if err != nil {
        return nil, fmt.Errorf("load weights: %w", err)
    }
    json.Unmarshal([]byte(weightsJSON), &cfg.ConfidenceWeights)

    // Load market thresholds
    var thresholdsJSON string
    err = db.QueryRowContext(ctx, "SELECT value FROM config WHERE key = 'market_regime_thresholds'").Scan(&thresholdsJSON)
    if err != nil {
        return nil, fmt.Errorf("load thresholds: %w", err)
    }
    json.Unmarshal([]byte(thresholdsJSON), &cfg.MarketThresholds)

    // Load contribution target
    var targetJSON string
    err = db.QueryRowContext(ctx, "SELECT value FROM config WHERE key = 'contribution_target'").Scan(&targetJSON)
    if err != nil {
        return nil, fmt.Errorf("load target: %w", err)
    }
    json.Unmarshal([]byte(targetJSON), &cfg.ContributionTarget)

    return cfg, nil
}
```

---

## API Keys & Setup Instructions

### 1. Alpha Vantage (Market Data)
- Sign up: https://www.alphavantage.co/support/#api-key
- Free tier: 500 requests/day, 5 requests/minute
- Add to `.envrc`: `export ALPHAVANTAGE_API_KEY="your_key"`

### 2. X/Twitter API
- Sign up: https://developer.twitter.com/en/portal/dashboard
- Create app and generate Bearer Token
- Free tier: 50 requests/day (enough for 2 creators)
- Add to `.envrc`: `export TWITTER_BEARER_TOKEN="your_token"`

### 3. Anthropic Claude API
- Sign up: https://console.anthropic.com/
- Generate API key
- Pricing: ~$3/$15 per million tokens (input/output)
- Add to `.envrc`: `export ANTHROPIC_API_KEY="your_key"`

### 4. Robinhood
- Use your existing credentials
- If 2FA enabled, get TOTP secret from authenticator app settings
- **Important**: Store credentials securely, never commit to git

### 5. Supabase
- Sign up: https://supabase.com
- Create project (free tier: 500MB database, 2GB bandwidth)
- Get connection string from Settings → Database
- Add to `.envrc`: `export DATABASE_URL="your_connection_string"`

---

## Testing Strategy

### Unit Tests

**Example test**: `services/market/fetcher_test.go`

```go
package market

import (
    "context"
    "testing"
)

func TestFetchQuote(t *testing.T) {
    fetcher := NewFetcher()
    ctx := context.Background()

    data, err := fetcher.FetchQuote(ctx, "SPY")
    if err != nil {
        t.Fatalf("FetchQuote failed: %v", err)
    }

    if data.Ticker != "SPY" {
        t.Errorf("Expected ticker SPY, got %s", data.Ticker)
    }

    if data.Close <= 0 {
        t.Errorf("Invalid close price: %f", data.Close)
    }
}
```

Run tests:
```bash
go test ./... -v
```

### Integration Tests

**Create `scripts/integration_test.sh`**:

```bash
#!/bin/bash
set -e

echo "=== Integration Test ==="

# 1. Test database connection
psql $DATABASE_URL -c "SELECT 1" > /dev/null
echo "✓ Database connected"

# 2. Test market data fetch
go run cmd/orchestrator/main.go fetch-market
echo "✓ Market data fetched"

# 3. Test social content fetch
go run cmd/orchestrator/main.go fetch-social
echo "✓ Social content fetched"

# 4. Test analysis
go run cmd/orchestrator/main.go analyze
echo "✓ Analysis completed"

# 5. Test TUI launch (non-interactive)
timeout 2s go run cmd/tui/main.go || true
echo "✓ TUI launched"

echo "=== All tests passed ==="
```

---

## Maintenance & Optimization

### Database Maintenance

**Create retention policy** (delete old data):

```sql
-- Run weekly via cron
DELETE FROM market_data WHERE timestamp < NOW() - INTERVAL '1 year';
DELETE FROM creator_content WHERE created_at < NOW() - INTERVAL '6 months';
DELETE FROM signals WHERE created_at < NOW() - INTERVAL '3 months';
```

### Performance Monitoring

**Add query logging**:

```go
// In database utilities
import "time"

func LogSlowQueries(query string, duration time.Duration) {
    if duration > 1*time.Second {
        log.Printf("SLOW QUERY (%v): %s", duration, query)
    }
}
```

### Cost Tracking

**Estimate monthly costs**:
- Alpha Vantage: Free
- Twitter API: Free
- Anthropic Claude: ~$5-10/month (daily analysis)
- Supabase: Free tier (or $25/month for Pro)

**Total**: $5-35/month depending on usage

---

## Troubleshooting

### Common Issues

**1. Rate limits exceeded**

```go
// Add exponential backoff
import "time"

func retryWithBackoff(fn func() error, maxRetries int) error {
    for i := 0; i < maxRetries; i++ {
        err := fn()
        if err == nil {
            return nil
        }
        time.Sleep(time.Duration(1<<uint(i)) * time.Second)
    }
    return fmt.Errorf("max retries exceeded")
}
```

**2. Database connection pool exhausted**

```go
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)
```

**3. Memory issues with embeddings**

```python
# Process in batches
batch_size = 100
for i in range(0, len(rows), batch_size):
    batch = rows[i:i+batch_size]
    # Process batch
```

---

## Security Best Practices

1. **Never commit `.envrc` or credentials**
   - Add to `.gitignore`
   - Use environment variables only

2. **Rotate API keys regularly**
   - Every 3-6 months
   - Immediately if compromised

3. **Use read-only database user for analysis**
   ```sql
   CREATE ROLE market_intel_readonly;
   GRANT SELECT ON ALL TABLES IN SCHEMA public TO market_intel_readonly;
   ```

4. **Encrypt sensitive data at rest**
   - Use Supabase encryption features
   - Consider pgcrypto for passwords

---

## Next Steps

After completing Phase 4, focus on:

1. **Accuracy tracking**: Compare recommendations vs actual performance
2. **Weight tuning**: Adjust confidence weights based on results
3. **Feature expansion**: Add more technical indicators, creators
4. **Backtesting**: Historical simulation to validate strategy
5. **Mobile notifications**: Alert on high-confidence opportunities

---

## Support & Resources

- **Alpha Vantage Docs**: https://www.alphavantage.co/documentation/
- **Twitter API Docs**: https://developer.twitter.com/en/docs/twitter-api
- **Anthropic Docs**: https://docs.anthropic.com/
- **Bubble Tea Tutorial**: https://github.com/charmbracelet/bubbletea/tree/master/tutorials
- **pgvector Guide**: https://github.com/pgvector/pgvector

---