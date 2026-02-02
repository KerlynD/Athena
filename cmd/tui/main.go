// Package main provides the Terminal User Interface for the Market Intelligence Aggregator.
// Built with Bubble Tea and Lip Gloss for a beautiful, interactive experience.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "github.com/lib/pq"
)

// ASCII Art for ATHENA
const athenaLogo = `
    ___   ________  __  __  ______ _   __    ___
   /   | /_  __/ / / / / / / / __ \ | / /   /   |
  / /| |  / / / /_/ / / / / / / / /  |/ /   / /| |
 / ___ | / / / __  / /_/ / / /_/ / /|  /   / ___ |
/_/  |_|/_/ /_/ /_/\____/ \____/_/ |_/   /_/  |_|
                                                  
   M A R K E T   I N T E L L I G E N C E
`

// Styles
var (
	logoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			MarginTop(1)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(1, 2)

	buyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF00")).
			Bold(true)

	holdStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFF00"))

	waitStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6600"))

	highConfStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF00"))

	medConfStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFF00"))

	lowConfStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6600"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262")).
			MarginTop(1)

	statusOkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF00"))

	statusWarnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFF00"))

	statusErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF0000"))

	portfolioGainStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00FF00")).
				Bold(true)

	portfolioLossStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF0000")).
				Bold(true)
)

// Data models
type Recommendation struct {
	Ticker     string
	Action     string
	Amount     float64
	Confidence float64
	Reasoning  string
}

type MarketStatus struct {
	Regime     string
	VIX        float64
	LastUpdate time.Time
}

type ContentItem struct {
	Creator   string
	Content   string
	Sentiment string
	Tickers   []string
	PostedAt  time.Time
}

type PortfolioHolding struct {
	Ticker       string
	Quantity     float64
	AvgCost      float64
	CurrentPrice float64
	MarketValue  float64
	GainPercent  float64
}

type PortfolioSummary struct {
	TotalValue   float64
	TotalCost    float64
	TotalGain    float64
	GainPercent  float64
	Holdings     []PortfolioHolding
	LastUpdated  time.Time
}

type model struct {
	db              *sql.DB
	ready           bool
	width           int
	height          int
	activeTab       int
	recommendations []Recommendation
	marketStatus    MarketStatus
	recentContent   []ContentItem
	portfolio       PortfolioSummary
	recTable        table.Model
	holdingsTable   table.Model
	lastRefresh     time.Time
	err             error
}

type tickMsg time.Time
type dataMsg struct {
	recommendations []Recommendation
	marketStatus    MarketStatus
	recentContent   []ContentItem
	portfolio       PortfolioSummary
	err             error
}

