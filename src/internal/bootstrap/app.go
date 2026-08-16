package bootstrap

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/naifenmizuha/basetion/src/internal/application/conversation"
	"github.com/naifenmizuha/basetion/src/internal/config"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/roster"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
	domainteamquery "github.com/naifenmizuha/basetion/src/internal/domain/teamquery"
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
	logger := log.New(stderr, "basetion ", log.LstdFlags)
	diagnosticFields := cfg.DiagnosticFields()
	diagnosticFields["profile"] = profile
	logger.Printf("启动配置: %v", diagnosticFields)
	if err := adk.SetLanguage(adk.LanguageChinese); err != nil {
		fmt.Fprintf(stderr, "初始化 Eino 语言失败: %v\n", err)
		return 1
	}

	store, err := infrasession.NewFileStore(cfg.Session.Dir)
	if err != nil {
		fmt.Fprintf(stderr, "初始化 Session 存储失败: %v\n", err)
		return 1
	}
	databaseProfile := cfg.Database.Run
	if profile == cli.ProfileDev {
		databaseProfile = cfg.Database.Dev
	}
	var database *infrapostgres.ManagedStore
	switch databaseProfile.Mode {
	case config.DatabaseModeFixed:
		database, err = infrapostgres.OpenFixed(ctx, databaseProfile.URL)
	case config.DatabaseModeTemporary:
		database, err = infrapostgres.OpenTemporary(ctx, databaseProfile.AdminURL, databaseProfile.TemporaryPrefix, infrapostgres.WithDevelopmentFixtures())
	default:
		err = fmt.Errorf("unsupported database mode %q", databaseProfile.Mode)
	}
	if err != nil {
		fmt.Fprintf(stderr, "初始化 PostgreSQL 失败: %v\n", err)
		return 1
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := database.Close(cleanupCtx); err != nil {
			fmt.Fprintf(stderr, "清理 PostgreSQL 失败: %v\n", err)
			if exitCode == 0 {
				exitCode = 1
			}
		}
	}()
	teamQueryExecutor, err := infrateamquery.NewLuaExecutor(infrateamquery.DefaultLimits(), infrateamquery.WithRosterReader(database.Store.RosterReader()))
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
	clock := wallClock{}
	teamService, err := team.NewService(database.Store.Teams(), clock)
	if err != nil {
		fmt.Fprintf(stderr, "初始化球队写入服务失败: %v\n", err)
		return 1
	}
	playerService, err := player.NewService(database.Store.Players(), clock)
	if err != nil {
		fmt.Fprintf(stderr, "初始化球员写入服务失败: %v\n", err)
		return 1
	}
	rosterService, err := roster.NewService(database.Store, clock)
	if err != nil {
		fmt.Fprintf(stderr, "初始化名单写入服务失败: %v\n", err)
		return 1
	}
	teamModifyTool, err := basetiontools.NewTeamModify(teamService, playerService, rosterService)
	if err != nil {
		fmt.Fprintf(stderr, "初始化球队修改工具失败: %v\n", err)
		return 1
	}
	agenticModel, err := harness.NewAgenticModel(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "初始化 AgenticModel 失败: %v\n", err)
		return 1
	}
	runtime, err := harness.NewRuntime(ctx, agenticModel, []tool.BaseTool{teamQueryTool, teamModifyTool}, logger)
	if err != nil {
		fmt.Fprintf(stderr, "初始化 Agent Harness 失败: %v\n", err)
		return 1
	}
	conversationService, err := conversation.NewService(runtime.Runner, store, conversation.NewSessionLocker(), runtime.Callback)
	if err != nil {
		fmt.Fprintf(stderr, "初始化会话服务失败: %v\n", err)
		return 1
	}
	return cli.Execute(ctx, args, conversationService, stdout, stderr)
}
