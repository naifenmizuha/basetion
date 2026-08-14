package conversation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

// ErrSessionNotFound indicates that no completed conversation exists for an ID.
var ErrSessionNotFound = errors.New("session not found")

// Session is durable business conversation state. It is deliberately separate
// from Eino checkpoint state, which represents an interrupted in-flight run.
type Session struct {
	ID        string                   `json:"id"`
	Messages  []*schema.AgenticMessage `json:"messages"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
}

// NewSession creates an empty conversation session.
func NewSession(id string, now time.Time) Session {
	return Session{
		ID:        strings.TrimSpace(id),
		Messages:  make([]*schema.AgenticMessage, 0),
		CreatedAt: now.UTC(),
		UpdatedAt: now.UTC(),
	}
}

// SessionStore persists completed conversation state.
type SessionStore interface {
	Load(ctx context.Context, sessionID string) (Session, error)
	Save(ctx context.Context, session Session) error
}
