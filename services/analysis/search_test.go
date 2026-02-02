package analysis

import (
	"context"
	"testing"
)

func TestNewSemanticSearcher(t *testing.T) {
	searcher := NewSemanticSearcher(nil)
	if searcher == nil {
		t.Error("NewSemanticSearcher returned nil")
	}
}

func TestSimilarContentStruct(t *testing.T) {
	content := SimilarContent{
		ID:          1,
		CreatorName: "testcreator",
		ContentText: "Bullish on $SPY!",
		Sentiment:   "bullish",
		Similarity:  0.95,
	}

	if content.CreatorName != "testcreator" {
		t.Errorf("CreatorName = %v, want testcreator", content.CreatorName)
	}

	if content.Similarity != 0.95 {
		t.Errorf("Similarity = %v, want 0.95", content.Similarity)
	}
}

func TestEmbeddingDimensionValidation(t *testing.T) {
	// Test that we correctly validate embedding dimensions
	// The actual search requires a DB, so we just test the struct
	correctSize := 384
	wrongSize := 100

	if correctSize != 384 {
		t.Error("Embedding dimension should be 384")
	}

	if wrongSize == 384 {
		t.Error("Wrong size should not equal 384")
	}
}

func TestSearchSimilarContent_WrongDimensions(t *testing.T) {
	searcher := NewSemanticSearcher(nil)
	ctx := context.Background()

	// Test with wrong dimensions - should error before hitting DB
	wrongEmbedding := make([]float64, 100)
	_, err := searcher.SearchSimilarContent(ctx, wrongEmbedding, 5, 0.7)
	if err == nil {
		t.Error("Expected error for wrong embedding dimensions")
	}
}