func main() {
	db, err := connectDB()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	p := tea.NewProgram(initialModel(db), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

func connectDB() (*sql.DB, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

func initialModel(db *sql.DB) model {
	// Recommendations table
	recColumns := []table.Column{
		{Title: "Ticker", Width: 8},
		{Title: "Action", Width: 8},
		{Title: "Amount", Width: 12},
		{Title: "Confidence", Width: 12},
		{Title: "Reasoning", Width: 40},
	}

	recTable := table.New(
		table.WithColumns(recColumns),
		table.WithFocused(true),
		table.WithHeight(7),
	)

	// Holdings table
	holdingsColumns := []table.Column{
		{Title: "Ticker", Width: 8},
		{Title: "Shares", Width: 10},
		{Title: "Avg Cost", Width: 10},
		{Title: "Price", Width: 10},
		{Title: "Value", Width: 12},
		{Title: "Gain %", Width: 10},
	}

	holdingsTable := table.New(
		table.WithColumns(holdingsColumns),
		table.WithFocused(false),
		table.WithHeight(7),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Bold(false)
	recTable.SetStyles(s)
	holdingsTable.SetStyles(s)

	return model{
		db:            db,
		ready:         false,
		activeTab:     0,
		recTable:      recTable,
		holdingsTable: holdingsTable,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		loadData(m.db),
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func loadData(db *sql.DB) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var data dataMsg

		// Load recommendations
		data.recommendations = loadRecommendations(ctx, db)

		// Load market status
		data.marketStatus = loadMarketStatus(ctx, db)

		// Load recent content
		data.recentContent = loadRecentContent(ctx, db)

		// Load portfolio
		data.portfolio = loadPortfolio(ctx, db)

		return data
	}
}

func loadRecommendations(ctx context.Context, db *sql.DB) []Recommendation {
	rows, err := db.QueryContext(ctx, `
		SELECT ticker, action, amount, confidence_score, reasoning
		FROM signals
		WHERE created_at >= NOW() - INTERVAL '24 hours'
		ORDER BY created_at DESC
		LIMIT 10
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var recs []Recommendation
	for rows.Next() {
		var r Recommendation
		var reasoning sql.NullString
		if err := rows.Scan(&r.Ticker, &r.Action, &r.Amount, &r.Confidence, &reasoning); err != nil {
			continue
		}
		r.Reasoning = reasoning.String
		recs = append(recs, r)
	}
	return recs
}

func loadMarketStatus(ctx context.Context, db *sql.DB) MarketStatus {
	var status MarketStatus
	status.Regime = "unknown"

	// Try to get VIX
	var vixClose sql.NullFloat64
	var vixTime sql.NullTime
	db.QueryRowContext(ctx, `
		SELECT close, timestamp FROM market_data
		WHERE ticker = 'VIX' OR ticker = '^VIX'
		ORDER BY timestamp DESC LIMIT 1
	`).Scan(&vixClose, &vixTime)

	if vixClose.Valid {
		status.VIX = vixClose.Float64
		status.LastUpdate = vixTime.Time

		if status.VIX > 30 {
			status.Regime = "volatile"
		} else if status.VIX > 20 {
			status.Regime = "cautious"
		} else {
			status.Regime = "calm"
		}
	}

	return status
}

func loadRecentContent(ctx context.Context, db *sql.DB) []ContentItem {
	rows, err := db.QueryContext(ctx, `
		SELECT creator_name, content_text, COALESCE(sentiment, 'unanalyzed'), posted_at
		FROM creator_content
		ORDER BY created_at DESC
		LIMIT 5
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var items []ContentItem
	for rows.Next() {
		var item ContentItem
		if err := rows.Scan(&item.Creator, &item.Content, &item.Sentiment, &item.PostedAt); err != nil {
			continue
		}
		// Truncate content
		if len(item.Content) > 100 {
			item.Content = item.Content[:100] + "..."
		}
		items = append(items, item)
	}
	return items
}

func loadPortfolio(ctx context.Context, db *sql.DB) PortfolioSummary {
	var summary PortfolioSummary

	rows, err := db.QueryContext(ctx, `
		SELECT ticker, quantity, avg_cost, current_price, market_value, updated_at
		FROM holdings
		ORDER BY market_value DESC
	`)
	if err != nil {
		return summary
	}
	defer rows.Close()

	for rows.Next() {
		var h PortfolioHolding
		var updatedAt time.Time
		if err := rows.Scan(&h.Ticker, &h.Quantity, &h.AvgCost, &h.CurrentPrice, &h.MarketValue, &updatedAt); err != nil {
			continue
		}

		if h.AvgCost > 0 {
			h.GainPercent = (h.CurrentPrice - h.AvgCost) / h.AvgCost * 100
		}

		summary.Holdings = append(summary.Holdings, h)
		summary.TotalValue += h.MarketValue
		summary.TotalCost += h.AvgCost * h.Quantity
		summary.LastUpdated = updatedAt
	}

	summary.TotalGain = summary.TotalValue - summary.TotalCost
	if summary.TotalCost > 0 {
		summary.GainPercent = summary.TotalGain / summary.TotalCost * 100
	}

	return summary
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.activeTab = (m.activeTab + 1) % 4
		case "shift+tab":
			m.activeTab = (m.activeTab + 3) % 4
		case "r":
			return m, loadData(m.db)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

	case tickMsg:
		return m, tea.Batch(loadData(m.db), tickCmd())

	case dataMsg:
		m.recommendations = msg.recommendations
		m.marketStatus = msg.marketStatus
		m.recentContent = msg.recentContent
		m.portfolio = msg.portfolio
		m.lastRefresh = time.Now()
		m.err = msg.err

		// Update recommendations table rows
		recRows := make([]table.Row, len(m.recommendations))
		for i, r := range m.recommendations {
			recRows[i] = table.Row{
				r.Ticker,
				r.Action,
				fmt.Sprintf("$%.2f", r.Amount),
				fmt.Sprintf("%.0f%%", r.Confidence*100),
				truncate(r.Reasoning, 38),
			}
		}
		m.recTable.SetRows(recRows)

		// Update holdings table rows
		holdingsRows := make([]table.Row, len(m.portfolio.Holdings))
		for i, h := range m.portfolio.Holdings {
			holdingsRows[i] = table.Row{
				h.Ticker,
				fmt.Sprintf("%.4f", h.Quantity),
				fmt.Sprintf("$%.2f", h.AvgCost),
				fmt.Sprintf("$%.2f", h.CurrentPrice),
				fmt.Sprintf("$%.2f", h.MarketValue),
				fmt.Sprintf("%.2f%%", h.GainPercent),
			}
		}
		m.holdingsTable.SetRows(holdingsRows)
	}

	// Update the active table
	switch m.activeTab {
	case 0:
		m.recTable, cmd = m.recTable.Update(msg)
	case 1:
		m.holdingsTable, cmd = m.holdingsTable.Update(msg)
	}

	return m, cmd
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func (m model) View() string {
	if !m.ready {
		return "\n  Loading Athena..."
	}

	var b strings.Builder

	// ASCII Logo
	logo := logoStyle.Render(athenaLogo)
	b.WriteString(logo)

	// Date and status line
	dateLine := titleStyle.Render(fmt.Sprintf(" %s ", time.Now().Format("Monday, January 2, 2006 - 3:04 PM")))
	b.WriteString(dateLine + "\n")

	// Tabs
	tabs := m.renderTabs()
	b.WriteString(tabs + "\n")

	// Content based on active tab
	switch m.activeTab {
	case 0:
		b.WriteString(m.renderRecommendationsView())
	case 1:
		b.WriteString(m.renderPortfolioView())
	case 2:
		b.WriteString(m.renderMarketView())
	case 3:
		b.WriteString(m.renderContentView())
	}

	// Status bar
	b.WriteString(m.renderStatusBar())

	// Help
	help := helpStyle.Render("Tab: Switch views • r: Refresh • q: Quit")
	b.WriteString("\n" + help)

	return b.String()
}

func (m model) renderTabs() string {
	tabs := []string{"Recommendations", "Portfolio", "Market", "Content"}
	var rendered []string

	for i, tab := range tabs {
		style := lipgloss.NewStyle().Padding(0, 2)
		if i == m.activeTab {
			style = style.
				Background(lipgloss.Color("#7D56F4")).
				Foreground(lipgloss.Color("#FAFAFA")).
				Bold(true)
		} else {
			style = style.
				Foreground(lipgloss.Color("#626262"))
		}
		rendered = append(rendered, style.Render(tab))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

func (m model) renderRecommendationsView() string {
	var b strings.Builder

	header := headerStyle.Render("Today's Recommendations")
	b.WriteString(header + "\n\n")

	if len(m.recommendations) == 0 {
		noData := boxStyle.Render("No recommendations yet. Run 'orchestrator analyze' first.")
		b.WriteString(noData + "\n")
	} else {
		b.WriteString(m.recTable.View() + "\n")

		// Summary
		var totalAmount float64
		for _, r := range m.recommendations {
			if r.Action == "buy" {
				totalAmount += r.Amount
			}
		}
		summary := fmt.Sprintf("\nTotal allocation: $%.2f", totalAmount)
		b.WriteString(lipgloss.NewStyle().Bold(true).Render(summary))
	}

	return b.String()
}

func (m model) renderPortfolioView() string {
	var b strings.Builder

	header := headerStyle.Render("Portfolio Holdings")
	b.WriteString(header + "\n\n")

	if len(m.portfolio.Holdings) == 0 {
		noData := boxStyle.Render("No portfolio data. Run 'orchestrator fetch-portfolio' to sync with Robinhood.")
		b.WriteString(noData + "\n")
	} else {
		b.WriteString(m.holdingsTable.View() + "\n")

		// Portfolio summary
		gainStyle := portfolioGainStyle
		if m.portfolio.GainPercent < 0 {
			gainStyle = portfolioLossStyle
		}

		summaryBox := boxStyle.Render(fmt.Sprintf(
			"Total Value: $%.2f\n"+
				"Total Cost:  $%.2f\n"+
				"Total Gain:  %s\n"+
				"Last Updated: %s",
			m.portfolio.TotalValue,
			m.portfolio.TotalCost,
			gainStyle.Render(fmt.Sprintf("$%.2f (%.2f%%)", m.portfolio.TotalGain, m.portfolio.GainPercent)),
			m.portfolio.LastUpdated.Format("2006-01-02 15:04"),
		))
		b.WriteString("\n" + summaryBox)
	}

	return b.String()
}

func (m model) renderMarketView() string {
	var b strings.Builder

	header := headerStyle.Render("Market Status")
	b.WriteString(header + "\n\n")

	// Market regime indicator
	regimeStyle := statusOkStyle
	regimeEmoji := "🟢"
	if m.marketStatus.Regime == "volatile" {
		regimeStyle = statusErrorStyle
		regimeEmoji = "🔴"
	} else if m.marketStatus.Regime == "cautious" {
		regimeStyle = statusWarnStyle
		regimeEmoji = "🟡"
	}

	regimeBox := boxStyle.Render(fmt.Sprintf(
		"%s Market Regime: %s\n\n"+
			"   VIX: %.2f\n"+
			"   Last Update: %s",
		regimeEmoji,
		regimeStyle.Render(strings.ToUpper(m.marketStatus.Regime)),
		m.marketStatus.VIX,
		m.marketStatus.LastUpdate.Format("2006-01-02 15:04"),
	))
	b.WriteString(regimeBox + "\n")

	// Regime explanation
	var explanation string
	switch m.marketStatus.Regime {
	case "calm":
		explanation = "Normal market conditions. Standard allocation recommended."
	case "cautious":
		explanation = "Elevated volatility. Consider reducing position sizes."
	case "volatile":
		explanation = "High volatility. System may recommend waiting."
	default:
		explanation = "Unable to determine market regime. Run 'fetch-market' to get VIX data."
	}
	b.WriteString("\n" + helpStyle.Render(explanation))

	return b.String()
}

func (m model) renderContentView() string {
	var b strings.Builder

	header := headerStyle.Render("Recent Creator Content")
	b.WriteString(header + "\n\n")

	if len(m.recentContent) == 0 {
		noData := boxStyle.Render("No content yet. Use 'orchestrator add-content' to add creator posts.")
		b.WriteString(noData + "\n")
	} else {
		for _, item := range m.recentContent {
			sentimentStyle := holdStyle
			if item.Sentiment == "bullish" {
				sentimentStyle = buyStyle
			} else if item.Sentiment == "bearish" {
				sentimentStyle = waitStyle
			}

			contentBox := boxStyle.Render(fmt.Sprintf(
				"@%s [%s]\n%s\n\n%s",
				item.Creator,
				sentimentStyle.Render(item.Sentiment),
				item.Content,
				helpStyle.Render(item.PostedAt.Format("2006-01-02 15:04")),
			))
			b.WriteString(contentBox + "\n")
		}
	}

	return b.String()
}

func (m model) renderStatusBar() string {
	var status string
	if m.err != nil {
		status = statusErrorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	} else {
		status = statusOkStyle.Render("Connected") +
			helpStyle.Render(fmt.Sprintf(" • Last refresh: %s", m.lastRefresh.Format("15:04:05")))
	}
	return "\n" + status
}
