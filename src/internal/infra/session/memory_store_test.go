package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/application/conversation"
)

func TestMemoryStoreKeepsSessionsOnlyInMemory(t *testing.T) {
	store := NewMemoryStore()
	if _, err := store.Load(context.Background(), "missing"); !errors.Is(err, conversation.ErrSessionNotFound) {
		t.Fatalf("missing error=%v", err)
	}
	want := conversation.NewSession("test-session", time.Time{})
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background(), "test-session")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID {
		t.Fatalf("session=%#v want=%#v", got, want)
	}
}
