package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	domain "github.com/naifenmizuha/basetion/src/internal/domain/teamquery"
	"github.com/naifenmizuha/basetion/src/internal/domain/training"
	infrateamquery "github.com/naifenmizuha/basetion/src/internal/infra/teamquery"
)

func newTestTeamTools(t *testing.T) []tool.InvokableTool {
	t.Helper()
	executor, err := infrateamquery.NewLuaExecutor(infrateamquery.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	query, err := domain.NewService(executor)
	if err != nil {
		t.Fatal(err)
	}
	games, err := game.NewQueryService(&fetchGameRepository{})
	if err != nil {
		t.Fatal(err)
	}
	trainings, err := training.NewQueryService(&fetchTrainingRepository{}, training.WithPlayerReader(&fetchTrainingPlayerReader{}), training.WithTeamReader(&fetchTrainingTeamReader{}))
	if err != nil {
		t.Fatal(err)
	}
	tools, err := NewTeamTools(query, games, trainings, &teamModifyTestService{}, testPlayerModifier{}, &teamModifyTestService{}, testTrainingModifier{}, gameRecordingRunnerStub{}, WithTeamModifyIDGenerator(func() string { return "generated-id" }))
	if err != nil {
		t.Fatal(err)
	}
	return tools
}

func teamToolByName(t *testing.T, values []tool.InvokableTool, name string) tool.InvokableTool {
	t.Helper()
	for _, value := range values {
		info, err := value.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name == name {
			return value
		}
	}
	t.Fatalf("tool %q was not registered", name)
	return nil
}

func TestNewTeamToolsRegistersGameCreateTool(t *testing.T) {
	values := newTestTeamTools(t)
	if len(values) != 5 {
		t.Fatalf("tool count=%d", len(values))
	}
	for _, name := range []string{TeamDescribeToolName, TeamFetchToolName, TeamQueryToolName, TeamModifyToolName, TeamGameCreateToolName} {
		teamToolByName(t, values, name)
	}
}

func TestTeamDescribeRoutesNamespacedTopics(t *testing.T) {
	describe := teamToolByName(t, newTestTeamTools(t), TeamDescribeToolName)

	rootJSON, err := describe.InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	var root struct {
		Topics []teamDescribeCatalog `json:"topics"`
	}
	if err := json.Unmarshal([]byte(rootJSON), &root); err != nil {
		t.Fatal(err)
	}
	if len(root.Topics) != 3 || root.Topics[0].Name != "fetch" || root.Topics[1].Name != "query" || root.Topics[2].Name != "modify" {
		t.Fatalf("root=%#v", root)
	}

	output, err := describe.InvokableRun(context.Background(), `{"topics":["fetch.game.performances","query.player.list","modify.team.create","fetch.game.performances","unknown"]}`)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Topics []json.RawMessage `json:"topics"`
	}
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Topics) != 4 {
		t.Fatalf("topics=%s", output)
	}
	for index, want := range []struct {
		Name  string
		Found bool
	}{{"fetch.game.performances", true}, {"query.player.list", true}, {"modify.team.create", true}, {"unknown", false}} {
		var topic struct {
			Name  string `json:"name"`
			Found bool   `json:"found"`
		}
		if err := json.Unmarshal(decoded.Topics[index], &topic); err != nil {
			t.Fatal(err)
		}
		if topic.Name != want.Name || topic.Found != want.Found {
			t.Fatalf("topic %d = %#v, want %#v", index, topic, want)
		}
	}

	groupJSON, err := describe.InvokableRun(context.Background(), `{"topics":["query.game"]}`)
	if err != nil {
		t.Fatal(err)
	}
	var group struct {
		Topics []domain.TopicDescription `json:"topics"`
	}
	if err := json.Unmarshal([]byte(groupJSON), &group); err != nil {
		t.Fatal(err)
	}
	if len(group.Topics) != 1 || group.Topics[0].Name != "query.game" || len(group.Topics[0].Children) != 2 || group.Topics[0].Children[0].Name != "query.game.list" {
		t.Fatalf("group=%#v", group)
	}
}

func TestTeamDescribeSchemaOnlyAcceptsTopics(t *testing.T) {
	describe := teamToolByName(t, newTestTeamTools(t), TeamDescribeToolName)
	info, err := describe.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := parameters.Properties.Get("topics"); !ok {
		t.Fatalf("missing topics: %#v", parameters)
	}
	for _, field := range []string{"mode", "program", "operation", "arguments", "confirmed", "operations"} {
		if _, ok := parameters.Properties.Get(field); ok {
			t.Fatalf("unexpected %q: %#v", field, parameters)
		}
	}
}
