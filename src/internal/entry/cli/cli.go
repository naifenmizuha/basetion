package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"golang.org/x/term"
)

const usage = "用法: basetion [--profile run|dev] [--session-id <ID>] <提示词>\n       basetion [--profile run|dev] test --input <cases.toml> [--output <results.jsonl>]"

const (
	ProfileRun = "run"
	ProfileDev = "dev"
)

// ExtractProfile resolves the bootstrap profile and removes its flag before
// the remaining CLI arguments are parsed.
func ExtractProfile(args []string) (string, []string, error) {
	profile := ProfileRun
	remaining := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--profile":
			if index+1 >= len(args) {
				return "", nil, errors.New("--profile requires run or dev")
			}
			index++
			profile = args[index]
		case strings.HasPrefix(argument, "--profile="):
			profile = strings.TrimPrefix(argument, "--profile=")
		default:
			remaining = append(remaining, argument)
		}
	}
	if profile != ProfileRun && profile != ProfileDev {
		return "", nil, fmt.Errorf("unknown profile %q: want run or dev", profile)
	}
	return profile, remaining, nil
}

// Conversation is the entry-facing application use-case contract.
type Conversation interface {
	Run(ctx context.Context, sessionID, prompt string) *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]]
}

// Execute parses one CLI invocation, runs a conversation turn, and returns a process exit code.
func Execute(ctx context.Context, args []string, conversation Conversation, stdout, stderr io.Writer) int {
	if IsTestInvocation(args) {
		return ExecuteTest(ctx, args[1:], conversation, stdout, stderr)
	}
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
	if prompt == "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	resolvedSessionID := strings.TrimSpace(*sessionID)
	if resolvedSessionID == "" {
		var err error
		resolvedSessionID, err = generateSessionID()
		if err != nil {
			fmt.Fprintf(stderr, "创建会话 ID 失败: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "[会话] 新建 %s\n", resolvedSessionID)
	}
	if err := Render(stdout, conversation.Run(ctx, resolvedSessionID, prompt)); err != nil {
		fmt.Fprintf(stderr, "执行失败: %v\n", err)
		return 1
	}
	return 0
}

// IsTestInvocation reports whether args select the batch-test command. The
// bootstrap composition root uses it to choose an ephemeral SessionStore.
func IsTestInvocation(args []string) bool {
	return len(args) > 0 && args[0] == "test"
}

func generateSessionID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "session-" + hex.EncodeToString(random[:]), nil
}

// Render writes Eino events in order without flattening their semantic block types.
func Render(w io.Writer, events *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]]) error {
	if events == nil {
		return errors.New("event stream is nil")
	}
	renderer, err := newReplyRenderer(w)
	if err != nil {
		return fmt.Errorf("创建 Markdown 渲染器: %w", err)
	}
	for {
		event, ok := events.Next()
		if !ok {
			if err := renderer.flush(); err != nil {
				return err
			}
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
			if err := renderer.flush(); err != nil {
				return err
			}
			renderAction(w, event.Action)
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		message, err := event.Output.MessageOutput.GetMessage()
		if err != nil {
			return fmt.Errorf("读取 Agent 输出: %w", err)
		}
		if err := renderer.renderMessage(message); err != nil {
			return err
		}
	}
}

type replyRenderer struct {
	w        io.Writer
	markdown *glamour.TermRenderer
	reply    strings.Builder
}

func newReplyRenderer(w io.Writer) (*replyRenderer, error) {
	if !isTerminal(w) {
		return &replyRenderer{w: w}, nil
	}
	markdown, err := glamour.NewTermRenderer(glamour.WithAutoStyle())
	if err != nil {
		return nil, err
	}
	return &replyRenderer{w: w, markdown: markdown}, nil
}

func isTerminal(w io.Writer) bool {
	type fileDescriptor interface {
		Fd() uintptr
	}
	file, ok := w.(fileDescriptor)
	return ok && term.IsTerminal(int(file.Fd()))
}

func (r *replyRenderer) renderMessage(message *schema.AgenticMessage) error {
	if message == nil {
		return nil
	}
	for _, block := range message.ContentBlocks {
		if block == nil {
			continue
		}
		switch {
		case block.Reasoning != nil:
			if err := r.flush(); err != nil {
				return err
			}
			fmt.Fprintf(r.w, "[思考] %s\n", block.Reasoning.Text)
		case block.AssistantGenText != nil:
			r.reply.WriteString(block.AssistantGenText.Text)
		case block.FunctionToolCall != nil:
			if err := r.flush(); err != nil {
				return err
			}
			fmt.Fprintf(r.w, "\n[工具调用] %s call_id=%s 参数=%s\n", block.FunctionToolCall.Name, block.FunctionToolCall.CallID, block.FunctionToolCall.Arguments)
		case block.FunctionToolResult != nil:
			if err := r.flush(); err != nil {
				return err
			}
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
			fmt.Fprintf(r.w, "[工具结果] %s call_id=%s: %s\n", block.FunctionToolResult.Name, block.FunctionToolResult.CallID, strings.Join(parts, ""))
		default:
			if err := r.flush(); err != nil {
				return err
			}
			fmt.Fprintf(r.w, "[内容] %s\n", block.Type)
		}
	}
	return nil
}

func (r *replyRenderer) flush() error {
	if r.reply.Len() == 0 {
		return nil
	}
	markdown := r.reply.String()
	r.reply.Reset()
	rendered := markdown
	if r.markdown != nil {
		var err error
		rendered, err = r.markdown.Render(markdown)
		if err != nil {
			return fmt.Errorf("渲染回复 Markdown: %w", err)
		}
	}
	if _, err := fmt.Fprintln(r.w, "[回复]"); err != nil {
		return err
	}
	if _, err := fmt.Fprint(r.w, rendered); err != nil {
		return err
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
