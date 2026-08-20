package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	if err := os.WriteFile(inputPath, []byte(`[[test.alpha]]
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
	code := ExecuteTest(context.Background(), []string{"--input", inputPath, "--output", outputPath, "--max-concurrency", "2"}, conversation, &stdout, &stderr)
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

func TestDefaultTestOutputPathUsesPrivateResultsDirectory(t *testing.T) {
	got := defaultTestOutputPath("testdata/example.toml", "run-123")
	want := filepath.Join(".basetion", "test-results", "example-run-123.jsonl")
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
