# AGENTS.md

**Market Intelligence Aggregator - Agent Guidelines**

This document provides rules, patterns, and historical context for AI agents working on this repository. It's designed to create a positive feedback loop by learning from past mistakes and architectural decisions.

---

## Project Overview

**What this is**: A CLI-based market intelligence system that aggregates real-time market data, creator sentiment, and portfolio tracking to provide daily investment recommendations for a Roth IRA.

**Tech Stack**:
- **Primary**: Go 1.21+ (market data, social scraping, orchestration, TUI)
- **Secondary**: Python 3.11+ (Robinhood integration, technical indicators, embeddings)
- **Database**: Supabase (PostgreSQL 15+ with pgvector extension)
- **TUI**: Bubble Tea + Lip Gloss
- **APIs**: Alpha Vantage, X/Twitter, Anthropic Claude

**Key Philosophy**: Simple, maintainable, production-ready code. Readability over performance. No over-engineering.

---

## Critical Rules

### 1. Language Boundaries - NEVER VIOLATE

| Task | Language | Reason |
|------|----------|--------|
| Portfolio fetching | Python | `robin_stocks` library only available in Python |
| Technical indicators | Python | `pandas-ta` is the industry standard |
| Embeddings generation | Python | `sentence-transformers` ecosystem |
| Market data fetching | Go | Primary language, async capabilities |
| Social media scraping | Go | Primary language, HTTP client efficiency |
| Sentiment analysis (Claude) | Go | Primary language, API integration |
| Confidence scoring | Go | Primary language, business logic |
| TUI | Go | Bubble Tea framework |
| Orchestration | Go | Primary language, process coordination |

**Rule**: If you're tempted to rewrite Python components in Go (or vice versa), STOP. These boundaries exist for library availability, not preference.

### 2. Database Schema - READ BEFORE ANY DB OPERATIONS

**Before making ANY database changes**:
1. Read `scripts/setup_db.sql`
2. Understand table relationships
3. Check for existing indexes
4. Verify extension requirements (pgvector, timescaledb)

**Forbidden Actions**:
- ❌ Modifying schema without updating `setup_db.sql`
- ❌ Adding columns without considering null constraints
- ❌ Creating tables without proper indexes
- ❌ Dropping extensions (vector, timescaledb)
- ❌ Changing primary key types

**Required Actions**:
- ✅ Always use migrations for schema changes
- ✅ Add `created_at TIMESTAMPTZ DEFAULT NOW()` to all tables
- ✅ Index foreign keys and frequently queried columns
- ✅ Use `TEXT[]` for array columns (e.g., `mentioned_tickers`)
- ✅ Use `vector(384)` for embeddings (sentence-transformers dimension)

### 3. Environment Variables - SECURITY CRITICAL

**Sensitive Variables** (NEVER hardcode, NEVER commit):
```bash
DATABASE_URL
ROBINHOOD_USERNAME
ROBINHOOD_PASSWORD
ROBINHOOD_TOTP
ALPHAVANTAGE_API_KEY
TWITTER_BEARER_TOKEN
ANTHROPIC_API_KEY
```

**Loading Pattern**:
```go
// Always use os.Getenv() with validation
apiKey := os.Getenv("ALPHAVANTAGE_API_KEY")
if apiKey == "" {
    return fmt.Errorf("ALPHAVANTAGE_API_KEY not set")
}
```

**Rule**: If you add a new API or credential, immediately update:
1. `.envrc.example` (with placeholder values)
2. `README.md` (setup instructions)
3. This file (under "Lessons Learned")

### 4. Error Handling - NO SILENT FAILURES

**Required Pattern**:
```go
// BAD
data, _ := fetchData()  // NEVER ignore errors

// GOOD
data, err := fetchData()
if err != nil {
    return fmt.Errorf("fetch data: %w", err)  // Always wrap with context
}
```

**Logging Standards**:
```go
import "log"

// Always log before returning errors
log.Printf("Error fetching %s: %v", ticker, err)
return fmt.Errorf("fetch quote: %w", err)
```

**Python Error Handling**:
```python
# Always wrap in try-except with specific exceptions
try:
    result = api_call()
except requests.exceptions.RequestException as e:
    print(json.dumps({'status': 'error', 'message': str(e)}))
    sys.exit(1)
```

### 5. API Rate Limits - RESPECT THEM

| API | Limit | Sleep Between Calls |
|-----|-------|---------------------|
| Alpha Vantage | 5/min, 500/day | 15 seconds |
| Twitter | 50/day (free tier) | 5 seconds |
| Claude | No hard limit | 1 second (cost control) |
| Robinhood | Unofficial (be gentle) | 2 seconds |

