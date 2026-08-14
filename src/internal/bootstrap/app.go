package bootstrap

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/naifenmizuha/basetion/src/internal/application/conversation"
	"github.com/naifenmizuha/basetion/src/internal/config"
	domainteamops "github.com/naifenmizuha/basetion/src/internal/domain/teamops"
	"github.com/naifenmizuha/basetion/src/internal/entry/cli"
	"github.com/naifenmizuha/basetion/src/internal/harness"
	infrasession "github.com/naifenmizuha/basetion/src/internal/infra/session"
	infrateamops "github.com/naifenmizuha/basetion/src/internal/infra/teamops"
	basetiontools "github.com/naifenmizuha/basetion/src/internal/tools"
)

// Execute assembles the first vertical slice and invokes the CLI adapter.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := config.Init(); err != nil {
		fmt.Fprintf(stderr, "配置错误: %v\n", err)
		return 2
	}
	cfg := config.Get()
	logger := log.New(stderr, "basetion ", log.LstdFlags)
	logger.Printf("启动配置: %v", cfg.DiagnosticFields())
	if err := adk.SetLanguage(adk.LanguageChinese); err != nil {
		fmt.Fprintf(stderr, "初始化 Eino 语言失败: %v\n", err)
		return 1
	}

	store, err := infrasession.NewFileStore(cfg.Session.Dir)
	if err != nil {
		fmt.Fprintf(stderr, "初始化 Session 存储失败: %v\n", err)
		return 1
	}
	teamOpsExecutor, err := infrateamops.NewLuaExecutor(infrateamops.DefaultLimits())
	if err != nil {
		fmt.Fprintf(stderr, "初始化 TeamOps Lua 执行器失败: %v\n", err)
		return 1
	}
	teamOpsService, err := domainteamops.NewService(teamOpsExecutor)
	if err != nil {
		fmt.Fprintf(stderr, "初始化 TeamOps 领域服务失败: %v\n", err)
		return 1
	}
	teamOpsTool, err := basetiontools.NewTeamOps(teamOpsService)
	if err != nil {
		fmt.Fprintf(stderr, "初始化 TeamOps 工具失败: %v\n", err)
		return 1
	}
	agenticModel, err := harness.NewAgenticModel(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "初始化 AgenticModel 失败: %v\n", err)
		return 1
	}
	runtime, err := harness.NewRuntime(ctx, agenticModel, []tool.BaseTool{teamOpsTool}, logger)
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
