package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/naifenmizuha/basetion/src/internal/harness"
	"github.com/pelletier/go-toml/v2"
)

const defaultTestConcurrency = 4

type testCases struct {
	Runs []testRun
}

type testCasesFile struct {
	Test map[string][]testCase `toml:"test"`
}

type testRun struct {
	Name  string
	Cases []testCase
}

type testCase struct {
	Prompt       string   `toml:"prompt"`
	ExpectTools  []string `toml:"expect_tools"`
	ExpectSkills []string `toml:"expect_skills"`
	ForbidTools  []string `toml:"forbid_tools"`
}

type testResult struct {
	SchemaVersion int                         `json:"schema_version"`
	RunID         string                      `json:"run_id"`
	StartedAt     time.Time                   `json:"started_at"`
	FinishedAt    time.Time                   `json:"finished_at"`
	RunName       string                      `json:"run_name"`
	SessionID     string                      `json:"session_id"`
	SessionIndex  int                         `json:"session_index"`
	TurnIndex     int                         `json:"turn_index"`
	Prompt        string                      `json:"prompt"`
	ExpectTools   []string                    `json:"expect_tools,omitempty"`
	ExpectSkills  []string                    `json:"expect_skills,omitempty"`
	ForbidTools   []string                    `json:"forbid_tools,omitempty"`
	Status        string                      `json:"status"`
	Error         string                      `json:"error,omitempty"`
	Events        []testAgentEvent            `json:"events"`
	ModelRequests []harness.ModelRequestTrace `json:"model_requests"`
}

type testAgentEvent struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message,omitempty"`
	Action  json.RawMessage `json:"action,omitempty"`
}

// ExecuteTest runs the named TOML test tables. It is exported for
// entry-adapter tests; production callers reach it through Execute's test
// subcommand.
func ExecuteTest(ctx context.Context, args []string, conversation Conversation, stdout, stderr io.Writer) int {
	if conversation == nil {
		fmt.Fprintln(stderr, "启动失败: conversation service is required")
		return 1
	}
	flags := flag.NewFlagSet("basetion test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	inputPath := flags.String("input", "", "测试用例 TOML")
	outputPath := flags.String("output", "", "结果 JSONL")
	maxConcurrency := flags.Int("max-concurrency", defaultTestConcurrency, "最大并发 Session 数")
	if err := flags.Parse(args); err != nil || len(flags.Args()) != 0 {
		if err != nil {
			fmt.Fprintf(stderr, "%s\n", err)
		}
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if strings.TrimSpace(*inputPath) == "" || *maxConcurrency <= 0 {
		fmt.Fprintln(stderr, "test 需要非空的 --input，且 --max-concurrency 必须为正数")
		return 2
	}
	cases, err := loadTestCases(*inputPath)
	if err != nil {
		fmt.Fprintf(stderr, "读取测试用例失败: %v\n", err)
		return 2
	}
	runID, err := newTestRunID()
	if err != nil {
		fmt.Fprintf(stderr, "创建测试运行 ID 失败: %v\n", err)
		return 1
	}
	resolvedOutputPath := strings.TrimSpace(*outputPath)
	if resolvedOutputPath == "" {
		resolvedOutputPath = defaultTestOutputPath(*inputPath, time.Now())
	}
	writer, err := newTestResultWriter(resolvedOutputPath)
	if err != nil {
		fmt.Fprintf(stderr, "创建测试结果失败: %v\n", err)
		return 2
	}
	defer writer.Close()

	fmt.Fprintf(stdout, "[测试] run_id=%s runs=%d max_concurrency=%d output=%s\n", runID, len(cases.Runs), *maxConcurrency, resolvedOutputPath)
	semaphore := make(chan struct{}, *maxConcurrency)
	var group sync.WaitGroup
	var failures int
	var resultMu sync.Mutex
	for sessionIndex, run := range cases.Runs {
		group.Add(1)
		go func(sessionIndex int, run testRun) {
			defer group.Done()
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-semaphore }()
			sessionID := fmt.Sprintf("test-%s-%s", runID, run.Name)
			for turnIndex, testCase := range run.Cases {
				result := runTestTurn(ctx, conversation, runID, run.Name, sessionID, sessionIndex, turnIndex, testCase)
				if err := writer.Write(result); err != nil {
					result.Status = "failed"
					result.Error = fmt.Sprintf("write test result: %v", err)
				}
				resultMu.Lock()
				fmt.Fprintf(stdout, "[测试] run=%s turn=%d status=%s\n", run.Name, turnIndex+1, result.Status)
				if result.Status != "completed" {
					failures++
				}
				resultMu.Unlock()
			}
		}(sessionIndex, run)
	}
	group.Wait()
	resultMu.Lock()
	failed := failures
	resultMu.Unlock()
	fmt.Fprintf(stdout, "[测试] 完成 failures=%d\n", failed)
	if failed > 0 {
		return 1
	}
	return 0
}

func loadTestCases(path string) (testCases, error) {
	file, err := os.Open(strings.TrimSpace(path))
	if err != nil {
		return testCases{}, err
	}
	defer file.Close()
	var source testCasesFile
	if err := toml.NewDecoder(file).DisallowUnknownFields().Decode(&source); err != nil {
		return testCases{}, err
	}
	if len(source.Test) == 0 {
		return testCases{}, errors.New("test must contain at least one named run")
	}
	names := make([]string, 0, len(source.Test))
	for name := range source.Test {
		names = append(names, name)
	}
	sort.Strings(names)
	cases := testCases{Runs: make([]testRun, 0, len(names))}
	for _, name := range names {
		if !validTestRunName(name) {
			return testCases{}, fmt.Errorf("test run name %q must use letters, digits, hyphens, or underscores", name)
		}
		entries := source.Test[name]
		if len(entries) == 0 {
			return testCases{}, fmt.Errorf("test.%s must contain at least one table", name)
		}
		run := testRun{Name: name, Cases: make([]testCase, 0, len(entries))}
		for index, entry := range entries {
			prompt := strings.TrimSpace(entry.Prompt)
			if prompt == "" {
				return testCases{}, fmt.Errorf("test.%s[%d].prompt must not be blank", name, index)
			}
			expectTools, err := normalizeTestNames(entry.ExpectTools, fmt.Sprintf("test.%s[%d].expect_tools", name, index))
			if err != nil {
				return testCases{}, err
			}
			expectSkills, err := normalizeTestNames(entry.ExpectSkills, fmt.Sprintf("test.%s[%d].expect_skills", name, index))
			if err != nil {
				return testCases{}, err
			}
			forbidTools, err := normalizeTestNames(entry.ForbidTools, fmt.Sprintf("test.%s[%d].forbid_tools", name, index))
			if err != nil {
				return testCases{}, err
			}
			for _, toolName := range expectTools {
				if containsTestName(forbidTools, toolName) {
					return testCases{}, fmt.Errorf("test.%s[%d] both expects and forbids tool %q", name, index, toolName)
				}
			}
			run.Cases = append(run.Cases, testCase{
				Prompt: prompt, ExpectTools: expectTools, ExpectSkills: expectSkills, ForbidTools: forbidTools,
			})
		}
		cases.Runs = append(cases.Runs, run)
	}
	return cases, nil
}

func normalizeTestNames(values []string, field string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name == "" {
			return nil, fmt.Errorf("%s must not contain blank names", field)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("%s must not contain duplicate name %q", field, name)
		}
		seen[name] = struct{}{}
		normalized = append(normalized, name)
	}
	return normalized, nil
}

