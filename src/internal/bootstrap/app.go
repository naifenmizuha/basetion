package bootstrap

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/google/uuid"
	"github.com/naifenmizuha/basetion/src/internal/application/conversation"
	"github.com/naifenmizuha/basetion/src/internal/config"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
	domainteamquery "github.com/naifenmizuha/basetion/src/internal/domain/teamquery"
	"github.com/naifenmizuha/basetion/src/internal/domain/training"
	"github.com/naifenmizuha/basetion/src/internal/entry/cli"
	"github.com/naifenmizuha/basetion/src/internal/harness"
	infrapostgres "github.com/naifenmizuha/basetion/src/internal/infra/postgres"
	infrasession "github.com/naifenmizuha/basetion/src/internal/infra/session"
	infrateamquery "github.com/naifenmizuha/basetion/src/internal/infra/teamquery"
	basetiontools "github.com/naifenmizuha/basetion/src/internal/tools"
)

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

// Execute assembles the first vertical slice and invokes the CLI adapter.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer) (exitCode int) {
	profile, args, err := cli.ExtractProfile(args)
	if err != nil {
		fmt.Fprintf(stderr, "参数错误: %v\n", err)
		return 2
	}
	if err := config.Init(); err != nil {
		fmt.Fprintf(stderr, "配置错误: %v\n", err)
		return 2
	}
	cfg := config.Get()
	logFile, err := openLogFile(cfg.Log.File)
	if err != nil {
		fmt.Fprintf(stderr, "初始化运行日志失败: %v\n", err)
		return 2
	}
	defer logFile.Close()
	logger := log.New(logFile, "basetion ", log.LstdFlags)
	diagnosticFields := cfg.DiagnosticFields()
	diagnosticFields["profile"] = profile
	logger.Printf("启动配置: %v", diagnosticFields)
	if err := adk.SetLanguage(adk.LanguageChinese); err != nil {
		fmt.Fprintf(stderr, "初始化 Eino 语言失败: %v\n", err)
		return 1
	}

	var store conversation.SessionStore
	if cli.IsTestInvocation(args) {
		store = infrasession.NewMemoryStore()
	} else {
		store, err = infrasession.NewFileStore(cfg.Session.Dir)
		if err != nil {
			fmt.Fprintf(stderr, "初始化 Session 存储失败: %v\n", err)
			return 1
		}
	}
	databaseProfile := cfg.Database.Run
	if profile == cli.ProfileDev {
		databaseProfile = cfg.Database.Dev
	}
	database, err := infrapostgres.Open(ctx, databaseProfile.URL)
	if err != nil {
		fmt.Fprintf(stderr, "初始化 PostgreSQL 失败: %v\n", err)
		return 1
	}
	defer database.Close()
	teamReadService, err := team.NewQueryService(database.Teams())
	if err != nil {
		fmt.Fprintf(stderr, "初始化球队读取服务失败: %v\n", err)
		return 1
	}
	playerReadService, err := player.NewQueryService(database.PlayerReader())
	if err != nil {
		fmt.Fprintf(stderr, "初始化球员读取服务失败: %v\n", err)
		return 1
	}
	gameReadService, err := game.NewQueryService(database)
	if err != nil {
		fmt.Fprintf(stderr, "初始化比赛读取服务失败: %v\n", err)
		return 1
	}
	trainingReadService, err := training.NewQueryService(database.Training())
	if err != nil {
		fmt.Fprintf(stderr, "初始化自训记录读取服务失败: %v\n", err)
		return 1
	}
	teamQueryExecutor, err := infrateamquery.NewLuaExecutor(infrateamquery.DefaultLimits(), infrateamquery.WithTeamPlayerServices(teamReadService, playerReadService), infrateamquery.WithGameService(gameReadService), infrateamquery.WithTrainingService(trainingReadService))
	if err != nil {
		fmt.Fprintf(stderr, "初始化球队查询 Lua 执行器失败: %v\n", err)
		return 1
	}
	teamQueryService, err := domainteamquery.NewService(teamQueryExecutor)
	if err != nil {
		fmt.Fprintf(stderr, "初始化球队查询领域服务失败: %v\n", err)
		return 1
	}
	teamQueryTool, err := basetiontools.NewTeamQuery(teamQueryService)
	if err != nil {
		fmt.Fprintf(stderr, "初始化球队查询工具失败: %v\n", err)
		return 1
	}
	teamFetchTool, err := basetiontools.NewTeamFetch(gameReadService)
	if err != nil {
		fmt.Fprintf(stderr, "初始化球队读取工具失败: %v\n", err)
		return 1
	}
	clock := wallClock{}
	teamService, err := team.NewService(database.Teams(), clock)
	if err != nil {
		fmt.Fprintf(stderr, "初始化球队写入服务失败: %v\n", err)
		return 1
	}
	playerService, err := player.NewService(database.Players(), database, clock)
	if err != nil {
		fmt.Fprintf(stderr, "初始化球员写入服务失败: %v\n", err)
		return 1
	}
	gameService, err := game.NewService(database, clock, game.WithDirectRepositories(database.DirectGameRepositories()), game.WithIDGenerator(uuid.NewString))
	if err != nil {
		fmt.Fprintf(stderr, "初始化比赛写入服务失败: %v\n", err)
		return 1
	}
	trainingService, err := training.NewService(database.Training(), database.Players(), clock)
	if err != nil {
		fmt.Fprintf(stderr, "初始化自训记录写入服务失败: %v\n", err)
		return 1
	}
	teamModifyTool, err := basetiontools.NewTeamModify(teamService, playerService, gameService, trainingService)
	if err != nil {
		fmt.Fprintf(stderr, "初始化球队修改工具失败: %v\n", err)
		return 1
	}
	agenticModel, err := harness.NewAgenticModel(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "初始化 AgenticModel 失败: %v\n", err)
		return 1
	}
	runtime, err := harness.NewRuntime(ctx, agenticModel, []tool.BaseTool{teamFetchTool, teamQueryTool, teamModifyTool}, logger)
	if err != nil {
		fmt.Fprintf(stderr, "初始化 Agent Harness 失败: %v\n", err)
		return 1
	}
	conversationService, err := conversation.NewService(runtime.Runner, store, conversation.NewSessionLocker(), runtime.Callback, runtime.Telemetry)
	if err != nil {
		fmt.Fprintf(stderr, "初始化会话服务失败: %v\n", err)
		return 1
	}
	return cli.Execute(ctx, args, conversationService, stdout, stderr)
}

func openLogFile(path string) (*os.File, error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, fmt.Errorf("create log directory %q: %w", directory, err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", path, err)
	}
	return file, nil
}
