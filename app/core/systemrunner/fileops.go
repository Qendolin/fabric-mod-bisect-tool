package systemrunner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

const disabledExtension = ".disabled"

// ModActivator manages the physical file state of mods (enabled/disabled).
type ModActivator struct {
	modsDir string
	allMods map[string]*mods.Mod
	// currentActivations tracks the last known physical active state of each mod file.
	// This map stores modID -> true if the file is currently active (.jar), false if disabled (.jar.disabled).
	// This state is updated *after* a successful rename operation.
	currentActivations map[string]bool
}

// NewModActivator creates a new activator.
// It initializes the internal tracking based on the mod's initial active state.
func NewModActivator(modsDir string, allMods map[string]*mods.Mod) *ModActivator {
	activations := make(map[string]bool, len(allMods))
	for id, mod := range allMods {
		activations[id] = mod.IsInitiallyActive
	}

	return &ModActivator{
		modsDir:            modsDir,
		allMods:            allMods,
		currentActivations: activations,
	}
}

// BatchStateChange represents a single file rename operation.
type BatchStateChange struct {
	ModID    string
	OldPath  string
	NewPath  string
	Activate bool // True if the mod became active, false if it became disabled.
}

// Apply calculates and executes the necessary file renames to achieve the effectiveSet state.
// It performs a batch of renames and returns the list of changes made.
func (ma *ModActivator) Apply(effectiveSet map[string]struct{}) ([]BatchStateChange, error) {
	changes := ma.calculateChanges(effectiveSet)
	if len(changes) == 0 {
		return nil, nil
	}

	var enabledMods []string
	var disabledMods []string
	for _, change := range changes {
		if change.Activate {
			enabledMods = append(enabledMods, change.ModID)
		} else {
			disabledMods = append(disabledMods, change.ModID)
		}
	}
	sort.Strings(enabledMods)
	sort.Strings(disabledMods)

	logging.Infof("Activator: Applying changes to %d mod files. Enabling: %v, Disabling: %v", len(changes), enabledMods, disabledMods)

	appliedChanges := make([]BatchStateChange, 0, len(changes))
	for _, change := range changes {
		if err := os.Rename(change.OldPath, change.NewPath); err != nil {
			// On failure, revert all changes made so far in this batch and report the error.
			ma.Revert(appliedChanges)
			return nil, fmt.Errorf("failed to rename %s to %s for mod %s: %w", change.OldPath, change.NewPath, change.ModID, err)
		}
		ma.currentActivations[change.ModID] = change.Activate
		appliedChanges = append(appliedChanges, change)
	}

	return appliedChanges, nil
}

// Revert applies a set of changes in reverse order to restore a previous state.
// This is used for cleanup or undo operations.
func (ma *ModActivator) Revert(changes []BatchStateChange) {
	if len(changes) == 0 {
		return
	}

	var reEnabledMods []string
	var reDisabledMods []string
	// Iterate in reverse for revert
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		if change.Activate { // If it was activated, reverting means disabling it
			reDisabledMods = append(reDisabledMods, change.ModID)
		} else { // If it was disabled, reverting means enabling it
			reEnabledMods = append(reEnabledMods, change.ModID)
		}
	}
	sort.Strings(reEnabledMods)
	sort.Strings(reDisabledMods)

	logging.Infof("Activator: Reverting changes for %d mod files. Re-enabling: %v, Re-disabling: %v", len(changes), reEnabledMods, reDisabledMods)

	// Revert changes in reverse order of their original application.
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		// The 'newPath' of the original change becomes the 'oldPath' for the revert operation.
		// The 'oldPath' of the original change becomes the 'newPath' for the revert operation.
		if err := os.Rename(change.NewPath, change.OldPath); err != nil {
			logging.Errorf("Activator: Failed to revert mod %s (%s -> %s): %v", change.ModID, filepath.Base(change.NewPath), filepath.Base(change.OldPath), err)
			// Continue attempting to revert other files even if one fails.
		} else {
			// Update currentActivations based on the reverted state.
			ma.currentActivations[change.ModID] = !change.Activate
		}
	}
}

// calculateChanges determines which files need to be renamed based on the desired effective set
// and the current physical state of mod files on disk as tracked by the activator.
func (ma *ModActivator) calculateChanges(effectiveSet map[string]struct{}) []BatchStateChange {
	var changes []BatchStateChange
	for id, mod := range ma.allMods {
		isCurrentlyActive := ma.currentActivations[id] // The physical state as tracked by activator
		_, shouldBeActive := effectiveSet[id]          // The desired logical state from resolver

		if isCurrentlyActive == shouldBeActive {
			continue // No change needed for this mod; its physical state matches the desired logical state.
		}

		// Determine the *current physical path* on disk based on `isCurrentlyActive` and `mod.BaseFilename`.
		// `mod.BaseFilename` is the filename without the .jar or .disabled suffix (e.g., "mod-A-1.0").
		var currentPhysicalPath string
		if isCurrentlyActive {
			currentPhysicalPath = filepath.Join(ma.modsDir, mod.BaseFilename+".jar")
		} else {
			currentPhysicalPath = filepath.Join(ma.modsDir, mod.BaseFilename+".jar"+disabledExtension)
		}

		// Determine the *target physical path* on disk based on `shouldBeActive`.
		var newPhysicalPath string
		if shouldBeActive { // Desired state is active
			newPhysicalPath = filepath.Join(ma.modsDir, mod.BaseFilename+".jar")
		} else { // Desired state is disabled
			newPhysicalPath = filepath.Join(ma.modsDir, mod.BaseFilename+".jar"+disabledExtension)
		}

		// Although `isCurrentlyActive == shouldBeActive` should catch most redundant operations,
		// this check adds a layer of robustness.
		if currentPhysicalPath == newPhysicalPath {
			logging.Warnf("Activator: Calculated redundant rename for mod '%s'. Current path '%s' is already target path '%s'. Skipping.", id, currentPhysicalPath, newPhysicalPath)
			continue
		}

		changes = append(changes, BatchStateChange{
			ModID:    id,
			OldPath:  currentPhysicalPath,
			NewPath:  newPhysicalPath,
			Activate: shouldBeActive,
		})
	}
	return changes
}
