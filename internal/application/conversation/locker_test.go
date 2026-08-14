package conversation

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionLockerSerializesSameSession(t *testing.T) {
	t.Parallel()

	locker := NewSessionLocker()
	var active atomic.Int32
	var maxActive atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := locker.Lock("same")
			defer unlock()
			current := active.Add(1)
			for {
				old := maxActive.Load()
				if current <= old || maxActive.CompareAndSwap(old, current) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			active.Add(-1)
		}()
	}
	wg.Wait()
	if got := maxActive.Load(); got != 1 {
		t.Fatalf("same-session concurrency = %d, want 1", got)
	}
}

func TestSessionLockerAllowsDifferentSessions(t *testing.T) {
	t.Parallel()

	locker := NewSessionLocker()
	unlocked := make(chan struct{})
	firstUnlock := locker.Lock("one")
	go func() {
		secondUnlock := locker.Lock("two")
		secondUnlock()
		close(unlocked)
	}()

	select {
	case <-unlocked:
	case <-time.After(time.Second):
		t.Fatal("different session was unexpectedly blocked")
	}
	firstUnlock()
}
