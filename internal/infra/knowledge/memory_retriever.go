package knowledge

import (
	"context"
	"sort"
	"strings"
	"unicode"

	domain "github.com/naifenmizuha/basetion/internal/domain/knowledge"
)

// MemoryRetriever is a deterministic development adapter that can later be
// replaced by a vector-store implementation of the domain Retriever contract.
type MemoryRetriever struct {
	documents []domain.Document
}

// NewMemoryRetriever creates a retriever from an immutable document snapshot.
func NewMemoryRetriever(documents []domain.Document) *MemoryRetriever {
	copyOfDocuments := append([]domain.Document(nil), documents...)
	return &MemoryRetriever{documents: copyOfDocuments}
}

// DefaultDocuments supplies a small deterministic corpus for the first CLI
// slice; it is not intended as production knowledge storage.
func DefaultDocuments() []domain.Document {
	return []domain.Document{
		{ID: "architecture", Title: "Basetion 架构", Content: "Basetion 采用入口、会话应用、Agent Harness、工具、领域和基础设施六个职责区域。"},
		{ID: "session", Title: "会话与检查点", Content: "业务 Session 保存已完成对话；Eino Checkpoint 保存被中断的运行时状态，两者相互独立。"},
		{ID: "agentic", Title: "AgenticModel", Content: "AgenticModel 使用 AgenticMessage 和有序 ContentBlock 表达文本、reasoning 与工具调用。"},
	}
}

// Search ranks documents using deterministic token occurrence scoring.
func (r *MemoryRetriever) Search(ctx context.Context, query string, limit int) ([]domain.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tokens := tokenize(query)
	results := make([]domain.Document, 0, len(r.documents))
	for _, document := range r.documents {
		haystack := strings.ToLower(document.Title + " " + document.Content)
		var score float64
		for _, token := range tokens {
			score += float64(strings.Count(haystack, token))
		}
		if score > 0 {
			document.Score = score
			results = append(results, document)
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].ID < results[j].ID
		}
		return results[i].Score > results[j].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func tokenize(value string) []string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return nil
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	})
	if len(fields) == 0 {
		return []string{value}
	}
	return fields
}