**Required Pattern**:
```go
for _, ticker := range tickers {
    data, err := fetcher.FetchQuote(ctx, ticker)
    // ... handle data ...
    
    time.Sleep(15 * time.Second)  // REQUIRED for Alpha Vantage
}
```

**Rule**: If you see a 429 (rate limit) error, add exponential backoff:
```go
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

### 6. Testing - NON-NEGOTIABLE

**Before committing ANY code**:
```bash
# Go tests
go test ./... -v

# Python tests (if applicable)
pytest services/

# Integration test
./scripts/integration_test.sh
```

**Test Coverage Requirements**:
- All public functions in `services/` must have unit tests
- All database operations must have integration tests
- All API clients must have mock tests

**Mock External APIs**:
```go
// Use interfaces for testability
type MarketDataFetcher interface {
    FetchQuote(ctx context.Context, ticker string) (*MarketData, error)
}

// Create mock for testing
type MockFetcher struct {
    mockData *MarketData
    mockError error
}
```

### 7. Dependencies - MINIMAL APPROACH

**Go Dependencies** (current approved list):
```
github.com/charmbracelet/bubbletea
github.com/charmbracelet/bubbles
github.com/charmbracelet/lipgloss
github.com/lib/pq
```

**Python Dependencies** (current approved list):
```
robin-stocks==3.0.1
psycopg2-binary==2.9.9
pandas==2.1.4
pandas-ta==0.3.14b
sentence-transformers==2.2.2
```

**Rule**: Before adding ANY new dependency:
1. Search for existing Go stdlib solution
2. Check if existing dependency can handle it
3. Verify license compatibility (prefer MIT/Apache 2.0)
4. Document reason in this file

### 8. File Organization - STRICT STRUCTURE

```
market-intelligence/
├── cmd/                    # Executable commands only
│   ├── orchestrator/       # Main daily runner
│   └── tui/                # Terminal UI
├── services/               # Business logic (organized by domain)
│   ├── market/             # Market data (Go)
│   ├── social/             # Social media (Go)
│   ├── robinhood/          # Portfolio (Python)
│   ├── analysis/           # Analysis logic (mixed)
│   └── engine/             # Recommendation engine (Go)
├── pkg/                    # Shared utilities
│   ├── database/           # DB connection, migrations
│   └── config/             # Configuration loading
├── scripts/                # Automation scripts
├── deployments/            # Docker, systemd configs
└── .cursor/                # Agent guidelines (this file)
```

**Rules**:
- ❌ Never put business logic in `cmd/`
- ❌ Never put executable code in `pkg/`
- ✅ Keep `services/` organized by domain, not by language
- ✅ Put shared code in `pkg/`
- ✅ Keep scripts in `scripts/`, not root

### 9. Database Connection Pooling

**Always configure connection pools**:
```go
db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
if err != nil {
    return err
}

// REQUIRED settings
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)
```

**Rule**: Never create a new `sql.DB` instance per request. Use a singleton pattern or dependency injection.

### 10. Context Timeouts - ALWAYS SET

```go
// BAD
ctx := context.Background()

// GOOD
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
```

**Required for**:
- All HTTP requests
- All database queries
- All API calls

---

## Lessons Learned (Feedback Loop)

### Mistakes Made & Solutions

#### ❌ Mistake 1: Used `context.Background()` without timeout in HTTP client
**Date**: [To be filled by agent]  
**Impact**: API calls hung indefinitely  
**Solution**: 
```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
```
**Prevention**: Always use `context.WithTimeout` for I/O operations

---

#### ❌ Mistake 2: Forgot to close database connections
**Date**: [To be filled by agent]  
**Impact**: Connection pool exhausted after ~20 queries  
**Solution**:
```go
conn, err := db.Conn(ctx)
if err != nil {
    return err
}
defer conn.Close()  // CRITICAL
```
**Prevention**: Add `defer conn.Close()` immediately after acquiring connection

---

#### ❌ Mistake 3: Hardcoded API keys in test files
**Date**: [To be filled by agent]  
**Impact**: Credentials leaked in git history  
**Solution**: Used environment variables + `.envrc` with direnv  
**Prevention**: Run `git secrets --scan` before commits

---

#### ❌ Mistake 4: Tried to run systemd on macOS
**Date**: [To be filled by agent]  
**Impact**: Deployment script failed  
**Solution**: Use `launchd` on macOS, `systemd` on Linux  
**Prevention**: Check `uname -s` in scripts:
```bash
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS (launchd)
else
    # Linux (systemd)
