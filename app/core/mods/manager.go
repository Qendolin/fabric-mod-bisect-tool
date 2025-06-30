package mods

import (
	"sort"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/sets"
)

// StateManager provides a way to manage the state of mods. It uses a synchronous
// callback model for state change notifications.
type StateManager struct {
	// The canonical source of all static mod data.
	allMods            map[string]*Mod
	potentialProviders PotentialProvidersMap

	// Stores the runtime status for each mod, mapping mod ID to its status.
	modStatuses map[string]*ModStatus

	// Internal dependency resolver.
	resolver *DependencyResolver

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
		}
	}
	return &StateManager{
		allMods:            allMods,
		modStatuses:        modStatuses,
		potentialProviders: potentialProviders,
		resolver:           NewDependencyResolver(allMods, potentialProviders),
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

// SetOmitted updates the "ignored in search" state of a mod.
func (sm *StateManager) SetOmitted(modID string, isOmitted bool) {
	if status, ok := sm.modStatuses[modID]; ok {
		if status.Omitted == isOmitted {
			return
		}
		status.Omitted = isOmitted
		sm.notifyListeners()
	}
}

// SetForceEnabledBatch updates the force-enabled state for multiple mods at once.
// It sends only a single notification after all changes are made.
func (sm *StateManager) SetForceEnabledBatch(modIDs []string, enabled bool) {
	var changed bool
	for _, modID := range modIDs {
		if status, ok := sm.modStatuses[modID]; ok {
			if status.ForceEnabled != enabled {
				status.ForceEnabled = enabled
				if enabled {
					status.ForceDisabled = false
				}
				changed = true
			}
		}
	}
	if changed {
		sm.notifyListeners()
	}
}

// SetForceDisabledBatch updates the force-disabled state for multiple mods at once.
// It sends only a single notification after all changes are made.
func (sm *StateManager) SetForceDisabledBatch(modIDs []string, disabled bool) {
	var changed bool
	for _, modID := range modIDs {
		if status, ok := sm.modStatuses[modID]; ok {
			if status.ForceDisabled != disabled {
				status.ForceDisabled = disabled
				if disabled {
					status.ForceEnabled = false
				}
				changed = true
			}
		}
	}
	if changed {
		sm.notifyListeners()
	}
}

// SetOmittedBatch updates the "ignored in search" state for multiple mods at once.
// It sends only a single notification after all changes are made.
func (sm *StateManager) SetOmittedBatch(modIDs []string, omitted bool) {
	var changed bool
	for _, modID := range modIDs {
		if status, ok := sm.modStatuses[modID]; ok {
			if status.Omitted != omitted {
				status.Omitted = omitted
				changed = true
			}
		}
	}
	if changed {
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

// ResolveEffectiveSet calculates the set of active top-level mods based on the
// given target set and the current mod statuses managed by the StateManager.
func (sm *StateManager) ResolveEffectiveSet(targetSet sets.Set) (sets.Set, []ResolutionInfo) {
	return sm.resolver.ResolveEffectiveSet(targetSet, sm.GetModStatusesSnapshot())
}

// FindTransitiveDependersOf delegates the call to its internal dependency resolver.
func (sm *StateManager) FindTransitiveDependersOf(targets sets.Set) sets.Set {
	return sm.resolver.FindTransitiveDependersOf(targets)
}
