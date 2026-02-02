// Package main provides manual content input functionality for the orchestrator.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/lib/pq"

	"athena/services/social"
)

// addContent handles the interactive content input flow
func addContent(ctx context.Context, db *sql.DB) error {
	log.Println("=== Add Creator Content ===")
	log.Println("Manually add content from creators for sentiment analysis.")
	log.Println("This content will be analyzed in the next 'analyze' run.")
	log.Println("")

	reader := bufio.NewReader(os.Stdin)

	// Get creator name
	fmt.Print("Creator name (e.g., MobyInvest): ")
	creatorName, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read creator name: %w", err)
	}
	creatorName = strings.TrimSpace(creatorName)
	if creatorName == "" {
		return fmt.Errorf("creator name cannot be empty")
	}

	// Get content
	fmt.Println("\nPaste the content (tweet/post). Press Enter twice when done:")
	var contentLines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" && len(contentLines) > 0 {
			break
		}
		if line != "" {
			contentLines = append(contentLines, line)
		}
	}

	if len(contentLines) == 0 {
		return fmt.Errorf("content cannot be empty")
	}

	content := strings.Join(contentLines, " ")

	// Extract tickers
	tickers := social.ExtractTickers(content)
	log.Printf("Detected tickers: %v", tickers)

	// Generate a unique content ID
	contentID := fmt.Sprintf("manual_%s_%d", creatorName, time.Now().UnixNano())

	// Store in database
	err = storeManualContent(ctx, db, creatorName, contentID, content, tickers)
	if err != nil {
		return fmt.Errorf("store content: %w", err)
	}

	log.Println("\n✓ Content saved successfully!")
	log.Println("Run 'orchestrator analyze' to process this content.")

	return nil
}

// storeManualContent saves manually entered content to the database
func storeManualContent(ctx context.Context, db *sql.DB, creatorName, contentID, content string, tickers []string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	query := `
		INSERT INTO creator_content 
		(creator_name, platform, content_id, content_text, mentioned_tickers, posted_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (content_id) DO NOTHING
	`

	_, err := db.ExecContext(ctx, query,
		creatorName,
		"manual",
		contentID,
		content,
		pq.Array(tickers),
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("insert content: %w", err)
	}

	return nil
}

// addContentBatch allows adding multiple pieces of content at once
func addContentBatch(ctx context.Context, db *sql.DB) error {
	log.Println("=== Batch Add Creator Content ===")
	log.Println("Add multiple pieces of content. Type 'done' when finished.")
	log.Println("")

	reader := bufio.NewReader(os.Stdin)
	count := 0

	for {
		fmt.Printf("\n--- Content #%d ---\n", count+1)
		fmt.Print("Creator name (or 'done' to finish): ")
		creatorName, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read creator name: %w", err)
		}
		creatorName = strings.TrimSpace(creatorName)

		if strings.ToLower(creatorName) == "done" {
			break
		}

		if creatorName == "" {
			log.Println("Skipping empty creator name...")
			continue
		}

		fmt.Println("Paste content (press Enter twice when done):")
		var contentLines []string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" && len(contentLines) > 0 {
				break
			}
			if line != "" {
				contentLines = append(contentLines, line)
			}
		}

		if len(contentLines) == 0 {
			log.Println("Skipping empty content...")
			continue
		}

		content := strings.Join(contentLines, " ")
		tickers := social.ExtractTickers(content)
		contentID := fmt.Sprintf("manual_%s_%d", creatorName, time.Now().UnixNano())

		err = storeManualContent(ctx, db, creatorName, contentID, content, tickers)
		if err != nil {
			log.Printf("Error saving content: %v", err)
			continue
		}

		count++
		log.Printf("✓ Saved content from %s (tickers: %v)", creatorName, tickers)
	}

	log.Printf("\n=== Saved %d content items ===", count)
	return nil
}

// listContent shows recent content in the database
func listContent(ctx context.Context, db *sql.DB, limit int) error {
	log.Println("=== Recent Creator Content ===")

	query := `
		SELECT creator_name, platform, content_text, mentioned_tickers, sentiment, posted_at
		FROM creator_content
		ORDER BY created_at DESC
		LIMIT $1
	`

	rows, err := db.QueryContext(ctx, query, limit)
	if err != nil {
		return fmt.Errorf("query content: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var creatorName, platform, content string
		var tickers pq.StringArray
		var sentiment sql.NullString
		var postedAt time.Time

		if err := rows.Scan(&creatorName, &platform, &content, &tickers, &sentiment, &postedAt); err != nil {
			return fmt.Errorf("scan row: %w", err)
		}

		count++
		fmt.Printf("\n[%d] @%s (%s) - %s\n", count, creatorName, platform, postedAt.Format("2006-01-02 15:04"))

		// Truncate long content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		fmt.Printf("    %s\n", content)

		if len(tickers) > 0 {
			fmt.Printf("    Tickers: %v\n", []string(tickers))
		}

		if sentiment.Valid {
			fmt.Printf("    Sentiment: %s\n", sentiment.String)
		} else {
			fmt.Printf("    Sentiment: (not analyzed)\n")
		}
	}

	if count == 0 {
		log.Println("No content found. Use 'add-content' to add some.")
	}

	return nil
}
