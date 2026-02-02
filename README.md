# Athena - Market Intelligence Aggregator

A CLI-based market intelligence system that aggregates real-time market data, creator sentiment analysis, and portfolio tracking to provide daily investment recommendations for a Roth IRA.

## Features

- **Portfolio Tracking**: Syncs with Robinhood to track current holdings
- **Market Data**: Fetches real-time and historical data from Alpha Vantage
- **Social Sentiment**: Analyzes tweets from financial creators (Moby Invest, Carbon Finance)
- **Technical Analysis**: Calculates RSI, SMA, MACD, and other indicators
- **AI Sentiment Analysis**: Uses Claude API for nuanced sentiment interpretation
- **Semantic Search**: Vector embeddings for historical context retrieval
- **Confidence Scoring**: Weighted scoring system for recommendation quality
- **Beautiful TUI**: Terminal interface built with Bubble Tea
- **Automated Daily Runs**: Scheduled execution at 7 AM on trading days

## Architecture

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
       │                   │
       ▼                   ▼
┌─────────────────────────────────┐
│   Supabase (PostgreSQL)         │
│   - pgvector (embeddings)       │
│   - Time-series data            │
└─────────────────────────────────┘
```

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Primary Language | Go 1.21+ |
| Portfolio Integration | Python 3.11+ (robin_stocks) |
| Database | Supabase (PostgreSQL 15+ with pgvector) |
| Embeddings | sentence-transformers (all-MiniLM-L6-v2) |
| Market Data | Alpha Vantage API |
| Social Media | X/Twitter API v2 |
| LLM Analysis | Claude 3.5 Sonnet |
| Technical Indicators | pandas-ta |
| TUI Framework | Bubble Tea + Lip Gloss |

## Quick Start

### Prerequisites

- Go 1.21+
- Python 3.11+
- PostgreSQL client (psql)
- API keys (see below)

### 1. Clone and Setup

```bash
cd "c:\Users\Angel\Cursor Projects\Finance\Athena"

# Create Python virtual environment
python -m venv venv

# Activate virtual environment
# Windows:
.\venv\Scripts\Activate.ps1
# Linux/macOS:
source venv/bin/activate

# Install Python dependencies
pip install -r services/robinhood/requirements.txt
pip install -r services/analysis/requirements.txt

# Install Go dependencies
go mod download
```

### 2. Configure Environment

```bash
# Copy example environment file
cp .envrc.example .envrc

# Edit .envrc with your API keys and credentials
# (Use your preferred editor)
```

Required environment variables:
- `DATABASE_URL` - Supabase PostgreSQL connection string
- `ALPHAVANTAGE_API_KEY` - [Get here](https://www.alphavantage.co/support/#api-key)
- `TWITTER_BEARER_TOKEN` - [Get here](https://developer.twitter.com/en/portal/dashboard)
- `ANTHROPIC_API_KEY` - [Get here](https://console.anthropic.com/)
- `ROBINHOOD_USERNAME` - Your Robinhood email
- `ROBINHOOD_PASSWORD` - Your Robinhood password
- `ROBINHOOD_TOTP` - (Optional) TOTP secret if 2FA enabled

### 3. Setup Database

1. Create a project at [Supabase](https://supabase.com)
2. Enable the `vector` extension in the Supabase Dashboard
3. Run the schema:

```bash
psql $DATABASE_URL -f scripts/setup_db.sql
```

### 4. Run the System

```bash
# Run complete daily workflow
# Windows:
.\scripts\daily_run.ps1

# Linux/macOS:
./scripts/daily_run.sh

# Or run individual commands:
go run ./cmd/orchestrator fetch-market    # Fetch market data
go run ./cmd/orchestrator fetch-social    # Fetch tweets
go run ./cmd/orchestrator analyze         # Generate recommendations

# Launch TUI
go run ./cmd/tui
```

## Project Structure

```
athena/
├── cmd/
│   ├── orchestrator/     # Main daily runner
│   └── tui/              # Terminal UI
├── services/
│   ├── market/           # Market data fetcher (Go)
│   ├── social/           # Twitter scraper (Go)
│   ├── robinhood/        # Portfolio fetcher (Python)
│   ├── analysis/         # Sentiment & indicators (Go + Python)
│   └── engine/           # Recommendation engine (Go)
├── pkg/
│   ├── database/         # Database utilities
│   └── config/           # Configuration loading
├── scripts/
│   ├── setup_db.sql      # Database schema
│   ├── daily_run.sh      # Linux/macOS runner
│   └── daily_run.ps1     # Windows runner
├── deployments/
│   ├── docker-compose.yml
│   └── Dockerfile
├── .envrc.example        # Environment template
└── go.mod
```

## Development

### Running Tests

```bash
# Go tests
go test ./... -v

# Integration tests (requires env vars)
./scripts/integration_test.sh
```

### Building

```bash
# Build orchestrator
go build -o bin/orchestrator ./cmd/orchestrator

# Build TUI
go build -o bin/tui ./cmd/tui
```

### Docker Development

```bash
# Start local PostgreSQL with pgvector
docker-compose -f deployments/docker-compose.yml up -d

# Connect to local database
psql postgresql://postgres:postgres@localhost:5432/athena
```

## API Rate Limits

| API | Limit | Delay Between Calls |
|-----|-------|---------------------|
| Alpha Vantage | 5/min, 500/day | 15 seconds |
| Twitter | 50/day (free tier) | 5 seconds |
| Claude | No hard limit | 1 second (cost control) |

## Cost Estimates

| Service | Monthly Cost |
|---------|-------------|
| Supabase | Free tier / $25 Pro |
| Alpha Vantage | Free |
| Twitter API | Free tier |
| Claude API | ~$5-10 |
| **Total** | **$5-35** |

## Security Notes

- **Never commit `.envrc`** - Contains secrets
- API keys stored in environment variables only
- Database credentials rotated regularly
- Robinhood 2FA recommended

## Implementation Status

- [x] Phase 1: Foundation (project structure, database, basic fetchers)
- [ ] Phase 2: Analysis Engine (technical indicators, embeddings, sentiment)
- [ ] Phase 3: Recommendation Engine (market regime, allocation, signals)
- [ ] Phase 4: TUI & Orchestration (Bubble Tea UI, automation)

## License

Private - For personal use only.

## Resources

- [PRD Documentation](.cursor/PRD.md)
- [Agent Guidelines](.cursor/AGENTS.md)
- [Alpha Vantage Docs](https://www.alphavantage.co/documentation/)
- [Twitter API Docs](https://developer.twitter.com/en/docs/twitter-api)
- [Anthropic Docs](https://docs.anthropic.com/)
- [Bubble Tea Tutorial](https://github.com/charmbracelet/bubbletea/tree/master/tutorials)
