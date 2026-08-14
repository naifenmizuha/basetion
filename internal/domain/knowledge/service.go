package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	defaultLimit = 5
	maxLimit     = 20
)

// Document is a storage-independent knowledge search result.
type Document struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// Retriever finds domain documents without exposing a concrete vector or
// database implementation.
type Retriever interface {
	Search(ctx context.Context, query string, limit int) ([]Document, error)
}

// Service applies knowledge-search domain validation and delegates retrieval.
type Service struct {
	retriever Retriever
}

// NewService creates a knowledge search service.
func NewService(retriever Retriever) (*Service, error) {
	if retriever == nil {
		return nil, errors.New("knowledge retriever is required")
	}
	return &Service{retriever: retriever}, nil
}

// Search validates and normalizes a knowledge query.
func (s *Service) Search(ctx context.Context, query string, limit int) ([]Document, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query must not be empty")
	}
	if limit == 0 {
		limit = defaultLimit
	}
	if limit < 0 || limit > maxLimit {
		return nil, fmt.Errorf("limit must be between 1 and %d", maxLimit)
	}
	documents, err := s.retriever.Search(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("retrieve knowledge: %w", err)
	}
	return documents, nil
}
