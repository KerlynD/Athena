// Package analysis provides semantic search using pgvector embeddings.
package analysis

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SimilarContent represents a search result from semantic search
type SimilarContent struct {
	ID          int
	CreatorName string
	ContentText string
	Sentiment   string
	Similarity  float64
	PostedAt    time.Time
}

// SemanticSearcher handles vector similarity searches
type SemanticSearcher struct {
	db *sql.DB
}

// NewSemanticSearcher creates a new semantic searcher
func NewSemanticSearcher(db *sql.DB) *SemanticSearcher {
	return &SemanticSearcher{db: db}
}

// SearchSimilarContent finds content similar to the given embedding
// Uses cosine similarity via pgvector's <=> operator
func (s *SemanticSearcher) SearchSimilarContent(ctx context.Context, embedding []float64, limit int, minSimilarity float64) ([]SimilarContent, error) {
	// Validate embedding dimensions first (before context operations)
	if len(embedding) != 384 {
		return nil, fmt.Errorf("embedding must have 384 dimensions, got %d", len(embedding))
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Build embedding string for pgvector
	embeddingStr := "["
	for i, v := range embedding {
		if i > 0 {
			embeddingStr += ","
		}
		embeddingStr += fmt.Sprintf("%f", v)
	}
	embeddingStr += "]"

	query := `
		SELECT 
			id, 
			creator_name, 
			content_text, 
			COALESCE(sentiment, 'unknown') as sentiment,
			1 - (embedding <=> $1::vector) as similarity,
			posted_at
		FROM creator_content
		WHERE embedding IS NOT NULL
			AND 1 - (embedding <=> $1::vector) > $2
		ORDER BY embedding <=> $1::vector
		LIMIT $3
	`

	rows, err := s.db.QueryContext(ctx, query, embeddingStr, minSimilarity, limit)
	if err != nil {
		return nil, fmt.Errorf("query similar content: %w", err)
	}
	defer rows.Close()

	var results []SimilarContent
	for rows.Next() {
		var item SimilarContent
		if err := rows.Scan(
			&item.ID,
			&item.CreatorName,
			&item.ContentText,
			&item.Sentiment,
			&item.Similarity,
			&item.PostedAt,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		results = append(results, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return results, nil
}

// SearchByTicker finds similar historical content for a ticker
func (s *SemanticSearcher) SearchByTicker(ctx context.Context, ticker string, limit int) ([]SimilarContent, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Get the most recent content about this ticker that has an embedding
	var refEmbedding string
	err := s.db.QueryRowContext(ctx, `
		SELECT embedding::text
		FROM creator_content
		WHERE $1 = ANY(mentioned_tickers)
			AND embedding IS NOT NULL
		ORDER BY posted_at DESC
		LIMIT 1
	`, ticker).Scan(&refEmbedding)

	if err == sql.ErrNoRows {
		return nil, nil // No reference content found
	}
	if err != nil {
		return nil, fmt.Errorf("get reference embedding: %w", err)
	}

	// Find similar historical content (excluding the reference)
	query := `
		SELECT 
			id, 
			creator_name, 
			content_text, 
			COALESCE(sentiment, 'unknown') as sentiment,
			1 - (embedding <=> $1::vector) as similarity,
			posted_at
		FROM creator_content
		WHERE embedding IS NOT NULL
			AND 1 - (embedding <=> $1::vector) > 0.5
			AND 1 - (embedding <=> $1::vector) < 1.0
		ORDER BY embedding <=> $1::vector
		LIMIT $2
	`

	rows, err := s.db.QueryContext(ctx, query, refEmbedding, limit)
	if err != nil {
		return nil, fmt.Errorf("query similar content: %w", err)
	}
	defer rows.Close()

	var results []SimilarContent
	for rows.Next() {
		var item SimilarContent
		if err := rows.Scan(
			&item.ID,
			&item.CreatorName,
			&item.ContentText,
			&item.Sentiment,
			&item.Similarity,
			&item.PostedAt,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		results = append(results, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return results, nil
}

// GetHistoricalContext retrieves historical context for sentiment analysis
func (s *SemanticSearcher) GetHistoricalContext(ctx context.Context, ticker string) (string, error) {
	results, err := s.SearchByTicker(ctx, ticker, 5)
	if err != nil {
		return "", err
	}

	if len(results) == 0 {
		return "No historical context available.", nil
	}

	context := fmt.Sprintf("Historical context from %d similar posts:\n", len(results))
	for i, r := range results {
		sentiment := r.Sentiment
		if sentiment == "unknown" {
			sentiment = "unanalyzed"
		}

		// Truncate long content
		text := r.ContentText
		if len(text) > 150 {
			text = text[:150] + "..."
		}

		context += fmt.Sprintf("%d. [%s, %s] %s (similarity: %.0f%%)\n",
			i+1, r.CreatorName, sentiment, text, r.Similarity*100)
	}

	return context, nil
}
