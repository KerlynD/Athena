// Package social provides Twitter content storage for the Market Intelligence Aggregator.
package social

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"
)

// CreatorContent represents stored content from a creator
type CreatorContent struct {
	ID               int
	CreatorName      string
	Platform         string
	ContentID        string
	ContentText      string
	MentionedTickers []string
	Sentiment        sql.NullString
	ConfidenceScore  sql.NullFloat64
	PostedAt         time.Time
	CreatedAt        time.Time
}

// Store handles Twitter content persistence
type Store struct {
	db *sql.DB
}

// NewStore creates a new Twitter content store
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// SaveTweet stores a tweet in the database
func (s *Store) SaveTweet(ctx context.Context, creatorName string, tweet Tweet) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Extract tickers from tweet text
	tickers := ExtractTickers(tweet.Text)

	query := `
		INSERT INTO creator_content 
		(creator_name, platform, content_id, content_text, mentioned_tickers, posted_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (content_id) DO NOTHING
	`

	result, err := s.db.ExecContext(ctx, query,
		creatorName,
		"twitter",
		tweet.ID,
		tweet.Text,
		pq.Array(tickers),
		tweet.CreatedAt,
	)

	if err != nil {
		log.Printf("Error saving tweet %s: %v", tweet.ID, err)
		return fmt.Errorf("save tweet: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		log.Printf("Saved tweet %s from @%s (tickers: %v)", tweet.ID, creatorName, tickers)
	} else {
		log.Printf("Tweet %s already exists, skipped", tweet.ID)
	}

	return nil
}

// SaveTweets stores multiple tweets for a creator
func (s *Store) SaveTweets(ctx context.Context, creatorName string, tweets []Tweet) (int, []error) {
	saved := 0
	var errors []error

	for _, tweet := range tweets {
		if err := s.SaveTweet(ctx, creatorName, tweet); err != nil {
			errors = append(errors, fmt.Errorf("tweet %s: %w", tweet.ID, err))
		} else {
			saved++
		}
	}

	return saved, errors
}

// GetRecentContent retrieves recent content from all creators
func (s *Store) GetRecentContent(ctx context.Context, hours int) ([]CreatorContent, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	query := `
		SELECT id, creator_name, platform, content_id, content_text, 
		       mentioned_tickers, sentiment, confidence_score, posted_at, created_at
		FROM creator_content
		WHERE posted_at >= NOW() - $1::interval
		ORDER BY posted_at DESC
	`

	interval := fmt.Sprintf("%d hours", hours)
	rows, err := s.db.QueryContext(ctx, query, interval)
	if err != nil {
		return nil, fmt.Errorf("query recent content: %w", err)
	}
	defer rows.Close()

	var results []CreatorContent
	for rows.Next() {
		var content CreatorContent
		var tickers pq.StringArray

		if err := rows.Scan(
			&content.ID,
			&content.CreatorName,
			&content.Platform,
			&content.ContentID,
			&content.ContentText,
			&tickers,
			&content.Sentiment,
			&content.ConfidenceScore,
			&content.PostedAt,
			&content.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		content.MentionedTickers = []string(tickers)
		results = append(results, content)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return results, nil
}

// GetContentByTicker retrieves content mentioning a specific ticker
func (s *Store) GetContentByTicker(ctx context.Context, ticker string, limit int) ([]CreatorContent, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	query := `
		SELECT id, creator_name, platform, content_id, content_text, 
		       mentioned_tickers, sentiment, confidence_score, posted_at, created_at
		FROM creator_content
		WHERE $1 = ANY(mentioned_tickers)
		ORDER BY posted_at DESC
		LIMIT $2
	`

	rows, err := s.db.QueryContext(ctx, query, ticker, limit)
	if err != nil {
		return nil, fmt.Errorf("query content by ticker: %w", err)
	}
	defer rows.Close()

	var results []CreatorContent
	for rows.Next() {
		var content CreatorContent
		var tickers pq.StringArray

		if err := rows.Scan(
			&content.ID,
			&content.CreatorName,
			&content.Platform,
			&content.ContentID,
			&content.ContentText,
			&tickers,
			&content.Sentiment,
			&content.ConfidenceScore,
			&content.PostedAt,
			&content.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		content.MentionedTickers = []string(tickers)
		results = append(results, content)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return results, nil
}

// GetContentWithoutEmbeddings retrieves content that needs embeddings generated
func (s *Store) GetContentWithoutEmbeddings(ctx context.Context, limit int) ([]CreatorContent, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	query := `
		SELECT id, creator_name, platform, content_id, content_text, 
		       mentioned_tickers, sentiment, confidence_score, posted_at, created_at
		FROM creator_content
		WHERE embedding IS NULL
		ORDER BY created_at DESC
		LIMIT $1
	`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query content without embeddings: %w", err)
	}
	defer rows.Close()

	var results []CreatorContent
	for rows.Next() {
		var content CreatorContent
		var tickers pq.StringArray

		if err := rows.Scan(
			&content.ID,
			&content.CreatorName,
			&content.Platform,
			&content.ContentID,
			&content.ContentText,
			&tickers,
			&content.Sentiment,
			&content.ConfidenceScore,
			&content.PostedAt,
			&content.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		content.MentionedTickers = []string(tickers)
		results = append(results, content)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return results, nil
}

// UpdateSentiment updates the sentiment analysis for a content item
func (s *Store) UpdateSentiment(ctx context.Context, contentID int, sentiment string, confidence float64) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	query := `
		UPDATE creator_content
		SET sentiment = $1, confidence_score = $2
		WHERE id = $3
	`

	_, err := s.db.ExecContext(ctx, query, sentiment, confidence, contentID)
	if err != nil {
		return fmt.Errorf("update sentiment: %w", err)
	}

	return nil
}

// GetCreatorHandles retrieves the list of creator Twitter handles from config
func (s *Store) GetCreatorHandles(ctx context.Context) ([]string, error) {
	// Default creators if config not found
	defaultCreators := []string{"mobyinvest", "carbonfinance"}
	return defaultCreators, nil
}
