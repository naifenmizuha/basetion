package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type batchTestConversation struct {
	mu        sync.Mutex
	prompts   map[string][]string
	active    int
	maxActive int
}

func (c *batchTestConversation) Run(_ context.Context, sessionID, prompt string) *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]] {
	iter, generator := adk.NewAsyncIteratorPair[*adk.TypedAgentEvent[*schema.AgenticMessage]]()
	go func() {
		c.mu.Lock()
		c.active++
		if c.active > c.maxActive {
			c.maxActive = c.active
		}
		c.prompts[sessionID] = append(c.prompts[sessionID], prompt)
		c.mu.Unlock()
		defer func() {
			c.mu.Lock()
			c.active--
			c.mu.Unlock()
			generator.Close()
		}()
		time.Sleep(15 * time.Millisecond)
		generator.Send(adk.EventFromAgenticMessage(nil, schema.StreamReaderFromArray([]*schema.AgenticMessage{{
			Role: schema.AgenticRoleTypeAssistant,
			ContentBlocks: []*schema.ContentBlock{
				schema.NewContentBlock(&schema.Reasoning{Text: "摘要"}),
				schema.NewContentBlock(&schema.AssistantGenText{Text: "reply: " + prompt}),
			},
		}}), schema.AgenticRoleTypeAssistant))
	}()
	return iter
}

func TestExecuteTestRunsNamedTablesAndWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "cases.toml")
	outputPath := filepath.Join(dir, "result.jsonl")
	if err := os.WriteFile(inputPath, []byte(`[settings]
max_concurrency = 2

[[test.alpha]]
prompt = "a1"
expect_tools = ["lookup"]
expect_skills = ["project-knowledge"]
forbid_tools = ["write"]

[[test.alpha]]
prompt = "a2"

[[test.beta]]
prompt = "b1"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	conversation := &batchTestConversation{prompts: make(map[string][]string)}
	var stdout, stderr bytes.Buffer
	code := ExecuteTest(context.Background(), []string{"--input", inputPath, "--output", outputPath}, conversation, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("result lines=%d data=%s", len(lines), data)
	}
	if bytes.Contains(data, []byte(`"input"`)) || bytes.Contains(data, []byte(`"output"`)) || bytes.Contains(data, []byte(`"status_bar"`)) {
		t.Fatalf("JSONL contains model request contents: %s", data)
	}
	seen := make(map[string]int)
	runForPrompt := make(map[string]string)
	for _, line := range lines {
		var result testResult
		if err := json.Unmarshal(line, &result); err != nil {
			t.Fatal(err)
		}
		if result.Status != "completed" || result.RunID == "" || result.RunName == "" || result.SessionID == "" || len(result.Events) != 1 {
			t.Fatalf("unexpected result: %#v", result)
		}
		if result.SchemaVersion != 2 {
			t.Fatalf("schema version=%d", result.SchemaVersion)
		}
		seen[result.Prompt]++
		runForPrompt[result.Prompt] = result.RunName
		if result.Prompt == "a1" && (!containsTestName(result.ExpectTools, "lookup") || !containsTestName(result.ExpectSkills, "project-knowledge") || !containsTestName(result.ForbidTools, "write")) {
			t.Fatalf("expectations were not recorded: %#v", result)
		}
	}
	if len(seen) != 3 || seen["a1"] != 1 || seen["a2"] != 1 || seen["b1"] != 1 {
		t.Fatalf("result prompts=%v", seen)
	}
	if runForPrompt["a1"] != "alpha" || runForPrompt["a2"] != "alpha" || runForPrompt["b1"] != "beta" {
		t.Fatalf("run names=%v", runForPrompt)
	}
	conversation.mu.Lock()
	defer conversation.mu.Unlock()
	if conversation.maxActive != 2 {
		t.Fatalf("max active sessions=%d, want 2", conversation.maxActive)
	}
	for sessionID, prompts := range conversation.prompts {
		if len(prompts) == 2 && (prompts[0] != "a1" || prompts[1] != "a2") {
			t.Fatalf("session %s prompts=%v", sessionID, prompts)
		}
	}
}

func TestExecuteTestRunsSessionsSeriallyWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "cases.toml")
	outputPath := filepath.Join(dir, "result.jsonl")
	if err := os.WriteFile(inputPath, []byte(`[settings]
max_concurrency = 1

[[test.alpha]]
prompt = "a1"

[[test.beta]]
prompt = "b1"

[[test.gamma]]
prompt = "c1"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	conversation := &batchTestConversation{prompts: make(map[string][]string)}
	var stdout, stderr bytes.Buffer
	code := ExecuteTest(context.Background(), []string{"--input", inputPath, "--output", outputPath}, conversation, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	conversation.mu.Lock()
	defer conversation.mu.Unlock()
	if conversation.maxActive != 1 {
		t.Fatalf("max active sessions=%d, want 1", conversation.maxActive)
	}
}

func TestExecuteTestRejectsUnknownConcurrencyFlag(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "cases.toml")
	outputPath := filepath.Join(dir, "result.jsonl")
	if err := os.WriteFile(inputPath, []byte("[[test.ok]]\nprompt = \"ok\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := ExecuteTest(context.Background(), []string{"--input", inputPath, "--output", outputPath, "--max-concurrency", "2"}, &batchTestConversation{prompts: make(map[string][]string)}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("removed flag accepted, code=%d stderr=%s", code, stderr.String())
	}
}

func TestLoadTestCasesDefaultsAndValidatesConcurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cases.toml")
	if err := os.WriteFile(path, []byte("[[test.ok]]\nprompt = \"ok\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases, err := loadTestCases(path)
	if err != nil {
		t.Fatal(err)
	}
	if cases.MaxConcurrency != defaultTestConcurrency {
		t.Fatalf("default concurrency=%d want %d", cases.MaxConcurrency, defaultTestConcurrency)
	}

	if err := os.WriteFile(path, []byte("[settings]\nmax_concurrency = 3\n\n[[test.ok]]\nprompt = \"ok\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases, err = loadTestCases(path)
	if err != nil {
		t.Fatal(err)
	}
	if cases.MaxConcurrency != 3 {
		t.Fatalf("concurrency=%d want 3", cases.MaxConcurrency)
	}

	for _, value := range []string{"0", "-1"} {
		if err := os.WriteFile(path, []byte("[settings]\nmax_concurrency = "+value+"\n\n[[test.ok]]\nprompt = \"ok\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadTestCases(path); err == nil {
			t.Fatalf("invalid max_concurrency %s accepted", value)
		}
	}

	if err := os.WriteFile(path, []byte("[settings]\nunknown = 1\n\n[[test.ok]]\nprompt = \"ok\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadTestCases(path); err == nil {
		t.Fatal("unknown settings key accepted")
	}
}

func TestDefaultTestOutputPathUsesTimestampedResultDirectory(t *testing.T) {
	got := defaultTestOutputPath("testdata/example.toml", time.Date(2026, 8, 20, 19, 24, 56, 0, time.Local))
	want := filepath.Join(".basetion", "test-results", "example-260820192456", "log.jsonl")
	if got != want {
		t.Fatalf("default output path=%q want %q", got, want)
	}
}

func TestLoadTestCasesGroupsNamedTablesInSortedRunOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cases.toml")
	if err := os.WriteFile(path, []byte(`[[test.zebra]]
prompt = "z1"

[[test.alpha]]
prompt = "a1"

[[test.zebra]]
prompt = "z2"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cases, err := loadTestCases(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases.Runs) != 2 || cases.Runs[0].Name != "alpha" || cases.Runs[1].Name != "zebra" {
		t.Fatalf("runs=%#v", cases.Runs)
	}
	if got := cases.Runs[1].Cases; len(got) != 2 || got[0].Prompt != "z1" || got[1].Prompt != "z2" {
		t.Fatalf("zebra cases=%v", got)
	}
}

func TestExecuteTestRejectsInvalidCasesAndExistingOutput(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "cases.toml")
	outputPath := filepath.Join(dir, "result.jsonl")
	if err := os.WriteFile(inputPath, []byte("prompts = [[]]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := ExecuteTest(context.Background(), []string{"--input", inputPath, "--output", outputPath}, &batchTestConversation{prompts: make(map[string][]string)}, &stdout, &stderr); code != 2 {
		t.Fatalf("invalid cases code=%d stderr=%s", code, stderr.String())
	}
	if err := os.WriteFile(inputPath, []byte("[[test.ok]]\nprompt = \"ok\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := ExecuteTest(context.Background(), []string{"--input", inputPath, "--output", outputPath}, &batchTestConversation{prompts: make(map[string][]string)}, &stdout, &stderr); code != 2 {
		t.Fatalf("existing output code=%d stderr=%s", code, stderr.String())
	}
}

func TestLoadTestCasesRejectsInvalidNamedTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cases.toml")
	for _, contents := range []string{
		"[[test.bad]]\nprompt = \"   \"\n",
		"[[test.bad]]\nprompt = \"ok\"\nunknown = true\n",
		"[[test.\"bad name\"]]\nprompt = \"ok\"\n",
		"[[test.bad]]\nprompt = \"ok\"\nexpect_tools = [\"team_query\", \"team_query\"]\n",
		"[[test.bad]]\nprompt = \"ok\"\nexpect_tools = [\"team_query\"]\nforbid_tools = [\"team_query\"]\n",
	} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadTestCases(path); err == nil {
			t.Fatalf("invalid cases accepted: %s", contents)
		}
	}
}

func TestLoadTestCasesValidatesDependsOn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cases.toml")
	for _, contents := range []string{
		// 引用未知 run
		"[[test.alpha]]\nprompt = \"a\"\ndepends_on = [\"missing\"]\n",
		// 自依赖
		"[[test.alpha]]\nprompt = \"a\"\ndepends_on = [\"alpha\"]\n",
		// 循环 a→b→a
		"[[test.a]]\nprompt = \"a\"\ndepends_on = [\"b\"]\n\n[[test.b]]\nprompt = \"b\"\ndepends_on = [\"a\"]\n",
		// 循环 a→b→c→a
		"[[test.a]]\nprompt = \"a\"\ndepends_on = [\"c\"]\n\n[[test.b]]\nprompt = \"b\"\ndepends_on = [\"a\"]\n\n[[test.c]]\nprompt = \"c\"\ndepends_on = [\"b\"]\n",
		// 非首表带 depends_on
		"[[test.alpha]]\nprompt = \"a1\"\n\n[[test.alpha]]\nprompt = \"a2\"\ndepends_on = [\"beta\"]\n\n[[test.beta]]\nprompt = \"b\"\n",
		// 重复依赖名
		"[[test.alpha]]\nprompt = \"a\"\ndepends_on = [\"beta\", \"beta\"]\n\n[[test.beta]]\nprompt = \"b\"\n",
	} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadTestCases(path); err == nil {
			t.Fatalf("invalid depends_on accepted: %s", contents)
		}
	}
}

func TestLoadTestCasesParsesDependsOn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cases.toml")
	if err := os.WriteFile(path, []byte(`[[test.write]]
prompt = "w"

[[test.read]]
depends_on = ["write"]
prompt = "r"

[[test.zeta]]
depends_on = ["read"]
prompt = "z"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cases, err := loadTestCases(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases.Runs) != 3 {
		t.Fatalf("runs=%d", len(cases.Runs))
	}
	order := []string{cases.Runs[0].Name, cases.Runs[1].Name, cases.Runs[2].Name}
	if order[0] != "write" || order[1] != "read" || order[2] != "zeta" {
		t.Fatalf("topological order=%v", order)
	}
	if got := cases.Runs[1].DependsOn; len(got) != 1 || got[0] != "write" {
		t.Fatalf("read depends_on=%v", got)
	}
	if got := cases.Runs[2].DependsOn; len(got) != 1 || got[0] != "read" {
		t.Fatalf("zeta depends_on=%v", got)
	}
}

type orderedTestConversation struct {
	mu     sync.Mutex
	events []string
	fail   map[string]bool
}

func (c *orderedTestConversation) Run(_ context.Context, sessionID, prompt string) *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]] {
	iter, generator := adk.NewAsyncIteratorPair[*adk.TypedAgentEvent[*schema.AgenticMessage]]()
	go func() {
		c.mu.Lock()
		c.events = append(c.events, "start:"+sessionID+":"+prompt)
		shouldFail := c.fail[sessionID]
		c.mu.Unlock()
		defer func() {
			c.mu.Lock()
			c.events = append(c.events, "end:"+sessionID+":"+prompt)
			c.mu.Unlock()
			generator.Close()
		}()
		time.Sleep(10 * time.Millisecond)
		if shouldFail {
			generator.Send(&adk.TypedAgentEvent[*schema.AgenticMessage]{Err: errors.New("forced failure")})
			return
		}
		generator.Send(adk.EventFromAgenticMessage(nil, schema.StreamReaderFromArray([]*schema.AgenticMessage{{
			Role:          schema.AgenticRoleTypeAssistant,
			ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text: "ok"})},
		}}), schema.AgenticRoleTypeAssistant))
	}()
	return iter
}

func TestExecuteTestRunsDependencyInOrder(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "cases.toml")
	outputPath := filepath.Join(dir, "result.jsonl")
	if err := os.WriteFile(inputPath, []byte(`[settings]
max_concurrency = 2

[[test.write]]
prompt = "w1"

[[test.read]]
depends_on = ["write"]
prompt = "r1"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	conversation := &orderedTestConversation{fail: make(map[string]bool)}
	var stdout, stderr bytes.Buffer
	code := ExecuteTest(context.Background(), []string{"--input", inputPath, "--output", outputPath}, conversation, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	conversation.mu.Lock()
	defer conversation.mu.Unlock()
	var writeEnd, readStart int
	for i, event := range conversation.events {
		if strings.HasPrefix(event, "end:") && strings.Contains(event, ":w1") {
			writeEnd = i
		}
		if strings.HasPrefix(event, "start:") && strings.Contains(event, ":r1") {
			readStart = i
		}
	}
	if readStart <= writeEnd {
		t.Fatalf("read started before write finished: events=%v", conversation.events)
	}
}

func TestExecuteTestSkipsDownstreamWhenUpstreamFails(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "cases.toml")
	outputPath := filepath.Join(dir, "result.jsonl")
	if err := os.WriteFile(inputPath, []byte(`[settings]
max_concurrency = 2

[[test.write]]
prompt = "w1"

[[test.read]]
depends_on = ["write"]
prompt = "r1"

[[test.downstream]]
depends_on = ["read"]
prompt = "d1"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	conversation := &orderedTestConversation{fail: make(map[string]bool)}
	conversationWithFail := &failBySessionIDConversation{inner: conversation, failSuffix: "-write"}
	var stdout, stderr bytes.Buffer
	code := ExecuteTest(context.Background(), []string{"--input", inputPath, "--output", outputPath}, conversationWithFail, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit code when upstream fails")
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("lines=%d data=%s", len(lines), data)
	}
	statusByRun := make(map[string]string)
	for _, line := range lines {
		var result testResult
		if err := json.Unmarshal(line, &result); err != nil {
			t.Fatal(err)
		}
		statusByRun[result.RunName] = result.Status
	}
	if statusByRun["write"] != "failed" {
		t.Fatalf("write status=%q", statusByRun["write"])
	}
	if statusByRun["read"] != "skipped" {
		t.Fatalf("read status=%q", statusByRun["read"])
	}
	if statusByRun["downstream"] != "skipped" {
		t.Fatalf("downstream status=%q", statusByRun["downstream"])
	}
}

// failBySessionIDConversation 包裹 orderedTestConversation，在匹配后缀的 sessionID 上注入失败。
type failBySessionIDConversation struct {
	inner      *orderedTestConversation
	failSuffix string
}

func (c *failBySessionIDConversation) Run(ctx context.Context, sessionID, prompt string) *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]] {
	if strings.HasSuffix(sessionID, c.failSuffix) {
		iter, generator := adk.NewAsyncIteratorPair[*adk.TypedAgentEvent[*schema.AgenticMessage]]()
		go func() {
			defer generator.Close()
			generator.Send(&adk.TypedAgentEvent[*schema.AgenticMessage]{Err: errors.New("forced failure")})
		}()
		return iter
	}
	return c.inner.Run(ctx, sessionID, prompt)
}
