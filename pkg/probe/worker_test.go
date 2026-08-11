package probe

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWorkerSerializesAndCoalescesProbes verifies that the Worker runs at most
// one probe at a time and that intermediate queued requests are coalesced into
// the most recent one, which always completes.
func TestWorkerSerializesAndCoalescesProbes(t *testing.T) {
	w := NewWorker()
	dir := t.TempDir()

	var mu sync.Mutex
	var inFlight, maxInFlight int
	var last atomic.Int32
	const n = 50

	for i := 0; i < n; i++ {
		i := i
		w.Request(dir, func(ProbeResult) {
			last.Store(int32(i))
			mu.Lock()
			inFlight++
			if inFlight > maxInFlight {
				maxInFlight = inFlight
			}
			mu.Unlock()

			time.Sleep(time.Millisecond) // let later requests pile up

			mu.Lock()
			inFlight--
			mu.Unlock()
		})
	}

	deadline := time.After(5 * time.Second)
	for last.Load() != int32(n-1) {
		select {
		case <-deadline:
			t.Fatalf("the most recent request (index %d) never completed", n-1)
		case <-time.After(time.Millisecond):
		}
	}
	if maxInFlight > 1 {
		t.Fatalf("expected at most 1 probe in flight, got %d", maxInFlight)
	}
}

// TestWorkerRejectsInvalidDirectories verifies that requests for paths that are
// not directories are dropped before being queued.
func TestWorkerRejectsInvalidDirectories(t *testing.T) {
	w := NewWorker()

	var mu sync.Mutex
	invalidCalled := false
	w.Request(filepath.Join(t.TempDir(), "does-not-exist"), func(ProbeResult) {
		mu.Lock()
		invalidCalled = true
		mu.Unlock()
	})

	done := make(chan struct{})
	w.Request(t.TempDir(), func(ProbeResult) {
		mu.Lock()
		defer mu.Unlock()
		if invalidCalled {
			t.Error("callback invoked for a path that is not a valid directory")
		}
		close(done)
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the valid request never completed")
	}
}
