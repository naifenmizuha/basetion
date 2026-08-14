package bootstrap

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/naifenmizuha/basetion/internal/application/conversation"
	"github.com/naifenmizuha/basetion/internal/config"
	"github.com/naifenmizuha/basetion/internal/domain/knowledge"
	"github.com/naifenmizuha/basetion/internal/entry/cli"
	"github.com/naifenmizuha/basetion/internal/harness"
	infraknowledge "github.com/naifenmizuha/basetion/internal/infra/knowledge"
	infrasession "github.com/naifenmizuha/basetion/internal/infra/session"
	basetiontools "github.com/naifenmizuha/basetion/internal/tools"
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
	retriever := infraknowledge.NewMemoryRetriever(infraknowledge.DefaultDocuments())
	knowledgeService, err := knowledge.NewService(retriever)
	if err != nil {
		fmt.Fprintf(stderr, "初始化知识领域服务失败: %v\n", err)
		return 1
	}
	knowledgeTool, err := basetiontools.NewKnowledgeSearch(knowledgeService)
	if err != nil {
		fmt.Fprintf(stderr, "初始化知识工具失败: %v\n", err)
		return 1
	}
	agenticModel, err := harness.NewAgenticModel(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "初始化 AgenticModel 失败: %v\n", err)
		return 1
	}
	runtime, err := harness.NewRuntime(ctx, agenticModel, []tool.BaseTool{knowledgeTool}, logger)
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
