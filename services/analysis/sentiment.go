// Package analysis provides sentiment analysis using Claude API.
// It analyzes creator content and market context to determine sentiment.
package analysis

import (
	"bytes"
	"context"
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
	claudeAPIURL     = "https://api.anthropic.com/v1/messages"
	claudeModel      = "claude-sonnet-4-20250514"
	claudeVersion    = "2023-06-01"
	maxTokens        = 1000
	requestTimeout   = 30 * time.Second
	rateLimitDelay   = 1 * time.Second // Cost control
)

// ClaudeRequest represents the API request structure
type ClaudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []ClaudeMessage `json:"messages"`
}

// ClaudeMessage represents a message in the conversation
type ClaudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ClaudeResponse represents the API response structure
type ClaudeResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// SentimentResult represents the analyzed sentiment for a ticker
type SentimentResult struct {
	Ticker     string  `json:"ticker"`
	Sentiment  string  `json:"sentiment"`  // bullish, bearish, neutral
	Confidence float64 `json:"confidence"` // 0.0 to 1.0
	Reasoning  string  `json:"reasoning"`
}

// Analyzer handles sentiment analysis using Claude API
type Analyzer struct {
	apiKey     string
	httpClient *http.Client
}

// NewAnalyzer creates a new sentiment analyzer
func NewAnalyzer() (*Analyzer, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set")
	}

	return &Analyzer{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
	}, nil
}

// AnalyzeSentiment analyzes sentiment for a ticker based on creator content and market context
func (a *Analyzer) AnalyzeSentiment(ctx context.Context, ticker string, creatorContent []string, marketContext string) (*SentimentResult, error) {
	// Build prompt
	prompt := buildSentimentPrompt(ticker, creatorContent, marketContext)

	// Create request
	reqBody := ClaudeRequest{
		Model:     claudeModel,
		MaxTokens: maxTokens,
		Messages: []ClaudeMessage{
			{
				Role:    "user",
				Content: prompt,
			},
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
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", claudeVersion)

	log.Printf("Analyzing sentiment for %s...", ticker)

	resp, err := a.httpClient.Do(req)
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

	var claudeResp ClaudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if claudeResp.Error != nil {
		return nil, fmt.Errorf("Claude API error: %s", claudeResp.Error.Message)
	}

	if len(claudeResp.Content) == 0 {
		return nil, fmt.Errorf("empty response from Claude API")
	}

	// Parse sentiment result from response
	result, err := parseSentimentResult(claudeResp.Content[0].Text, ticker)
	if err != nil {
		return nil, fmt.Errorf("parse sentiment result: %w", err)
	}

	log.Printf("Sentiment for %s: %s (%.0f%% confidence)", ticker, result.Sentiment, result.Confidence*100)
	return result, nil
}

// AnalyzeMultiple analyzes sentiment for multiple tickers with rate limiting
func (a *Analyzer) AnalyzeMultiple(ctx context.Context, tickers []string, contentByTicker map[string][]string, marketContext string) (map[string]*SentimentResult, []error) {
	results := make(map[string]*SentimentResult)
	var errors []error

	for i, ticker := range tickers {
		select {
		case <-ctx.Done():
			errors = append(errors, ctx.Err())
			return results, errors
		default:
		}

		content := contentByTicker[ticker]
		if len(content) == 0 {
			log.Printf("No content for %s, skipping sentiment analysis", ticker)
			continue
		}

		result, err := a.AnalyzeSentiment(ctx, ticker, content, marketContext)
		if err != nil {
			log.Printf("Error analyzing %s: %v", ticker, err)
			errors = append(errors, fmt.Errorf("%s: %w", ticker, err))
		} else {
			results[ticker] = result
		}

		// Rate limit (skip after last ticker)
		if i < len(tickers)-1 {
			select {
			case <-time.After(rateLimitDelay):
			case <-ctx.Done():
				errors = append(errors, ctx.Err())
				return results, errors
			}
		}
	}

	return results, errors
}

// buildSentimentPrompt creates the prompt for Claude
func buildSentimentPrompt(ticker string, creatorContent []string, marketContext string) string {
	contentStr := formatContent(creatorContent)

	return fmt.Sprintf(`Analyze the sentiment for stock ticker %s based on the following information.

## Creator Content (from market analysts):
%s

## Market Context:
%s

## Instructions:
1. Analyze the overall sentiment toward %s from the creator content
2. Consider the market context for additional perspective
3. Provide a sentiment rating (bullish, bearish, or neutral)
4. Provide a confidence score from 0.0 to 1.0
5. Explain your reasoning briefly

## Response Format:
Respond with ONLY valid JSON in this exact format (no markdown, no explanation outside JSON):
{
    "ticker": "%s",
    "sentiment": "bullish|bearish|neutral",
    "confidence": 0.0-1.0,
    "reasoning": "brief explanation (1-2 sentences)"
}`, ticker, contentStr, marketContext, ticker, ticker)
}

// formatContent formats content items for the prompt
func formatContent(content []string) string {
	if len(content) == 0 {
		return "No recent content available."
	}

	var builder strings.Builder
	for i, c := range content {
		builder.WriteString(fmt.Sprintf("%d. %s\n\n", i+1, c))
	}
	return builder.String()
}

// parseSentimentResult extracts structured result from Claude's response
func parseSentimentResult(responseText string, ticker string) (*SentimentResult, error) {
	// Clean response (remove markdown code blocks if present)
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	// Validate JSON before parsing
	if !json.Valid([]byte(responseText)) {
		return nil, fmt.Errorf("invalid JSON response: %s", responseText[:min(100, len(responseText))])
	}

	var result SentimentResult
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return nil, fmt.Errorf("unmarshal result: %w", err)
	}

	// Validate result
	if result.Ticker == "" {
		result.Ticker = ticker
	}

	validSentiments := map[string]bool{"bullish": true, "bearish": true, "neutral": true}
	if !validSentiments[result.Sentiment] {
		return nil, fmt.Errorf("invalid sentiment value: %s", result.Sentiment)
	}

	if result.Confidence < 0 || result.Confidence > 1 {
		return nil, fmt.Errorf("invalid confidence value: %f", result.Confidence)
	}

	return &result, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