func containsTestName(values []string, name string) bool {
	for _, value := range values {
		if value == name {
			return true
		}
	}
	return false
}

func validTestRunName(name string) bool {
	if name == "" {
		return false
	}
	for _, character := range name {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '-' && character != '_' {
			return false
		}
	}
	return true
}

func runTestTurn(ctx context.Context, conversation Conversation, runID, runName, sessionID string, sessionIndex, turnIndex int, testCase testCase) testResult {
	startedAt := time.Now().UTC()
	trace := harness.NewTurnTrace()
	events, runErr := collectTestEvents(conversation.Run(harness.WithTurnTrace(ctx, trace), sessionID, testCase.Prompt))
	result := testResult{
		SchemaVersion: 2,
		RunID:         runID, StartedAt: startedAt, FinishedAt: time.Now().UTC(), RunName: runName,
		SessionID: sessionID, SessionIndex: sessionIndex + 1, TurnIndex: turnIndex + 1,
		Prompt: testCase.Prompt, ExpectTools: testCase.ExpectTools, ExpectSkills: testCase.ExpectSkills, ForbidTools: testCase.ForbidTools,
		Status: "completed", Events: events, ModelRequests: trace.Snapshot(),
	}
	if runErr != nil {
		result.Status = "failed"
		result.Error = runErr.Error()
	}
	return result
}

func collectTestEvents(events *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]]) ([]testAgentEvent, error) {
	if events == nil {
		return nil, errors.New("event stream is nil")
	}
	result := make([]testAgentEvent, 0)
	for {
		event, ok := events.Next()
		if !ok {
			return result, nil
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			return result, event.Err
		}
		if event.Action != nil {
			result = append(result, testAgentEvent{Type: "action", Action: marshalJSON(event.Action)})
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		message, err := event.Output.MessageOutput.GetMessage()
		if err != nil {
			return result, fmt.Errorf("read agent output: %w", err)
		}
		if message != nil {
			result = append(result, testAgentEvent{Type: "message", Message: marshalJSON(message)})
		}
	}
}

type testResultWriter struct {
	file   *os.File
	writer *bufio.Writer
	mu     sync.Mutex
}

func newTestResultWriter(path string) (*testResultWriter, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("output path must not be blank")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	return &testResultWriter{file: file, writer: bufio.NewWriter(file)}, nil
}

func defaultTestOutputPath(inputPath string, startedAt time.Time) string {
	base := filepath.Base(strings.TrimSpace(inputPath))
	name := strings.TrimSuffix(base, filepath.Ext(base))
	if name == "" || name == "." {
		name = "test"
	}
	return filepath.Join(".basetion", "test-results", name+"-"+startedAt.Format("060102150405"), "log.jsonl")
}

func (w *testResultWriter) Write(result testResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.writer.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := w.writer.Flush(); err != nil {
		return err
	}
	return w.file.Sync()
}

func (w *testResultWriter) Close() error {
	if w == nil || w.file == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.writer.Flush(); err != nil {
		_ = w.file.Close()
		return err
	}
	return w.file.Close()
}

func newTestRunID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(random[:]), nil
}

func marshalJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{"trace_encoding_error":true}`)
	}
	return data
}
