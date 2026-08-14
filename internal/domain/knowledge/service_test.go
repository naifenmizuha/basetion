package knowledge

import (
	"context"
	"errors"
	"testing"
)

type stubRetriever struct {
	query string
	limit int
	err   error
}

func (r *stubRetriever) Search(_ context.Context, query string, limit int) ([]Document, error) {
	r.query, r.limit = query, limit
	return []Document{{ID: "one"}}, r.err
}

func TestServiceSearch(t *testing.T) {
	t.Parallel()

	retriever := &stubRetriever{}
	service, err := NewService(retriever)
	if err != nil {
		t.Fatal(err)
	}
	documents, err := service.Search(context.Background(), "  Eino  ", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 1 || retriever.query != "Eino" || retriever.limit != defaultLimit {
		t.Fatalf("unexpected search: documents=%#v query=%q limit=%d", documents, retriever.query, retriever.limit)
	}
}

func TestServiceSearchRejectsInvalidInputAndWrapsErrors(t *testing.T) {
	t.Parallel()

	retriever := &stubRetriever{err: errors.New("offline")}
	service, _ := NewService(retriever)
	if _, err := service.Search(context.Background(), " ", 1); err == nil {
		t.Fatal("Search() accepted an empty query")
	}
	if _, err := service.Search(context.Background(), "query", maxLimit+1); err == nil {
		t.Fatal("Search() accepted an excessive limit")
	}
	if _, err := service.Search(context.Background(), "query", 1); !errors.Is(err, retriever.err) {
		t.Fatalf("Search() error = %v, want wrapped retriever error", err)
	}
}
