package app

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// BisectionStep stores the state for one step of bisection, for undo.
type BisectionStep struct {
	SearchSpace           []string
	GroupAOriginal        []string
	GroupBOriginal        []string
	IssuePresentInA       bool // Result of Group A test for this step
	ForceEnabledSnapshot  map[string]bool
	ForceDisabledSnapshot map[string]bool
}

// Bisector manages the bisection process.
type Bisector struct {
	ModsDir         string
	AllMods         map[string]*Mod
	AllModIDsSorted []string
	ForceEnabled    map[string]bool
	ForceDisabled   map[string]bool

	CurrentSearchSpace []string
	History            []BisectionStep

	CurrentGroupAOriginal  []string
	CurrentGroupBOriginal  []string
	CurrentGroupAEffective map[string]bool
	CurrentGroupBEffective map[string]bool

	testingPhase    int // 0: prepareA, 1: testingA, 2: testingB
	IterationCount  int
	MaxIterations   int
	InitialModCount int

	LastKnownBadEffectiveSet map[string]bool
	lastTestCausedIssue      bool // True if the most recent user-reported test failed

	resolver *DependencyResolver
	strategy BisectionStrategy // The bisection strategy to use
}

// Constants for testing phases.
const (
	PhasePrepareA = 0
	PhaseTestingA = 1
	PhaseTestingB = 2
)

// NewBisector initializes a new Bisector instance.
func NewBisector(modsDir string, allMods map[string]*Mod, allModIDsSorted []string,
	potentialProviders PotentialProvidersMap, strategy BisectionStrategy) *Bisector {
	b := &Bisector{
		ModsDir:                  modsDir,
		AllMods:                  allMods,
		AllModIDsSorted:          allModIDsSorted,
		ForceEnabled:             make(map[string]bool),
		ForceDisabled:            make(map[string]bool),
		History:                  make([]BisectionStep, 0),
		LastKnownBadEffectiveSet: make(map[string]bool),
		testingPhase:             PhasePrepareA,
		strategy:                 strategy,
	}
	b.resolver = NewDependencyResolver(allMods, potentialProviders, b.ForceEnabled, b.ForceDisabled)

	for _, modID := range b.AllModIDsSorted {
		mod, ok := b.AllMods[modID]
		if !ok {
			continue
		}
		if mod.IsCurrentlyActive && !isImplicitMod(modID) {
			b.CurrentSearchSpace = append(b.CurrentSearchSpace, modID)
		}
	}
	sort.Strings(b.CurrentSearchSpace)
	b.InitialModCount = len(b.CurrentSearchSpace)

	if b.InitialModCount > 0 {
		b.MaxIterations = int(math.Ceil(math.Log2(float64(b.InitialModCount))))
		if b.MaxIterations < 1 {
			b.MaxIterations = 1
		}
	}

	log.Printf("%sBisector initialized with %s. Initial search space: %d mods. Max iterations: ~%d.",
		LogInfoPrefix, BisectionStrategyTypeStrings[GetStrategyType(strategy)], b.InitialModCount, b.MaxIterations)
	return b
}

// PrepareNextTestOrConclude advances the bisection.
// Returns: done (bool), question (string), status (string).
func (b *Bisector) PrepareNextTestOrConclude() (bool, string, string) {
	if b.testingPhase != PhasePrepareA {
		log.Printf("%sPrepareNextTestOrConclude called in unexpected phase %d. Resetting to prepare A.", LogErrorPrefix, b.testingPhase)
		b.testingPhase = PhasePrepareA
	}

	if len(b.CurrentSearchSpace) == 0 {
		return true, "", b.determineConclusionForEmptySearchSpace()
	}

	if len(b.CurrentSearchSpace) == 1 { // Single mod remaining
		modID := b.CurrentSearchSpace[0]
		modName := modID
		if mod, ok := b.AllMods[modID]; ok {
			modName = mod.FriendlyName()
		}
		conclusion := fmt.Sprintf("Problematic mod identified: %s (%s). It's the only one left in the search space.", modName, modID)
		return true, "", b.formatConclusionMessage(conclusion)
	}

	b.IterationCount++
	log.Printf("%sStarting Iteration %d / ~%d. Search Space: %d mods.", LogInfoPrefix, b.IterationCount, b.MaxIterations, len(b.CurrentSearchSpace))

	b.splitSearchSpaceAndRecordHistory()

	b.CurrentGroupAEffective = b.calculateEffectiveGroup(b.CurrentGroupAOriginal, "A")
	b.CurrentGroupBEffective = b.calculateEffectiveGroup(b.CurrentGroupBOriginal, "B")

	b.applyModSet(b.CurrentGroupAEffective)
	b.testingPhase = PhaseTestingA

	status := fmt.Sprintf("Iteration %d. Search Space: %d mods.", b.IterationCount, len(b.CurrentSearchSpace))
	question := b.formatQuestion("A", b.CurrentGroupAOriginal, b.CurrentGroupAEffective)
	return false, question, status
}

