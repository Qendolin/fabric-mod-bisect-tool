package mods

import (
	"sort"
	"sync"
)

// StateManager provides a thread-safe way to manage the state of mods.
type StateManager struct {
	mu sync.RWMutex
	// The canonical source of all mod data.
	allMods map[string]*Mod
	// Stores the runtime status for each mod, mapping mod ID to its status.
	modStatuses map[string]*ModStatus

	changeListeners []chan<- struct{}
}

// NewStateManager creates a new mod state manager.
// It initializes the mod statuses based on the initially loaded mod data.
func NewStateManager(allMods map[string]*Mod) *StateManager {
	modStatuses := make(map[string]*ModStatus, len(allMods))
	for id, mod := range allMods {
		modStatuses[id] = &ModStatus{
			ID:            id,
			Mod:           mod,
			ForceEnabled:  false,             // Default to not force-enabled
			ForceDisabled: false,             // Default to not force-disabled
			ManuallyGood:  mod.ConfirmedGood, // Initial state from loaded metadata
		}
	}
	return &StateManager{
		allMods:         allMods,
		modStatuses:     modStatuses,
		changeListeners: make([]chan<- struct{}, 0),
	}
}

// AddChangeListener registers a channel that will be signaled on state changes.
func (sm *StateManager) AddChangeListener(listener chan<- struct{}) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.changeListeners = append(sm.changeListeners, listener)
}

// notifyListeners signals all registered listeners that a state change occurred.
func (sm *StateManager) notifyListeners() {
	for _, listener := range sm.changeListeners {
		// Non-blocking send
		select {
		case listener <- struct{}{}:
		default:
			// If the channel is full, do not block. This ensures UI responsiveness.
		}
	}
}

// SetForceEnabled updates the force-enabled state of a mod.
func (sm *StateManager) SetForceEnabled(modID string, enabled bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if status, ok := sm.modStatuses[modID]; ok {
		status.ForceEnabled = enabled
		// If force-enabled, it cannot also be force-disabled.
		if enabled {
			status.ForceDisabled = false
		}
		sm.notifyListeners()
	}
}

// SetForceDisabled updates the force-disabled state of a mod.
func (sm *StateManager) SetForceDisabled(modID string, disabled bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if status, ok := sm.modStatuses[modID]; ok {
		status.ForceDisabled = disabled
		// If force-disabled, it cannot also be force-enabled.
		if disabled {
			status.ForceEnabled = false
		}
		sm.notifyListeners()
	}
}

// SetManuallyGood updates the "manually confirmed good" state of a mod.
func (sm *StateManager) SetManuallyGood(modID string, isGood bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if status, ok := sm.modStatuses[modID]; ok {
		status.ManuallyGood = isGood
		sm.notifyListeners()
	}
}

// GetModStatus returns the current ModStatus for a given modID.
// Returns nil and false if the modID is not found.
func (sm *StateManager) GetModStatus(modID string) (*ModStatus, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	status, ok := sm.modStatuses[modID]
	return status, ok
}

// GetModStatusesSnapshot returns a consistent snapshot of the current mod statuses.
// It returns a copy of the map, with copies of ModStatus structs to ensure immutability
// for external consumers and prevent race conditions.
func (sm *StateManager) GetModStatusesSnapshot() map[string]ModStatus {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	snapshot := make(map[string]ModStatus, len(sm.modStatuses))
	for id, status := range sm.modStatuses {
		// Create a copy of the ModStatus struct. The 'Mod' pointer inside
		// remains the same, pointing to the immutable Mod data.
		snapStatus := *status
		snapshot[id] = snapStatus
	}
	return snapshot
}

// GetAllModIDs returns a sorted slice of all mod IDs known to the manager.
func (sm *StateManager) GetAllModIDs() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	ids := make([]string, 0, len(sm.allMods))
	for id := range sm.allMods {
		ids = append(ids, id)
	}
	sort.Strings(ids) // Ensure deterministic order
	return ids
}