fi
```

---

#### ❌ Mistake 5: Exceeded Twitter API rate limit
**Date**: [To be filled by agent]  
**Impact**: 429 errors, blocked for 15 minutes  
**Solution**: Added 5-second sleep between requests  
**Prevention**: Document rate limits in code comments

---

#### ❌ Mistake 6: SQL injection vulnerability in dynamic queries
**Date**: [To be filled by agent]  
**Impact**: Security risk  
**Solution**: Always use parameterized queries:
```go
// BAD
query := fmt.Sprintf("SELECT * FROM users WHERE id = %s", userID)

// GOOD
query := "SELECT * FROM users WHERE id = $1"
db.QueryContext(ctx, query, userID)
```
**Prevention**: Never use `fmt.Sprintf` for SQL queries

---

#### ❌ Mistake 7: Ignored `pgvector` dimension mismatch
**Date**: [To be filled by agent]  
**Impact**: Embeddings failed to insert (dimension error)  
**Solution**: Verified sentence-transformers output (384 dimensions) matches schema:
```sql
embedding vector(384)  -- Must match model output
```
**Prevention**: Always check model documentation for embedding dimensions

---

#### ❌ Mistake 8: Used synchronous HTTP calls in orchestrator
**Date**: [To be filled by agent]  
**Impact**: Daily run took 10+ minutes  
**Solution**: Implemented goroutines with wait groups:
```go
var wg sync.WaitGroup
for _, ticker := range tickers {
    wg.Add(1)
    go func(t string) {
        defer wg.Done()
        // Fetch data
    }(ticker)
}
wg.Wait()
```
**Prevention**: Profile long-running operations, parallelize I/O

---

#### ❌ Mistake 9: Forgot to handle NULL values from database
**Date**: [To be filled by agent]  
**Impact**: Panic on `Scan()` when optional fields were NULL  
**Solution**: Use `sql.NullString`, `sql.NullFloat64`:
```go
var rsi sql.NullFloat64
err := row.Scan(&rsi)
if rsi.Valid {
    // Use rsi.Float64
}
```
**Prevention**: Check schema for nullable columns, use `sql.Null*` types

---

#### ❌ Mistake 10: Didn't validate JSON from Claude API
**Date**: [To be filled by agent]  
**Impact**: Unmarshaling failed silently, returned empty struct  
**Solution**: Validate JSON before unmarshaling:
```go
if !json.Valid([]byte(responseText)) {
    return fmt.Errorf("invalid JSON from Claude API")
}
```
**Prevention**: Always validate external API responses

---

#### ❌ Mistake 11: Used reserved word `timestamp` in RETURNS TABLE
**Date**: 2026-02-01  
**Impact**: SQL syntax error in Supabase when creating functions  
**Solution**: Rename output columns with `out_` prefix:
```sql
-- BAD
RETURNS TABLE (
    ticker VARCHAR(10),
    timestamp TIMESTAMPTZ,  -- Reserved word!
    open DECIMAL(18, 4)     -- Also problematic
)

