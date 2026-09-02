package tools

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	domain "github.com/naifenmizuha/basetion/src/internal/domain/teamquery"
	"github.com/naifenmizuha/basetion/src/internal/domain/training"
)

const TeamDescribeToolName = "team_describe"

type teamDescribeInput struct {
	Topics []string `json:"topics,omitempty" jsonschema:"description=要读取的目录或精确 topic；使用 fetch.*、query.*、modify.* 前缀，省略时返回三个目录"`
}

type teamDescribeOutput struct {
	Topics []any `json:"topics"`
}

type teamDescribeCatalog struct {
	Name     string                     `json:"name"`
	Kind     string                     `json:"kind"`
	Found    bool                       `json:"found"`
	Summary  string                     `json:"summary"`
	Children []teamDescribeCatalogChild `json:"children"`
}

type teamDescribeCatalogChild struct {
	Name      string `json:"name"`
	Summary   string `json:"summary"`
	Available *bool  `json:"available,omitempty"`
}

type teamDescribeUnknownTopic struct {
	Name  string `json:"name"`
	Found bool   `json:"found"`
	Error string `json:"error"`
}

type teamDescribeHandler struct {
	query  *domain.Service
	modify *teamModifyHandler
}

// NewTeamTools builds the complete model-visible team protocol. The describe
// and modify tools share one operation catalog so their contracts cannot
// drift apart.
func NewTeamTools(query *domain.Service, games *game.QueryService, trainings *training.QueryService, teams TeamModifier, players PlayerModifier, gameModifier GameModifier, training TrainingModifier, gameRecorder GameRecordingRunner, options ...TeamModifyOption) ([]tool.InvokableTool, error) {
	if query == nil {
		return nil, errors.New("team query service is required")
	}
	if games == nil {
		return nil, errors.New("game query service is required")
	}
	if trainings == nil {
		return nil, errors.New("training query service is required")
	}
	modifyHandler, err := newTeamModifyHandler(teams, players, gameModifier, training, options...)
	if err != nil {
		return nil, err
	}
	describeTool, err := newTeamDescribeTool(&teamDescribeHandler{query: query, modify: modifyHandler})
	if err != nil {
		return nil, err
	}
	fetchTool, err := NewTeamFetch(games, trainings)
	if err != nil {
		return nil, err
	}
	queryTool, err := NewTeamQuery(query)
	if err != nil {
		return nil, err
	}
	modifyTool, err := newTeamModifyTool(modifyHandler)
	if err != nil {
		return nil, err
	}
	gameCreateTool, err := newTeamGameCreateTool(gameRecorder)
	if err != nil {
		return nil, err
	}
	return []tool.InvokableTool{describeTool, fetchTool, queryTool, modifyTool, gameCreateTool}, nil
}

func newTeamDescribeTool(handler *teamDescribeHandler) (tool.InvokableTool, error) {
	return toolutils.InferTool(
		TeamDescribeToolName,
		"按需读取球队读取、Lua 查询和预定义修改的目录及精确协议。topic 使用 fetch.*、query.*、modify.* 前缀；先读取所需 topic，再调用对应工具。此工具不读取或修改业务数据。",
		handler.invoke,
	)
}

func (h *teamDescribeHandler) invoke(ctx context.Context, input teamDescribeInput) (teamDescribeOutput, error) {
	if err := ctx.Err(); err != nil {
		return teamDescribeOutput{}, err
	}
	requested := input.Topics
	if len(requested) == 0 {
		requested = []string{"fetch", "query", "modify"}
	}
	result := make([]any, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, raw := range requested {
		name := strings.TrimSpace(raw)
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		topic, err := h.describeTopic(ctx, name)
		if err != nil {
			return teamDescribeOutput{}, err
		}
		result = append(result, topic)
	}
	return teamDescribeOutput{Topics: result}, nil
}

func (h *teamDescribeHandler) describeTopic(ctx context.Context, name string) (any, error) {
	if name == "fetch" || name == "query" || name == "modify" {
		return h.describeCatalog(ctx, name)
	}
	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 || parts[1] == "" {
		return teamDescribeUnknownTopic{Name: name, Found: false, Error: "team describe topic must use fetch.*, query.*, or modify.*"}, nil
	}
	switch parts[0] {
	case "fetch":
		return qualifyFetchTopic(parts[0], describeFetchTopics([]string{parts[1]})[0]), nil
	case "query":
		description, err := h.query.Describe(ctx, []string{parts[1]})
		if err != nil {
			return nil, err
		}
		return qualifyQueryTopic(parts[0], description.Topics[0]), nil
	case "modify":
		return qualifyModifyTopic(parts[0], h.modify.describe([]string{parts[1]})[0]), nil
	default:
		return teamDescribeUnknownTopic{Name: name, Found: false, Error: "unknown team describe catalog"}, nil
	}
}

func (h *teamDescribeHandler) describeCatalog(ctx context.Context, catalog string) (teamDescribeCatalog, error) {
	result := teamDescribeCatalog{Name: catalog, Kind: "catalog", Found: true}
	switch catalog {
	case "fetch":
		result.Summary = "服务端组合比赛读取。"
		for _, topic := range describeFetchTopics(nil) {
			result.Children = append(result.Children, teamDescribeCatalogChild{Name: qualifiedTopicName(catalog, topic.Name), Summary: topic.Summary})
		}
	case "query":
		result.Summary = "受限 Lua 基础数据查询。"
		description, err := h.query.Describe(ctx, nil)
		if err != nil {
			return teamDescribeCatalog{}, err
		}
		for _, topic := range description.Topics {
			available := topic.Available
			result.Children = append(result.Children, teamDescribeCatalogChild{Name: qualifiedTopicName(catalog, topic.Name), Summary: topic.Summary, Available: &available})
		}
	case "modify":
		result.Summary = "预定义球队数据修改。"
		for _, topic := range h.modify.describe(nil) {
			result.Children = append(result.Children, teamDescribeCatalogChild{Name: qualifiedTopicName(catalog, topic.Name), Summary: topic.Summary})
		}
	}
	return result, nil
}

func qualifiedTopicName(catalog, name string) string {
	if name == "" {
		return ""
	}
	return catalog + "." + name
}

func qualifyFetchTopic(catalog string, topic teamFetchTopic) teamFetchTopic {
	topic.Name = qualifiedTopicName(catalog, topic.Name)
	for index := range topic.Children {
		topic.Children[index] = qualifiedTopicName(catalog, topic.Children[index])
	}
	return topic
}

func qualifyQueryTopic(catalog string, topic domain.TopicDescription) domain.TopicDescription {
	topic.Name = qualifiedTopicName(catalog, topic.Name)
	for index := range topic.Children {
		topic.Children[index].Name = qualifiedTopicName(catalog, topic.Children[index].Name)
	}
	return topic
}

func qualifyModifyTopic(catalog string, topic modifyTopicDescription) modifyTopicDescription {
	topic.Name = qualifiedTopicName(catalog, topic.Name)
	for index := range topic.Children {
		topic.Children[index].Name = qualifiedTopicName(catalog, topic.Children[index].Name)
	}
	return topic
}
