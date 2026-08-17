package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/naifenmizuha/basetion/src/internal/application/conversation"
)

func TestFileStoreRoundTripAcrossInstances(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Nanosecond)
	want := conversation.NewSession("../../unsafe/session", now)
	want.Messages = []*schema.AgenticMessage{
		schema.UserAgenticMessage("hello"),
		{
			Role: schema.AgenticRoleTypeAssistant,
			ContentBlocks: []*schema.ContentBlock{
				schema.NewContentBlock(&schema.Reasoning{Text: "summary", Signature: "signature"}),
				schema.NewContentBlock(&schema.FunctionToolCall{CallID: "call-1", Name: "skill", Arguments: `{"skill":"project-knowledge"}`}),
			},
		},
		{
			Role: schema.AgenticRoleTypeUser,
			ContentBlocks: []*schema.ContentBlock{
				schema.NewContentBlock(&schema.FunctionToolResult{
					CallID: "call-1",
					Name:   "skill",
					Content: []*schema.FunctionToolResultContentBlock{{
						Type: schema.FunctionToolResultContentBlockTypeText,
						Text: &schema.UserInputText{Text: "Go is a programming language."},
					}},
				}),
			},
		},
	}
	want.UpdatedAt = now.Add(time.Second)

	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	reopened, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Load(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch\ngot:  %#v\nwant: %#v", got, want)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || filepath.Ext(entries[0].Name()) != ".json" {
		t.Fatalf("unexpected session files: %#v", entries)
	}
}

func TestFileStoreNotFoundAndCorrupt(t *testing.T) {
	t.Parallel()

	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Load(context.Background(), "missing")
	if !errors.Is(err, conversation.ErrSessionNotFound) {
		t.Fatalf("Load() error = %v, want ErrSessionNotFound", err)
	}

	path, err := store.path("broken")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background(), "broken"); err == nil {
		t.Fatal("Load() accepted corrupt data")
	}
}

func TestFileStoreFailedSavePreservesCompletedSnapshot(t *testing.T) {
	t.Parallel()

	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	completed := conversation.NewSession("stable", now)
	completed.Messages = []*schema.AgenticMessage{schema.UserAgenticMessage("complete")}
	if err := store.Save(context.Background(), completed); err != nil {
		t.Fatal(err)
	}

	invalid := completed
	invalid.Messages = []*schema.AgenticMessage{{Extra: map[string]any{"unsupported": make(chan int)}}}
	if err := store.Save(context.Background(), invalid); err == nil {
		t.Fatal("Save() accepted an unsupported extension")
	}
	got, err := store.Load(context.Background(), completed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, completed) {
		t.Fatalf("failed save replaced completed state: %#v", got)
	}
}