-- GOOD
RETURNS TABLE (
    out_ticker VARCHAR(10),
    out_timestamp TIMESTAMPTZ,
    out_open DECIMAL(18, 4)
)
```
**Prevention**: In PostgreSQL function RETURNS TABLE, prefix output columns with `out_` to avoid reserved word conflicts

---

#### ❌ Mistake 12: Assumed TimescaleDB was available in Supabase
**Date**: 2026-02-01  
**Impact**: TimescaleDB extension not available - hypertable commands would fail  
**Solution**: Commented out TimescaleDB-specific commands, use standard PostgreSQL indexes  
**Prevention**: Supabase no longer supports TimescaleDB. Use standard `(ticker, timestamp DESC)` indexes for time-series queries - performance is still good for our scale

---

#### ❌ Mistake 13: Assumed Twitter Free API tier allows reading tweets
**Date**: 2026-02-01  
**Impact**: 402 Payment Required error when fetching tweets  
**Solution**: Twitter/X restricted free tier in 2023 - now only allows posting, not reading. Options:
1. Skip Twitter and use other signals (technical indicators, market data)
2. Subscribe to Twitter Basic tier ($100/month)
3. Use alternative free APIs (Reddit, StockTwits, RSS feeds)
4. Allow manual content input for analysis
**Prevention**: Always check current API pricing/limits before building integrations. Free tiers change frequently.

---

### Architecture Decisions

#### ✅ Decision 1: Kept Python for Robinhood instead of porting to Go
**Reason**: `robin_stocks` is the only reliable library, no Go alternative  
**Trade-off**: Multi-language complexity vs reinventing the wheel  
**Outcome**: Accepted complexity, saves hundreds of hours

---

#### ✅ Decision 2: Used Supabase instead of self-hosted Postgres
**Reason**: pgvector support out of the box, free tier sufficient  
**Trade-off**: Vendor lock-in vs operational simplicity  
**Outcome**: Can migrate later if needed, focus on features now

---

#### ✅ Decision 3: Chose Bubble Tea over CLI flags for UX
**Reason**: Better user experience for daily review of recommendations  
**Trade-off**: Complexity vs usability  
**Outcome**: TUI is intuitive, worth the effort

---

#### ✅ Decision 4: Stored raw tweets instead of just sentiment
**Reason**: Enables reprocessing with better models later  
**Trade-off**: Storage space vs flexibility  
**Outcome**: Historical data invaluable for backtesting

---

#### ✅ Decision 5: Used systemd timer instead of cron
**Reason**: Better logging, dependency management, restart policies  
**Trade-off**: Platform-specific vs universal  
**Outcome**: Worth it for production reliability

---

#### ✅ Decision 6: Separated embeddings generation from content ingestion
**Reason**: Decouples data collection from analysis, allows batch processing  
**Trade-off**: Extra step vs cleaner architecture  
**Outcome**: Easier to debug, can regenerate embeddings without refetching

---

#### ✅ Decision 7: Used confidence scoring instead of binary signals
**Reason**: Provides nuance (78% vs 92% confidence matters)  
**Trade-off**: Complexity vs better decisions  
**Outcome**: User can adjust risk tolerance based on score

---

#### ✅ Decision 8: Implemented market regime detector
**Reason**: Prevents buying during high volatility (risk management)  
**Trade-off**: May miss opportunities vs protects capital  
**Outcome**: Aligns with long-term DCA strategy

---

## Code Patterns (Copy-Paste Approved)

### Pattern 1: Database Query with Context

```go
func fetchLatestIndicators(ctx context.Context, db *sql.DB, ticker string) (*TechnicalIndicators, error) {
    query := `
        SELECT rsi_14, sma_50, sma_200, macd, macd_signal, atr_14
        FROM technical_indicators
        WHERE ticker = $1
        ORDER BY timestamp DESC
        LIMIT 1
    `
    
    var indicators TechnicalIndicators
    err := db.QueryRowContext(ctx, query, ticker).Scan(
        &indicators.RSI14,
        &indicators.SMA50,
        &indicators.SMA200,
        &indicators.MACD,
        &indicators.MACDSignal,
        &indicators.ATR14,
    )
    
    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("no indicators found for %s", ticker)
    }
    if err != nil {
        return nil, fmt.Errorf("query indicators: %w", err)
    }
    
    return &indicators, nil
}
```

### Pattern 2: HTTP Client with Timeout

```go
func makeAPIRequest(ctx context.Context, url string, bearerToken string) ([]byte, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }
    
    req.Header.Set("Authorization", "Bearer "+bearerToken)
    req.Header.Set("Content-Type", "application/json")
    
    client := &http.Client{
        Timeout: 15 * time.Second,
    }
    
    resp, err := client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("execute request: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }
    
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("read response: %w", err)
    }
    
    return body, nil
}
```

### Pattern 3: Goroutine with Error Collection

```go
func fetchMultipleTickers(ctx context.Context, tickers []string) (map[string]*MarketData, error) {
    type result struct {
        ticker string
        data   *MarketData
        err    error
    }
    
    results := make(chan result, len(tickers))
    var wg sync.WaitGroup
    
    for _, ticker := range tickers {
        wg.Add(1)
        go func(t string) {
            defer wg.Done()
            
            data, err := fetchQuote(ctx, t)
            results <- result{ticker: t, data: data, err: err}
        }(ticker)
    }
    
    go func() {
        wg.Wait()
        close(results)
    }()
    
    dataMap := make(map[string]*MarketData)
    var errs []error
    
    for r := range results {
        if r.err != nil {
            errs = append(errs, fmt.Errorf("%s: %w", r.ticker, r.err))
            continue
        }
        dataMap[r.ticker] = r.data
    }
    
    if len(errs) > 0 {
        return dataMap, fmt.Errorf("errors: %v", errs)
    }
    
    return dataMap, nil
}
```

### Pattern 4: Python Script Output (for Go consumption)

```python
#!/usr/bin/env python3
import json
import sys

