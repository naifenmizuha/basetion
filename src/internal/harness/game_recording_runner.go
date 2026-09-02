package harness

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	appconfig "github.com/naifenmizuha/basetion/src/internal/config"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	basetiontools "github.com/naifenmizuha/basetion/src/internal/tools"
)

type gameRecordingSourceKey struct{}

// GameRecordingTurnScope installs a user-only snapshot before the outer Agent
// starts. Unlike model middleware context, this context also reaches tools.
type GameRecordingTurnScope struct{}

func NewGameRecordingTurnScope() *GameRecordingTurnScope { return &GameRecordingTurnScope{} }

func (*GameRecordingTurnScope) Begin(ctx context.Context) (context.Context, func()) {
	return ctx, func() {}
}

func (*GameRecordingTurnScope) WithMessages(ctx context.Context, messages []*schema.AgenticMessage) context.Context {
	return context.WithValue(ctx, gameRecordingSourceKey{}, userMessageSnapshot(messages))
}

// gameRecordingSource is installed by the outer visibility middleware before
// the model chooses team_game_create. Only user text is copied into the child.
func gameRecordingSource(ctx context.Context) []*schema.AgenticMessage {
	value, _ := ctx.Value(gameRecordingSourceKey{}).([]*schema.AgenticMessage)
	return append([]*schema.AgenticMessage(nil), value...)
}

type GameRecordingRunner struct {
	model model.AgenticModel
	games basetiontools.GameModifier
}

func NewGameRecordingRunner(agenticModel model.AgenticModel, games basetiontools.GameModifier) (*GameRecordingRunner, error) {
	if agenticModel == nil || games == nil {
		return nil, errors.New("agentic model and game modifier are required")
	}
	return &GameRecordingRunner{model: agenticModel, games: games}, nil
}

func (r *GameRecordingRunner) Run(ctx context.Context) (game.GameCreateProgress, error) {
	source := gameRecordingSource(ctx)
	if len(source) == 0 {
		return game.GameCreateProgress{}, errors.New("game recording has no user context")
	}
	intake := basetiontools.NewGameRecordingIntake(randomPetname())
	tools, err := basetiontools.NewGameRecordingTools(r.games, intake)
	if err != nil {
		return game.GameCreateProgress{}, err
	}
	baseTools := make([]tool.BaseTool, len(tools))
	for index := range tools {
		baseTools[index] = tools[index]
	}
	cfg := appconfig.Get()
	agent, err := adk.NewTypedChatModelAgent(ctx, &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
		Name: "game_recording_session", Description: "内部比赛录入会话", Instruction: gameRecordingInstruction,
		Model: newStatusModel(r.model, cfg.User.Name, cfg.OpenAI.ContextWindowTokens, time.Now), MaxIterations: cfg.Agent.MaxIterations,
		Handlers:    []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]{newGameRecordingVisibility()},
		ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: baseTools}},
	})
	if err != nil {
		return game.GameCreateProgress{}, fmt.Errorf("create game recording agent: %w", err)
	}
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: agent, EnableStreaming: false})
	iter := runner.Run(ctx, source)
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event != nil && event.Err != nil {
			return game.GameCreateProgress{}, event.Err
		}
	}
	if progress, ok := intake.Progress(); ok {
		return progress, nil
	}
	return game.GameCreateProgress{}, errors.New("game recording ended without team_game_finalize")
}

func randomPetname() string {
	adjectives := []string{"amber", "brisk", "calm", "daring", "eager", "gentle", "lucky", "mellow"}
	animals := []string{"otter", "fox", "panda", "sparrow", "badger", "heron", "tiger", "whale"}
	pick := func(values []string) string {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(values))))
		if err != nil {
			return values[0]
		}
		return values[n.Int64()]
	}
	return fmt.Sprintf("%s-%s", pick(adjectives), pick(animals))
}

const gameRecordingInstruction = `你正在执行一次内部比赛录入。用户资料只来自输入中的用户消息。
严格按工具阶段工作：先 team_game_begin，再以连续 Play 调用一次或多次 team_game_append_plays，最后 team_game_finalize。通常每半局一个块；过长时按连续打席拆分。
球队、球数、垒包、出局和比分由服务端推导。逐球结果使用 ball、called_strike、swinging_strike、foul、in_play 等 Schema 枚举；外野守备位置使用 outfielder。打者首次上垒的 runner_outcome 使用 from_base=0；得分使用 result=score、from_base 和 earned，必要时填写 rbi_batter_jersey_number。
append 的每个 Play 必须使用这个外层形状，字段名不得替换或缩写：
{"intake_id":"amber-otter","plays":[{"inning":1,"half":"top","batting_order":1,"batter_jersey_number":18,"pitcher_jersey_number":2,"batting_result":"single","result_description":"中外野一垒安打","pitches":[{"result":"called_strike"},{"result":"in_play"}],"runner_outcomes":[{"jersey_number":18,"result":"advance","from_base":0,"to_base":1}]}]}
不得使用 outcome、pitch_sequence、type、runner_jersey_number 或 safe。每次 append 仅提交一个半局或更小的连续打席块。
复杂事件按以下模板拆分，示例中的背号仅为示意：
双杀的 batting_result 必须是 ground_out；double_play 不是 batting_result。垒上跑者封杀使用 force_out；out 不是合法 runner result：
{"batting_result":"ground_out","runner_outcomes":[{"jersey_number":4,"result":"force_out","from_base":1}],"fielding_outcomes":[{"jersey_number":6,"position":"shortstop","result":"assist"},{"jersey_number":25,"position":"second_base","result":"assist"},{"jersey_number":18,"position":"first_base","result":"putout"}]}
牺牲触击：
{"batting_result":"sacrifice_bunt","runner_outcomes":[{"jersey_number":6,"result":"advance","from_base":1,"to_base":2}],"fielding_outcomes":[{"jersey_number":2,"position":"pitcher","result":"assist"},{"jersey_number":1,"position":"first_base","result":"putout"}]}
牺牲飞球送回跑者：
{"batting_result":"sacrifice_fly","runner_outcomes":[{"jersey_number":9,"result":"score","from_base":3,"to_base":4,"earned":true,"rbi_batter_jersey_number":7}],"fielding_outcomes":[{"jersey_number":31,"position":"outfielder","result":"putout"}]}
阳春本垒打：
{"batting_result":"home_run","runner_outcomes":[{"jersey_number":4,"result":"score","from_base":0,"to_base":4,"earned":true,"rbi_batter_jersey_number":4}]}
未知可选字段必须省略，绝不填空字符串、0 或 false。不要输出 JSON 文本、不要调用不存在的工具、不要自动重试失败调用。完整提交后只简短报告完成。`
