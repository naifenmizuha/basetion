package conversation

import "sync"

// SessionLocker serializes turns sharing a business session ID while allowing
// unrelated sessions to execute concurrently.
type SessionLocker struct {
	mu      sync.Mutex
	entries map[string]*lockEntry
}

type lockEntry struct {
	mu   sync.Mutex
	refs int
}

// NewSessionLocker creates a session-scoped lock registry.
func NewSessionLocker() *SessionLocker {
	return &SessionLocker{entries: make(map[string]*lockEntry)}
}

// Lock acquires the lock for sessionID and returns its idempotent unlock
// function.
func (l *SessionLocker) Lock(sessionID string) func() {
	l.mu.Lock()
	entry := l.entries[sessionID]
	if entry == nil {
		entry = &lockEntry{}
		l.entries[sessionID] = entry
	}
	entry.refs++
	l.mu.Unlock()

	entry.mu.Lock()
	var once sync.Once
	return func() {
		once.Do(func() {
			entry.mu.Unlock()
			l.mu.Lock()
			entry.refs--
			if entry.refs == 0 {
				delete(l.entries, sessionID)
			}
			l.mu.Unlock()
		})
	}
}
