package tools

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	domain "github.com/naifenmizuha/basetion/internal/domain/knowledge"
)

const KnowledgeSearchToolName = "search_knowledge"

type knowledgeSearchInput struct {
	Query string `json:"query" jsonschema:"required,description=要检索的知识问题或关键词"`
	Limit int    `json:"limit,omitempty" jsonschema:"description=最多返回的结果数，范围 1 到 20"`
}

type knowledgeSearchOutput struct {
	Documents []domain.Document `json:"documents"`
}

// NewKnowledgeSearch creates the Eino adapter from model arguments to the
// knowledge domain service.
func NewKnowledgeSearch(service *domain.Service) (tool.InvokableTool, error) {
	if service == nil {
		return nil, errors.New("knowledge service is required")
	}
	return toolutils.InferTool(
		KnowledgeSearchToolName,
		"在 Basetion 的已有知识中检索与问题相关的内容。",
		func(ctx context.Context, input knowledgeSearchInput) (knowledgeSearchOutput, error) {
			documents, err := service.Search(ctx, input.Query, input.Limit)
			if err != nil {
				return knowledgeSearchOutput{}, err
			}
			return knowledgeSearchOutput{Documents: documents}, nil
		},
	)
}
