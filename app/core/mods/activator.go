package mods

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

const disabledExtension = ".disabled"

// Activator manages the physical file state of mods (enabled/disabled).
type Activator struct {
	modsDir string
	allMods map[string]*Mod
	// currentActivations tracks the last known physical active state of each mod file.
	// This map stores modID -> true if the file is currently active (.jar), false if disabled (.jar.disabled).
	// This state is updated *after* a successful rename operation.
	currentActivations map[string]bool
}

// NewModActivator creates a new activator.
// It initializes the internal tracking based on the mod's initial active state.
func NewModActivator(modsDir string, allMods map[string]*Mod) *Activator {
	activations := make(map[string]bool, len(allMods))
	for id, mod := range allMods {
		activations[id] = mod.IsInitiallyActive
	}

	return &Activator{
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
func (a *Activator) Apply(effectiveSet sets.Set, statuses map[string]ModStatus) ([]BatchStateChange, error) {
	changes := a.calculateChanges(effectiveSet, statuses)
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
	var missingFileErrors []*FileMissingError

	for _, change := range changes {
		if _, err := os.Stat(change.OldPath); os.IsNotExist(err) {
			// Check if the file is already in the target state (e.g., already disabled).
			if _, err := os.Stat(change.NewPath); !os.IsNotExist(err) {
				a.currentActivations[change.ModID] = change.Activate
				continue // File already in correct state. Not an error.
			}
			// The file is truly missing from the filesystem.
			logging.Warnf("Activator: Source file for mod '%s' is missing: %s", change.ModID, change.OldPath)
			missingFileErrors = append(missingFileErrors, &FileMissingError{ModID: change.ModID, FilePath: change.OldPath})
			continue // Continue processing other files.
		}

		if err := os.Rename(change.OldPath, change.NewPath); err != nil {
			// A hard I/O error (e.g., permissions). This is fatal for this operation.
			logging.Errorf("Activator: A hard I/O error occurred: %v", err)
			a.Revert(appliedChanges) // Revert what we've done so far.
			return nil, fmt.Errorf("failed to rename '%s' for mod '%s': %w", filepath.Base(change.OldPath), change.ModID, err)
		}

		a.currentActivations[change.ModID] = change.Activate
		appliedChanges = append(appliedChanges, change)
	}

	if len(missingFileErrors) > 0 {
		a.Revert(appliedChanges)
		return nil, &MissingFilesError{Errors: missingFileErrors}
	}

	return appliedChanges, nil
}

// Revert applies a set of changes in reverse order to restore a previous state.
// This is used for cleanup or undo operations.
func (a *Activator) Revert(changes []BatchStateChange) {
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
			a.currentActivations[change.ModID] = !change.Activate
		}
	}
}

func (a *Activator) EnableAll(statuses map[string]ModStatus) error {
	logging.Info("Activator: Enabling all non-missing mods for a clean initial state.")
	targetSet := make(sets.Set, len(a.allMods))
	for id, status := range statuses {
		if !status.IsMissing {
			targetSet[id] = struct{}{}
		}
	}

	_, err := a.Apply(targetSet, statuses)
	if err != nil {
		return fmt.Errorf("failed during initial enabling of all mods: %w", err)
	}

	return nil
}

// calculateChanges determines which files need to be renamed based on the desired effective set
// and the current physical state of mod files on disk as tracked by the activator.
func (a *Activator) calculateChanges(effectiveSet sets.Set, statuses map[string]ModStatus) []BatchStateChange {
	var changes []BatchStateChange
	for id, mod := range a.allMods {
		status, ok := statuses[id]
		if !ok || status.IsMissing {
			continue // Skip missing mods entirely.
		}

		isCurrentlyActive := a.currentActivations[id] // The physical state as tracked by activator
		_, shouldBeActive := effectiveSet[id]         // The desired logical state from resolver

		if isCurrentlyActive == shouldBeActive {
			logging.Debugf("Activator: Mod '%s' physical state matches desired state (active: %t). No change needed.", id, isCurrentlyActive)
			continue
		}

		// Determine the *current physical path* on disk based on `isCurrentlyActive` and `mod.BaseFilename`.
		// `mod.BaseFilename` is the filename without the .jar or .disabled suffix (e.g., "mod-A-1.0").
		var currentPhysicalPath string
		if isCurrentlyActive {
			currentPhysicalPath = filepath.Join(a.modsDir, mod.BaseFilename+".jar")
		} else {
			currentPhysicalPath = filepath.Join(a.modsDir, mod.BaseFilename+".jar"+disabledExtension)
		}

		// Determine the *target physical path* on disk based on `shouldBeActive`.
		var newPhysicalPath string
		if shouldBeActive { // Desired state is active
			newPhysicalPath = filepath.Join(a.modsDir, mod.BaseFilename+".jar")
		} else { // Desired state is disabled
			newPhysicalPath = filepath.Join(a.modsDir, mod.BaseFilename+".jar"+disabledExtension)
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
