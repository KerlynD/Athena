package social

import (
	"testing"
	"time"
)

func TestCreatorContentStruct(t *testing.T) {
	content := CreatorContent{
		ID:               1,
		CreatorName:      "testcreator",
		Platform:         "twitter",
		ContentID:        "123456789",
		ContentText:      "Bullish on $SPY today!",
		MentionedTickers: []string{"SPY"},
		PostedAt:         time.Now(),
		CreatedAt:        time.Now(),
	}

	if content.CreatorName != "testcreator" {
		t.Errorf("CreatorName = %v, want testcreator", content.CreatorName)
	}

	if len(content.MentionedTickers) != 1 {
		t.Errorf("MentionedTickers count = %v, want 1", len(content.MentionedTickers))
	}
}

func TestNewStore(t *testing.T) {
	// NewStore should work with nil db (for testing struct creation)
	store := NewStore(nil)
	if store == nil {
		t.Error("NewStore returned nil")
	}
}

func TestTweetStruct(t *testing.T) {
	tweet := Tweet{
		ID:        "123456789",
		Text:      "Watching $QQQ closely",
		CreatedAt: time.Now(),
		AuthorID:  "987654321",
	}

	if tweet.ID != "123456789" {
		t.Errorf("ID = %v, want 123456789", tweet.ID)
	}

	// Test ticker extraction from this tweet
	tickers := ExtractTickers(tweet.Text)
	if len(tickers) != 1 || tickers[0] != "QQQ" {
		t.Errorf("ExtractTickers = %v, want [QQQ]", tickers)
	}
}