def main():
    try:
        # Do work
        result = do_work()
        
        # Output JSON for Go to parse
        print(json.dumps({
            'status': 'success',
            'data': result,
            'timestamp': datetime.now().isoformat()
        }))
        
    except Exception as e:
        # Error handling
        print(json.dumps({
            'status': 'error',
            'message': str(e)
        }), file=sys.stderr)
        sys.exit(1)

if __name__ == '__main__':
    main()
```

### Pattern 5: Graceful Shutdown

```go
func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    // Setup signal handling
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    
    go func() {
        <-sigChan
        log.Println("Shutting down gracefully...")
        cancel()
    }()
    
    // Run main logic
    if err := run(ctx); err != nil {
        log.Fatal(err)
    }
}
```

---

## Pre-Commit Checklist

Before committing ANY code, verify:

- [ ] All tests pass (`go test ./... -v`)
- [ ] No hardcoded credentials or API keys
- [ ] All errors are wrapped with context (`fmt.Errorf("operation: %w", err)`)
- [ ] All database connections have `defer conn.Close()`
- [ ] All HTTP requests have context timeouts
- [ ] Rate limits are respected (sleep between API calls)
- [ ] Logging is present for all errors
- [ ] Schema changes are reflected in `scripts/setup_db.sql`
- [ ] New dependencies are documented in this file
- [ ] Code follows existing patterns (check this file first)

---

## When to Update This File

**Add to "Lessons Learned" when**:
- You encounter a runtime error (not caught by tests)
- You discover a security issue
- You exceed rate limits
- You make an OS-specific mistake
- You experience data loss or corruption

**Add to "Architecture Decisions" when**:
- You choose between 2+ viable approaches
- You add a new external dependency
- You change database schema significantly
- You modify the orchestration workflow

**Add to "Code Patterns" when**:
- You solve a problem that will recur (e.g., retry logic)
- You implement a non-obvious integration
- You create a useful abstraction

---

## Agent Self-Improvement Protocol

After EVERY task completion:

1. **Reflect**: Did you make any mistakes? Did you violate any rules in this file?
2. **Document**: If yes, add to "Lessons Learned" with:
   - What you did wrong
   - Why it was wrong
   - How you fixed it
   - How to prevent it
3. **Patterns**: Did you write code that should be reusable? Add to "Code Patterns"
4. **Commit**: Update this file as part of your commit

**Example commit message**:

feat: Add market regime detector

- Implemented VIX-based volatility detection
- Added RSI overbought/oversold thresholds
- Updated AGENTS.md with rate limit mistake (Twitter API)


---

## Emergency Procedures

### If Production Breaks

1. **Check logs**: `journalctl -u market-intel.service -n 100`
2. **Check database**: `psql $DATABASE_URL -c "SELECT COUNT(*) FROM signals"`
3. **Verify API keys**: `env | grep -E "(API_KEY|TOKEN)"`
4. **Revert last change**: `git revert HEAD`
5. **Restart service**: `sudo systemctl restart market-intel.service`

### If Database Corrupted

1. **Backup immediately**: `pg_dump $DATABASE_URL > backup_$(date +%Y%m%d).sql`
2. **Check schema**: `psql $DATABASE_URL -c "\d+"`
3. **Restore from backup**: `psql $DATABASE_URL < backup_YYYYMMDD.sql`
4. **Re-run migrations**: `psql $DATABASE_URL -f scripts/setup_db.sql`

### If Rate Limited

1. **Stop orchestrator**: `sudo systemctl stop market-intel.timer`
2. **Wait for reset**: Check API documentation for reset time
3. **Increase sleep intervals**: Update in code
4. **Resume**: `sudo systemctl start market-intel.timer`

---

## Contact & Escalation

**For questions about**:
- Architecture decisions → Check "Architecture Decisions" section first
- Code patterns → Check "Code Patterns" section first
- Errors → Check "Lessons Learned" section first

**If stuck**:
1. Read relevant section in `PRD.md`
2. Check existing code for similar patterns
3. Search this file for keywords
4. Ask user for clarification (with specific context)

---

**Last Updated**: [Auto-generated by agent on each commit]  
**Version**: 1.0.0  
**Total Lessons Learned**: 10  
**Total Architecture Decisions**: 8  
**Total Code Patterns**: 5

---

*This file is a living document. Every agent interaction should make it better. If you found a mistake or learned something new, update this file immediately.*