func (b *Bisector) splitSearchSpaceAndRecordHistory() {
	currentHistoryEntry := BisectionStep{
		SearchSpace:           slices.Clone(b.CurrentSearchSpace),
		ForceEnabledSnapshot:  cloneMap(b.ForceEnabled),
		ForceDisabledSnapshot: cloneMap(b.ForceDisabled),
	}
	// Add to history before modifying CurrentGroupA/BOriginal for the new step
	b.History = append(b.History, currentHistoryEntry)
	historyIdx := len(b.History) - 1

	mid := len(b.CurrentSearchSpace) / 2
	if len(b.CurrentSearchSpace) == 1 {
		mid = 1
	}
	b.CurrentGroupAOriginal = slices.Clone(b.CurrentSearchSpace[:mid])
	b.CurrentGroupBOriginal = []string{}
	if mid < len(b.CurrentSearchSpace) {
		b.CurrentGroupBOriginal = slices.Clone(b.CurrentSearchSpace[mid:])
	}

	b.History[historyIdx].GroupAOriginal = b.CurrentGroupAOriginal
	b.History[historyIdx].GroupBOriginal = b.CurrentGroupBOriginal
}

func (b *Bisector) determineConclusionForEmptySearchSpace() string {
	msg := "Search space empty."
	if b.lastTestCausedIssue {
		msg += " Last test showed the issue. Problem likely in common dependencies, forced-enabled mods, or game core."
	} else {
		msg += " Last test was success. Bisection inconclusive or issue resolved."
	}
	return b.formatConclusionMessage(msg)
}

// ProcessUserFeedback updates state based on user's answer using the chosen strategy.
// Returns: done (bool), nextQuestion (string), status (string).
func (b *Bisector) ProcessUserFeedback(issueOccurred bool) (bool, string, string) {
	if len(b.History) == 0 {
		log.Printf("%sProcessUserFeedback: No history. Bisection not started or in error state.", LogErrorPrefix)
		return true, "", "Error: Bisection state error (no history)."
	}
	currentIterationHistory := &b.History[len(b.History)-1]
	b.lastTestCausedIssue = issueOccurred // Tracks the direct outcome of the user's test

	var outcome NextActionOutcome
	var done bool
	var nextQuestion, statusMsg string

	switch b.testingPhase {
	case PhaseTestingA:
		currentIterationHistory.IssuePresentInA = issueOccurred
		b.recordTestOutcomeDetails(issueOccurred, "A", b.CurrentGroupAEffective)
		outcome = b.strategy.DetermineNextActionAfterA(issueOccurred, b)

		statusMsg = outcome.Message
		if outcome.Conclude {
			done = true
			statusMsg = b.formatConclusionMessage(outcome.Message) // Strategy might provide a base conclusion
		} else if outcome.TestB {
			// Ensure Group B exists before attempting to test it. Strategy should factor this.
			if len(b.CurrentGroupBOriginal) == 0 {
				log.Printf("%sStrategy indicated testing Group B, but it's empty. Preparing new iteration.", LogWarningPrefix)
				b.testingPhase = PhasePrepareA
				done, nextQuestion, statusMsg = b.PrepareNextTestOrConclude()
			} else {
				b.applyModSet(b.CurrentGroupBEffective) // Assumes CurrentGroupBEffective is up-to-date
				b.testingPhase = PhaseTestingB
				nextQuestion = outcome.NextQuestionForB
				// statusMsg already set from outcome.Message
			}
		} else { // Proceed to next iteration (search space was updated by strategy)
			b.testingPhase = PhasePrepareA
			done, nextQuestion, statusMsg = b.PrepareNextTestOrConclude()
		}

	case PhaseTestingB:
		b.recordTestOutcomeDetails(issueOccurred, "B", b.CurrentGroupBEffective)
		// issueWasPresentInA is from currentIterationHistory
		outcome = b.strategy.DetermineNextActionAfterB(issueOccurred, currentIterationHistory.IssuePresentInA, b)

		if outcome.Conclude {
			done = true
			statusMsg = b.formatConclusionMessage(outcome.Message)
		} else { // Proceed to next iteration
			b.testingPhase = PhasePrepareA
			done, nextQuestion, statusMsg = b.PrepareNextTestOrConclude()
		}
	default:
		log.Printf("%sProcessUserFeedback called in unexpected phase: %d", LogErrorPrefix, b.testingPhase)
		return true, "", "Error: Unexpected bisection phase."
	}
	return done, nextQuestion, statusMsg
}

