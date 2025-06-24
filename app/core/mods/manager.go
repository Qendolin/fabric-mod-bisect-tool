package mods

import (
	"sort"
)

// StateManager provides a way to manage the state of mods. It uses a synchronous
// callback model for state change notifications.
type StateManager struct {
	// The canonical source of all static mod data.
	allMods            map[string]*Mod
	potentialProviders PotentialProvidersMap

	// Stores the runtime status for each mod, mapping mod ID to its status.
	modStatuses map[string]*ModStatus

	// OnStateChanged is a callback function that is executed whenever the
	// state of any mod is modified.
	OnStateChanged func()
}

// NewStateManager creates a new mod state manager.
// It initializes the mod statuses based on the initially loaded mod data.
func NewStateManager(allMods map[string]*Mod, potentialProviders PotentialProvidersMap) *StateManager {
	modStatuses := make(map[string]*ModStatus, len(allMods))
	for id, mod := range allMods {
		modStatuses[id] = &ModStatus{
			ID:            id,
			Mod:           mod,
			ForceEnabled:  false,
			ForceDisabled: false,
			ManuallyGood:  mod.ConfirmedGood,
		}
	}
	return &StateManager{
		allMods:            allMods,
		modStatuses:        modStatuses,
		potentialProviders: potentialProviders,
	}
}

// notifyListeners calls the registered callback if it exists.
func (sm *StateManager) notifyListeners() {
	if sm.OnStateChanged != nil {
		sm.OnStateChanged()
	}
}

// SetForceEnabled updates the force-enabled state of a mod.
func (sm *StateManager) SetForceEnabled(modID string, enabled bool) {
	if status, ok := sm.modStatuses[modID]; ok {
		// No change, no notification needed.
		if status.ForceEnabled == enabled {
			return
		}
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
	if status, ok := sm.modStatuses[modID]; ok {
		if status.ForceDisabled == disabled {
			return
		}
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
	if status, ok := sm.modStatuses[modID]; ok {
		if status.ManuallyGood == isGood {
			return
		}
		status.ManuallyGood = isGood
		sm.notifyListeners()
	}
}

// GetModStatus returns the current ModStatus for a given modID.
// Returns nil and false if the modID is not found.
func (sm *StateManager) GetModStatus(modID string) (*ModStatus, bool) {
	status, ok := sm.modStatuses[modID]
	return status, ok
}

// GetModStatusesSnapshot returns a consistent snapshot of the current mod statuses.
func (sm *StateManager) GetModStatusesSnapshot() map[string]ModStatus {
	snapshot := make(map[string]ModStatus, len(sm.modStatuses))
	for id, status := range sm.modStatuses {
		snapStatus := *status
		snapshot[id] = snapStatus
	}
	return snapshot
}

// GetAllModIDs returns a sorted slice of all mod IDs known to the manager.
func (sm *StateManager) GetAllModIDs() []string {
	ids := make([]string, 0, len(sm.allMods))
	for id := range sm.allMods {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// GetAllMods returns the map of all loaded mods.
func (sm *StateManager) GetAllMods() map[string]*Mod {
	return sm.allMods
}

// GetPotentialProviders returns the map of potential dependency providers.
func (sm *StateManager) GetPotentialProviders() PotentialProvidersMap {
	return sm.potentialProviders
}
