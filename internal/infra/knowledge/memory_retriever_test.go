package knowledge

import (
	"context"
	"testing"

	domain "github.com/naifenmizuha/basetion/internal/domain/knowledge"
)

func TestMemoryRetrieverRanksDeterministically(t *testing.T) {
	t.Parallel()

	retriever := NewMemoryRetriever([]domain.Document{
		{ID: "b", Title: "Go", Content: "Go language"},
		{ID: "a", Title: "Go Go", Content: "language"},
		{ID: "c", Title: "Rust", Content: "language"},
	})
	got, err := retriever.Search(context.Background(), "go", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("unexpected ranking: %#v", got)
	}
}