func (b *Bisector) formatQuestion(groupDesignator string, originalGroup []string, effectiveGroup map[string]bool) string {
	return fmt.Sprintf("Testing Group %s (%d mods, %d effective).\nLaunch Minecraft now.\n\nDoes the issue STILL OCCUR?",
		groupDesignator, len(originalGroup), len(effectiveGroup))
}

func (b *Bisector) formatConclusionMessage(baseMessage string) string {
	conclusion := baseMessage
	if baseMessage == "" { // Default if strategy provides no specific conclusion
		conclusion = "Bisection finished."
	}

	if len(b.LastKnownBadEffectiveSet) > 0 {
		var badModNames []string
		for modID := range b.LastKnownBadEffectiveSet {
			name := modID
			if mod, ok := b.AllMods[modID]; ok {
				name = mod.FriendlyName()
			}
			badModNames = append(badModNames, name)
		}
		sort.Strings(badModNames)
		conclusion += fmt.Sprintf("\nLast known failing set included %d mods: %s", len(badModNames), strings.Join(badModNames, ", "))
	} else if b.lastTestCausedIssue && !strings.Contains(conclusion, "Last test showed the issue") {
		// Append if not already part of strategy's message and last test was bad
		conclusion += "\nThe very last test performed still showed the issue."
	}
	log.Printf("%sBisection concluded: %s", LogInfoPrefix, conclusion)
	return conclusion
}

func (b *Bisector) recordTestOutcomeDetails(testGroupCausedIssue bool, groupTested string, effectiveSetTested map[string]bool) {
	if testGroupCausedIssue {
		b.LastKnownBadEffectiveSet = cloneMap(effectiveSetTested) // Clone to prevent modification
		log.Printf("%sIssue PRESENT with Group %s. Effective set (%d mods): %v",
			LogInfoPrefix, groupTested, len(effectiveSetTested), mapKeys(effectiveSetTested))
	} else {
		log.Printf("%sIssue GONE with Group %s.", LogInfoPrefix, groupTested)
	}
}

func (b *Bisector) calculateEffectiveGroup(originalGroup []string, name string) map[string]bool {
	effectiveSet, resolutionDetails := b.resolver.ResolveEffectiveSet(originalGroup)
	if len(resolutionDetails) > 0 {
		log.Printf("%sDependency Resolution Path for group %s %v:", LogInfoPrefix, name, originalGroup)
		for _, detail := range resolutionDetails {
			if detail.Reason == "Target" {
				continue
			}
			reasonStr := fmt.Sprintf("'%s' (%s)", detail.ModID, detail.Reason)
			if detail.Reason == "Dependency" {
				reasonStr += fmt.Sprintf(" for '%s' by '%s'", detail.SatisfiedDep, strings.Join(detail.NeededFor, ", "))
				if detail.SelectedProvider != nil {
					reasonStr += fmt.Sprintf(" (via provider %s v%s, direct: %t)", detail.SelectedProvider.TopLevelModID, detail.SelectedProvider.VersionOfProvidedItem, detail.SelectedProvider.IsDirectProvide)
				}
			}
			log.Printf("%s  - %s", LogInfoPrefix, reasonStr)
		}
	}
	return effectiveSet
}

