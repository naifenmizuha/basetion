package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/naifenmizuha/basetion/internal/application/conversation"
)

// FileStore persists each completed session as an atomic JSON snapshot.
type FileStore struct {
	dir string
}

// NewFileStore creates a local session adapter rooted at dir.
func NewFileStore(dir string) (*FileStore, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("session directory must not be empty")
	}
	return &FileStore{dir: dir}, nil
}

// Load reads a completed session snapshot.
func (s *FileStore) Load(ctx context.Context, sessionID string) (conversation.Session, error) {
	if err := ctx.Err(); err != nil {
		return conversation.Session{}, err
	}
	path, err := s.path(sessionID)
	if err != nil {
		return conversation.Session{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return conversation.Session{}, conversation.ErrSessionNotFound
	}
	if err != nil {
		return conversation.Session{}, fmt.Errorf("read session: %w", err)
	}

	var result conversation.Session
	if err := json.Unmarshal(data, &result); err != nil {
		return conversation.Session{}, fmt.Errorf("decode session: %w", err)
	}
	if result.ID != strings.TrimSpace(sessionID) {
		return conversation.Session{}, fmt.Errorf("session identity mismatch")
	}
	if result.Messages == nil {
		result.Messages = make([]*schema.AgenticMessage, 0)
	}
	return result, nil
}

// Save atomically replaces the completed session snapshot.
func (s *FileStore) Save(ctx context.Context, value conversation.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.path(value.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session: %w", err)
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}

	tmp, err := os.CreateTemp(s.dir, ".session-*.tmp")
	if err != nil {
		return fmt.Errorf("create session temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("secure session temporary file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write session temporary file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync session temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close session temporary file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("commit session snapshot: %w", err)
	}
	committed = true
	return nil
}

func (s *FileStore) path(sessionID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", errors.New("session ID must not be empty")
	}
	digest := sha256.Sum256([]byte(sessionID))
	return filepath.Join(s.dir, hex.EncodeToString(digest[:])+".json"), nil
}
