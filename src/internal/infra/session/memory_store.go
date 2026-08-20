package session

import (
	"context"
	"sync"

	"github.com/naifenmizuha/basetion/src/internal/application/conversation"
)

// MemoryStore holds completed sessions only for the lifetime of one process.
// It is used by the batch-test entrypoint so test conversations never create
// files in the configured production Session directory.
type MemoryStore struct {
	mu       sync.Mutex
	sessions map[string]conversation.Session
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{sessions: make(map[string]conversation.Session)}
}

func (s *MemoryStore) Load(ctx context.Context, sessionID string) (conversation.Session, error) {
	if err := ctx.Err(); err != nil {
		return conversation.Session{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return conversation.Session{}, conversation.ErrSessionNotFound
	}
	return session, nil
}

func (s *MemoryStore) Save(ctx context.Context, session conversation.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	return nil
}

var _ conversation.SessionStore = (*MemoryStore)(nil)