func (b *Bisector) calculateIntersectionSearchSpace(iterationInitialSearchSpace []string, groupAEffective, groupBEffective map[string]bool) []string {
	candidatePool := make(map[string]bool)
	for _, modID := range iterationInitialSearchSpace { // Only consider mods from the original search space for this step
		candidatePool[modID] = true
	}
	// Also consider force-enabled mods, as they might be the cause if they are dependencies
	for modID := range b.ForceEnabled {
		candidatePool[modID] = true
	}

	var intersectedAndFiltered []string
	if groupAEffective != nil && groupBEffective != nil {
		for modID := range groupAEffective {
			if groupBEffective[modID] && candidatePool[modID] { // Mod is in both effective sets AND in the allowed pool
				intersectedAndFiltered = append(intersectedAndFiltered, modID)
			}
		}
	}
	sort.Strings(intersectedAndFiltered)
	log.Printf("%sIntersection result for search space: %v", LogInfoPrefix, intersectedAndFiltered)
	return intersectedAndFiltered
}

// In app/bisector.go

func (b *Bisector) applyModSet(effectiveModsToEnable map[string]bool) {
	log.Printf("%sApplying mod set. Target effective mods (%d): %v", LogInfoPrefix, len(effectiveModsToEnable), mapKeys(effectiveModsToEnable))
	var enabledMods, disabledMods, renameErrors []string

	for _, modID := range b.AllModIDsSorted {
		mod, ok := b.AllMods[modID]
		if !ok {
			continue
		}
		shouldBeActive := effectiveModsToEnable[modID]
		if b.ForceDisabled[modID] {
			shouldBeActive = false
		}

		var err error

		if shouldBeActive && !mod.IsCurrentlyActive {
			err = mod.Enable(b.ModsDir)
			if err == nil {
				enabledMods = append(enabledMods, mod.ModID())
			}
		} else if !shouldBeActive && mod.IsCurrentlyActive {
			err = mod.Disable(b.ModsDir)
			if err == nil {
				disabledMods = append(disabledMods, mod.ModID())
			}
		}

		if err != nil {
			log.Printf("%sError applying state for %s (target active: %t): %v", LogErrorPrefix, mod.ModID(), shouldBeActive, err)
			renameErrors = append(renameErrors, fmt.Sprintf("%s: %v", mod.ModID(), err))
		}
	}

	if len(enabledMods) > 0 {
		sort.Strings(enabledMods)
		log.Printf("%sEnabled %d mods: %v", LogInfoPrefix, len(enabledMods), enabledMods)
	}
	if len(disabledMods) > 0 {
		sort.Strings(disabledMods)
		log.Printf("%sDisabled %d mods: %v", LogInfoPrefix, len(disabledMods), disabledMods)
	}

	if len(enabledMods) == 0 && len(disabledMods) == 0 && len(renameErrors) == 0 {
		log.Printf("%sNo mod state changes needed for the current effective set.", LogInfoPrefix)
	}

	if len(renameErrors) > 0 {
		log.Printf("%sWARNING: Renaming errors encountered for %d mods: %s.", LogWarningPrefix, len(renameErrors), strings.Join(renameErrors, "; "))
		// Consider if UI should be more forcefully updated with this error status
	}
}

func (b *Bisector) UndoLastStep() (possible bool, message string) {
	if len(b.History) == 0 {
		return false, "Cannot undo: No history available."
	}

	b.History = b.History[:len(b.History)-1] // Pop last step
	if b.IterationCount > 0 {
		b.IterationCount--
	}
	b.testingPhase = PhasePrepareA // Always go back to preparing for a test

	if len(b.History) > 0 { // If there are previous steps, restore to the end state of that step
		lastValidStep := b.History[len(b.History)-1]
		b.CurrentSearchSpace = slices.Clone(lastValidStep.SearchSpace)
		b.ForceEnabled = cloneMap(lastValidStep.ForceEnabledSnapshot)
		b.ForceDisabled = cloneMap(lastValidStep.ForceDisabledSnapshot)
		// Reset ConfirmedGood status for all mods, as undo might change context
		for _, mod := range b.AllMods {
			mod.ConfirmedGood = false
		}
		log.Printf("%sUndo: Reverted to state before Iteration %d. Search space: %d.", LogInfoPrefix, b.IterationCount+1, len(b.CurrentSearchSpace))
	} else { // No history left, revert to initial state
		b.CurrentSearchSpace = []string{}
		for _, modID := range b.AllModIDsSorted { // Rebuild initial search space
			mod := b.AllMods[modID]
			if mod.IsCurrentlyActive && !isImplicitMod(modID) {
				b.CurrentSearchSpace = append(b.CurrentSearchSpace, modID)
			}
		}
		sort.Strings(b.CurrentSearchSpace)
		b.ForceEnabled = make(map[string]bool)
		b.ForceDisabled = make(map[string]bool)
		b.IterationCount = 0
		log.Printf("%sUndo: Reverted to initial state. Search space: %d.", LogInfoPrefix, len(b.CurrentSearchSpace))
	}
	// Recalculate and apply a modset appropriate for the current (PrepareA) phase.
	// This usually means applying what would be GroupA of the *next* step from this reverted state.
	b.recalculateAndApplyCurrentModset("Undo operation")
	return true, "Undo successful. Press 'S' to start next test from this state."
}

