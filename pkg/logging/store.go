package logging

import "sync"

// LogStore holds all log entries in memory for the UI. It is thread-safe.
type LogStore struct {
	mu      sync.RWMutex
	entries []LogEntry
}

// newLogStore creates a new LogStore.
func newLogStore() *LogStore {
	return &LogStore{
		entries: make([]LogEntry, 0, 1024),
	}
}

// Add appends a new entry to the store.
func (s *LogStore) Add(entry LogEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
}

// GetAll returns a copy of all log entries.
func (s *LogStore) GetAll() []LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Return a copy to prevent race conditions on the slice itself
	entriesCopy := make([]LogEntry, len(s.entries))
	copy(entriesCopy, s.entries)
	return entriesCopy
}
