package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

const usage = "用法: basetion --session-id <ID> <提示词>"

// Conversation is the entry-facing application use-case contract.
type Conversation interface {
	Run(ctx context.Context, sessionID, prompt string) *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]]
}

// Execute parses one CLI invocation, runs a conversation turn, and returns a process exit code.
func Execute(ctx context.Context, args []string, conversation Conversation, stdout, stderr io.Writer) int {
	if conversation == nil {
		fmt.Fprintln(stderr, "启动失败: conversation service is required")
		return 1
	}
	flags := flag.NewFlagSet("basetion", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sessionID := flags.String("session-id", "", "业务会话 ID")
	if err := flags.Parse(args); err != nil {
		fmt.Fprintf(stderr, "%s\n%s\n", err, usage)
		return 2
	}
	prompt := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if strings.TrimSpace(*sessionID) == "" || prompt == "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if err := Render(stdout, conversation.Run(ctx, *sessionID, prompt)); err != nil {
		fmt.Fprintf(stderr, "执行失败: %v\n", err)
		return 1
	}
	return 0
}

// Render writes Eino events in order without flattening their semantic block types.
func Render(w io.Writer, events *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]]) error {
	if events == nil {
		return errors.New("event stream is nil")
	}
	for {
		event, ok := events.Next()
		if !ok {
			fmt.Fprintln(w, "\n[完成]")
			return nil
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			return event.Err
		}
		if event.Action != nil {
			renderAction(w, event.Action)
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		message, err := event.Output.MessageOutput.GetMessage()
		if err != nil {
			return fmt.Errorf("读取 Agent 输出: %w", err)
		}
		if err := renderMessage(w, message); err != nil {
			return err
		}
	}
}

func renderMessage(w io.Writer, message *schema.AgenticMessage) error {
	if message == nil {
		return nil
	}
	for _, block := range message.ContentBlocks {
		if block == nil {
			continue
		}
		switch {
		case block.Reasoning != nil:
			fmt.Fprintf(w, "[思考] %s\n", block.Reasoning.Text)
		case block.AssistantGenText != nil:
			fmt.Fprint(w, block.AssistantGenText.Text)
		case block.FunctionToolCall != nil:
			fmt.Fprintf(w, "\n[工具调用] %s call_id=%s 参数=%s\n", block.FunctionToolCall.Name, block.FunctionToolCall.CallID, block.FunctionToolCall.Arguments)
		case block.FunctionToolResult != nil:
			parts := make([]string, 0, len(block.FunctionToolResult.Content))
			for _, content := range block.FunctionToolResult.Content {
				if content == nil {
					continue
				}
				if content.Text != nil {
					parts = append(parts, content.Text.Text)
				} else {
					parts = append(parts, content.String())
				}
			}
			fmt.Fprintf(w, "[工具结果] %s call_id=%s: %s\n", block.FunctionToolResult.Name, block.FunctionToolResult.CallID, strings.Join(parts, ""))
		default:
			fmt.Fprintf(w, "[内容] %s\n", block.Type)
		}
	}
	return nil
}

func renderAction(w io.Writer, action *adk.AgentAction) {
	switch {
	case action.Interrupted != nil:
		fmt.Fprintln(w, "[动作] 执行已中断")
	case action.Exit:
		fmt.Fprintln(w, "[动作] Agent 请求结束")
	case action.BreakLoop != nil:
		fmt.Fprintln(w, "[动作] Agent 循环结束")
	case action.TransferToAgent != nil:
		fmt.Fprintf(w, "[动作] 转交 Agent: %s\n", action.TransferToAgent.DestAgentName)
	default:
		fmt.Fprintln(w, "[动作] 自定义动作")
	}
}