func (b *Bisector) ToggleForceEnable(modID string) {
	mod, ok := b.AllMods[modID]
	if !ok {
		return
	}
	modName := mod.FriendlyName()
	if b.ForceEnabled[modID] {
		delete(b.ForceEnabled, modID)
		log.Printf("%sMod '%s' is NO LONGER force-enabled.", LogInfoPrefix, modName)
	} else {
		b.ForceEnabled[modID] = true
		delete(b.ForceDisabled, modID)
		log.Printf("%sMod '%s' is NOW force-enabled.", LogInfoPrefix, modName)
	}
	b.recalculateAndApplyCurrentModset("ForceEnable toggle for " + modName)
}

func (b *Bisector) ToggleForceDisable(modID string) {
	mod, ok := b.AllMods[modID]
	if !ok {
		return
	}
	modName := mod.FriendlyName()
	if b.ForceDisabled[modID] {
		delete(b.ForceDisabled, modID)
		log.Printf("%sMod '%s' is NO LONGER force-disabled.", LogInfoPrefix, modName)
	} else {
		b.ForceDisabled[modID] = true
		delete(b.ForceEnabled, modID)
		log.Printf("%sMod '%s' is NOW force-disabled.", LogInfoPrefix, modName)
	}
	b.recalculateAndApplyCurrentModset("ForceDisable toggle for " + modName)
}

func (b *Bisector) recalculateAndApplyCurrentModset(reason string) {
	log.Printf("%sRecalculating mod set due to: %s.", LogInfoPrefix, reason)
	var effectiveGroupToReapply map[string]bool
	var currentOperationDesc string

	switch b.testingPhase {
	case PhaseTestingA: // Re-calculate and apply Group A's effective set
		b.CurrentGroupAEffective = b.calculateEffectiveGroup(b.CurrentGroupAOriginal, "A")
		effectiveGroupToReapply = b.CurrentGroupAEffective
		currentOperationDesc = "current Group A test"
	case PhaseTestingB:
		effectiveGroupToReapply = b.calculateEffectiveGroup(b.CurrentGroupBOriginal, "B")
		b.CurrentGroupBEffective = effectiveGroupToReapply
		currentOperationDesc = "current Group B test"
	case PhasePrepareA:
		// If in PrepareA, we might be before the first iteration or between iterations.
		// The goal is to set up for the *next* Group A test, or an initial state.
		if b.IterationCount == 0 && len(b.CurrentSearchSpace) > 0 {
			// Before first iteration, apply a neutral set based on current search space and forces.
			// This means enabling all mods in current search space + forced, minus forced-disabled.
			// This effectively reflects what the first Group A would be if it included everything.
			// However, it's safer to just calculate what the *next* group A would be.
			tempMid := len(b.CurrentSearchSpace) / 2
			if len(b.CurrentSearchSpace) == 1 {
				tempMid = 1
			}
			tempGroupAOrig := slices.Clone(b.CurrentSearchSpace[:tempMid]) // Tentative next Group A
			effectiveGroupToReapply = b.calculateEffectiveGroup(tempGroupAOrig, "Prospective A")
			currentOperationDesc = "prospective Group A for next iteration (or initial state)"

		} else if len(b.CurrentSearchSpace) > 0 {
			// Between iterations, calculate effective set for the next Group A
			tempMid := len(b.CurrentSearchSpace) / 2
			if len(b.CurrentSearchSpace) == 1 {
				tempMid = 1
			}
			tempGroupAOrig := slices.Clone(b.CurrentSearchSpace[:tempMid])
			effectiveGroupToReapply = b.calculateEffectiveGroup(tempGroupAOrig, "Prospective A")
			currentOperationDesc = "prospective Group A for next iteration"
		} else { // Search space is empty, or bisection concluded. Apply only forced-enabled mods.
			effectiveGroupToReapply = make(map[string]bool)
			for id := range b.ForceEnabled {
				if !b.ForceDisabled[id] { // Respect force-disable even for force-enable
					effectiveGroupToReapply[id] = true
				}
			}
			currentOperationDesc = "post-bisection or empty search space state"
		}
	default:
		log.Printf("%sRecalculation: Unknown testing phase %d. No mod set applied.", LogErrorPrefix, b.testingPhase)
		return
	}
	b.applyModSet(effectiveGroupToReapply)
	log.Printf("%sRe-applied mod set for %s. Effective mods: %d.", LogInfoPrefix, currentOperationDesc, len(effectiveGroupToReapply))
}

