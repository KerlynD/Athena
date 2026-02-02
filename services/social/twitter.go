// Package social provides social media content scraping for the Market Intelligence Aggregator.
// It fetches tweets from tracked creators and extracts ticker mentions.
package social

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"
)

const (
	// Twitter API rate limits (free tier)
	rateLimitDelay = 5 * time.Second
	requestTimeout = 15 * time.Second
)

// Tweet represents a parsed tweet
type Tweet struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
	AuthorID  string    `json:"author_id"`
}

// twitterAPITweet represents the raw API response tweet format
type twitterAPITweet struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
	AuthorID  string `json:"author_id"`
}

// Client handles Twitter/X API interactions
type Client struct {
	bearerToken string
	httpClient  *http.Client
	baseURL     string
}

// NewClient creates a new Twitter API client
func NewClient() (*Client, error) {
	bearerToken := os.Getenv("TWITTER_BEARER_TOKEN")
	if bearerToken == "" {
		return nil, fmt.Errorf("TWITTER_BEARER_TOKEN is not set")
	}

	return &Client{
		bearerToken: bearerToken,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
		baseURL: "https://api.twitter.com/2",
	}, nil
}

// GetUserID retrieves the Twitter user ID for a username
func (c *Client) GetUserID(ctx context.Context, username string) (string, error) {
	endpoint := fmt.Sprintf("%s/users/by/username/%s", c.baseURL, username)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.bearerToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(result.Errors) > 0 {
		return "", fmt.Errorf("API error: %s", result.Errors[0].Message)
	}

	if result.Data.ID == "" {
		return "", fmt.Errorf("user not found: %s", username)
	}

	return result.Data.ID, nil
}

// FetchRecentTweets fetches recent tweets from a user
func (c *Client) FetchRecentTweets(ctx context.Context, username string, maxResults int) ([]Tweet, error) {
	// First, get user ID from username
	userID, err := c.GetUserID(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("get user ID: %w", err)
	}

	// Rate limit delay
	select {
	case <-time.After(rateLimitDelay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Fetch tweets
	endpoint := fmt.Sprintf("%s/users/%s/tweets", c.baseURL, userID)
	params := url.Values{}
	params.Add("max_results", fmt.Sprintf("%d", maxResults))
	params.Add("tweet.fields", "created_at,text,author_id")
	params.Add("exclude", "retweets,replies")

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.bearerToken)

	log.Printf("Fetching tweets from @%s...", username)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result struct {
		Data   []twitterAPITweet `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("API error: %s", result.Errors[0].Message)
	}

	// Parse tweets
	tweets := make([]Tweet, 0, len(result.Data))
	for _, t := range result.Data {
		createdAt, err := time.Parse(time.RFC3339, t.CreatedAt)
		if err != nil {
			log.Printf("Warning: could not parse created_at for tweet %s: %v", t.ID, err)
			createdAt = time.Now()
		}

		tweets = append(tweets, Tweet{
			ID:        t.ID,
			Text:      t.Text,
			CreatedAt: createdAt,
			AuthorID:  t.AuthorID,
		})
	}

	log.Printf("Fetched %d tweets from @%s", len(tweets), username)
	return tweets, nil
}

// FetchFromMultipleUsers fetches tweets from multiple users with rate limiting
func (c *Client) FetchFromMultipleUsers(ctx context.Context, usernames []string, maxResultsPerUser int) (map[string][]Tweet, []error) {
	results := make(map[string][]Tweet)
	var errors []error

	for i, username := range usernames {
		// Check context cancellation
		select {
		case <-ctx.Done():
			errors = append(errors, ctx.Err())
			return results, errors
		default:
		}

		tweets, err := c.FetchRecentTweets(ctx, username, maxResultsPerUser)
		if err != nil {
			log.Printf("Error fetching tweets from @%s: %v", username, err)
			errors = append(errors, fmt.Errorf("@%s: %w", username, err))
		} else {
			results[username] = tweets
		}

		// Rate limit delay (skip after last user)
		if i < len(usernames)-1 {
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

// Known ticker symbols for detection
var knownTickers = map[string]bool{
	"SPY":  true,
	"QQQ":  true,
	"VOO":  true,
	"VTI":  true,
	"PLTR": true,
	"AAPL": true,
	"MSFT": true,
	"GOOGL": true,
	"AMZN": true,
	"NVDA": true,
	"META": true,
	"TSLA": true,
}

// tickerRegex matches $TICKER patterns
var tickerRegex = regexp.MustCompile(`\$([A-Z]{1,5})\b`)

// ExtractTickers finds stock ticker mentions in text
func ExtractTickers(text string) []string {
	tickers := make(map[string]bool)

	// Match $TICKER patterns
	matches := tickerRegex.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) > 1 && match[1] != "" {
			tickers[match[1]] = true
		}
	}

	// Also check for known tickers without $ prefix
	for ticker := range knownTickers {
		if regexp.MustCompile(`\b` + ticker + `\b`).MatchString(text) {
			tickers[ticker] = true
		}
	}

	result := make([]string, 0, len(tickers))
	for ticker := range tickers {
		result = append(result, ticker)
	}

	return result
}

// RateLimitDelay returns the rate limit delay for external use
func RateLimitDelay() time.Duration {
	return rateLimitDelay
}