func (b *Bisector) markModsAsGood(modIDs []string) {
	if len(modIDs) == 0 {
		return
	}
	log.Printf("%sMarking %d mods as good: %v", LogInfoPrefix, len(modIDs), modIDs)
	for _, id := range modIDs {
		if mod, ok := b.AllMods[id]; ok {
			mod.ConfirmedGood = true
		}
	}
}

func (b *Bisector) RestoreInitialModState() {
	log.Printf("%sRestoring initial mod states...", LogInfoPrefix)
	if b.AllMods == nil {
		log.Printf("%sNo mods loaded; cannot restore initial states.", LogWarningPrefix)
		return
	}
	restoredCount, errorCount := 0, 0
	for _, mod := range b.AllMods {
		activePath := filepath.Join(b.ModsDir, mod.BaseFilename+".jar")
		disabledPath := filepath.Join(b.ModsDir, mod.BaseFilename+".jar.disabled")
		_, errActiveExists := os.Stat(activePath)
		_, errDisabledExists := os.Stat(disabledPath)
		physicallyActive := !os.IsNotExist(errActiveExists) && os.IsNotExist(errDisabledExists)

		var err error
		actionTaken := false
		if mod.IsInitiallyActive {
			if !physicallyActive {
				err = mod.Enable(b.ModsDir)
				actionTaken = true
			}
		} else {
			if physicallyActive {
				err = mod.Disable(b.ModsDir)
				actionTaken = true
			}
		}
		if err != nil && !os.IsNotExist(err) { // os.IsNotExist can occur if file was deleted externally
			log.Printf("%sError restoring mod %s: %v", LogErrorPrefix, mod.ModID(), err)
			errorCount++
		} else if actionTaken {
			restoredCount++
		}
	}
	log.Printf("%sInitial mod states restoration attempt: %d actions, %d errors.", LogInfoPrefix, restoredCount, errorCount)
}

func mapKeys(m map[string]bool) []string {
	if m == nil {
		return []string{}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func cloneMap(originalMap map[string]bool) map[string]bool {
	if originalMap == nil {
		return make(map[string]bool)
	}
	newMap := make(map[string]bool, len(originalMap))
	for k, v := range originalMap {
		newMap[k] = v
	}
	return newMap
}

// GetStrategyType is a helper to get the BisectionStrategyType from an instance.
// This is primarily for logging/display if needed, as the type itself is stored in AppContext.
func GetStrategyType(s BisectionStrategy) BisectionStrategyType {
	switch s.(type) {
	case *fastStrategy:
		return StrategyFast
	case *partialStrategy:
		return StrategyPartial
	case *fullStrategy:
		return StrategyFull
	default:
		log.Printf("%sUnknown strategy type: %T", LogWarningPrefix, s)
		return StrategyFast // Fallback for safety
	}
}

func (b *Bisector) GetCurrentTestingPhase() int                { return b.testingPhase }
func (b *Bisector) GetIterationCount() int                     { return b.IterationCount }
func (b *Bisector) GetCurrentGroupAOriginal() []string         { return b.CurrentGroupAOriginal }
func (b *Bisector) GetCurrentGroupBOriginal() []string         { return b.CurrentGroupBOriginal }
func (b *Bisector) GetCurrentGroupAEffective() map[string]bool { return b.CurrentGroupAEffective }
func (b *Bisector) GetCurrentGroupBEffective() map[string]bool { return b.CurrentGroupBEffective }
func (b *Bisector) FormatQuestion(groupDesignator string, originalGroup []string, effectiveGroup map[string]bool) string {
	return b.formatQuestion(groupDesignator, originalGroup, effectiveGroup)
}
