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
	AllMods         map[string]*Mod // All manageable top-level mods
	AllModIDsSorted []string        // Sorted IDs of mods in AllMods
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
	MaxIterations   int // Estimated maximum iterations
	InitialModCount int

	LastKnownBadEffectiveSet map[string]bool
	lastTestCausedIssue      bool // True if the most recent test failed

	resolver *DependencyResolver
}

// Constants for testing phases.
const (
	PhasePrepareA = 0
	PhaseTestingA = 1
	PhaseTestingB = 2
)

// NewBisector initializes a new Bisector instance.
func NewBisector(modsDir string, allMods map[string]*Mod, allModIDsSorted []string, potentialProviders PotentialProvidersMap) *Bisector {
	b := &Bisector{
		ModsDir:                  modsDir,
		AllMods:                  allMods,
		AllModIDsSorted:          allModIDsSorted,
		ForceEnabled:             make(map[string]bool),
		ForceDisabled:            make(map[string]bool),
		History:                  make([]BisectionStep, 0),
		LastKnownBadEffectiveSet: make(map[string]bool),
		testingPhase:             PhasePrepareA,
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

	log.Printf("%sBisector initialized. Initial search space: %d mods. Max iterations: ~%d.",
		LogInfoPrefix, b.InitialModCount, b.MaxIterations)
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
	if len(b.CurrentSearchSpace) == 1 { // If only one mod, it becomes Group A
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
		msg += " Last test was success. Bisection inconclusive."
	}
	return b.formatConclusionMessage(msg)
}

// ProcessUserFeedback updates state based on user's answer.
// Returns: done (bool), nextQuestion (string), status (string).
func (b *Bisector) ProcessUserFeedback(issueOccurred bool) (bool, string, string) {
	if len(b.History) == 0 {
		log.Printf("%sProcessUserFeedback: No history available.", LogErrorPrefix)
		return true, "", "Error: Bisection state error (no history)."
	}
	currentIterationHistory := &b.History[len(b.History)-1]

	b.lastTestCausedIssue = issueOccurred

	switch b.testingPhase {
	case PhaseTestingA:
		return b.processGroupAResult(issueOccurred, currentIterationHistory)
	case PhaseTestingB:
		return b.processGroupBResult(issueOccurred, currentIterationHistory)
	default:
		log.Printf("%sProcessUserFeedback called in unexpected phase: %d", LogErrorPrefix, b.testingPhase)
		return true, "", "Error: Unexpected bisection phase."
	}
}

func (b *Bisector) prepareNewIteration() (bool, string, string) {
	b.testingPhase = PhasePrepareA
	return b.PrepareNextTestOrConclude()
}

func (b *Bisector) concludeBisectionWithSingleMod(modID string, reasonTemplate string) (bool, string, string) {
	modName := modID
	if mod, ok := b.AllMods[modID]; ok {
		modName = mod.FriendlyName()
	}
	finalMsg := fmt.Sprintf(reasonTemplate, modName, modID)
	return true, "", b.formatConclusionMessage(finalMsg)
}

func (b *Bisector) processGroupAResult(issueOccurredInA bool, currentIterationHistory *BisectionStep) (bool, string, string) {
	currentIterationHistory.IssuePresentInA = issueOccurredInA
	b.recordTestOutcomeDetails(issueOccurredInA, "A", b.CurrentGroupAEffective) // Log and store bad set if needed

	if !issueOccurredInA { // Group A SUCCESS (issue GONE)
		log.Printf("%sGroup A passed. Problem is in Group B original candidates.", LogInfoPrefix)
		b.markModsAsGood(b.CurrentGroupAOriginal)
		b.CurrentSearchSpace = b.CurrentGroupBOriginal
		return b.prepareNewIteration()
	}

	// Group A FAILED (issue PRESENT)
	log.Printf("%sGroup A failed.", LogInfoPrefix)
	if len(b.CurrentGroupBOriginal) == 0 {
		log.Printf("%sGroup A failed, and Group B is empty. Problem must be in Group A original candidates.", LogInfoPrefix)
		b.CurrentSearchSpace = b.CurrentGroupAOriginal
		if len(b.CurrentSearchSpace) == 1 {
			return b.concludeBisectionWithSingleMod(b.CurrentSearchSpace[0], "Problematic mod identified: %s (%s). It was in the failing Group A, and Group B was empty.")
		}
		return b.prepareNewIteration()
	}

	// Group A failed, Group B exists, proceed to test B
	log.Printf("%sProceeding to test Group B.", LogInfoPrefix)
	b.applyModSet(b.CurrentGroupBEffective) // CurrentGroupBEffective already calculated
	b.testingPhase = PhaseTestingB
	status := fmt.Sprintf("Iteration %d. Group A failed. Now testing Group B.", b.IterationCount)
	question := b.formatQuestion("B", b.CurrentGroupBOriginal, b.CurrentGroupBEffective)
	return false, question, status
}

func (b *Bisector) processGroupBResult(issueOccurredInB bool, currentIterationHistory *BisectionStep) (bool, string, string) {
	b.recordTestOutcomeDetails(issueOccurredInB, "B", b.CurrentGroupBEffective) // Log and store bad set if needed

	if !currentIterationHistory.IssuePresentInA {
		// This indicates a logical flaw if Group A had passed but we proceeded to test B.
		log.Printf("%sError: Tested Group B, but Group A was marked as passed for this iteration. State inconsistency.", LogErrorPrefix)
		return b.prepareNewIteration() // Attempt to recover by preparing a new iteration.
	}

	// Group A FAILED. Now evaluate Group B result.
	if !issueOccurredInB { // Group B SUCCESS (issue GONE) -> A failed, B passed
		log.Printf("%sGroup A failed, Group B passed. Problem is in Group A original candidates.", LogInfoPrefix)
		b.markModsAsGood(b.CurrentGroupBOriginal)
		b.CurrentSearchSpace = b.CurrentGroupAOriginal
		if len(b.CurrentSearchSpace) == 1 {
			return b.concludeBisectionWithSingleMod(b.CurrentSearchSpace[0], "Problematic mod identified: %s (%s). Group A (containing it) failed, and Group B passed.")
		}
	} else { // Group B FAILED (issue PRESENT) - Both A and B failed
		log.Printf("%sBoth Groups A and B failed. Problem in shared elements.", LogInfoPrefix)
		b.CurrentSearchSpace = b.calculateIntersectionSearchSpace(
			currentIterationHistory.SearchSpace,
			b.CurrentGroupAEffective,
			b.CurrentGroupBEffective,
		)
		if len(b.CurrentSearchSpace) == 1 {
			return b.concludeBisectionWithSingleMod(b.CurrentSearchSpace[0], "Problematic mod identified: %s (%s). It was common to both failing Group A and Group B.")
		}
	}
	return b.prepareNewIteration()
}

func (b *Bisector) formatQuestion(groupDesignator string, originalGroup []string, effectiveGroup map[string]bool) string {
	return fmt.Sprintf("Testing Group %s (%d mods, %d effective).\nLaunch Minecraft now.\n\nDoes the issue STILL OCCUR?",
		groupDesignator, len(originalGroup), len(effectiveGroup))
}

func (b *Bisector) formatConclusionMessage(baseMessage string) string {
	conclusion := baseMessage
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
	} else if b.lastTestCausedIssue {
		conclusion += "\nThe very last test performed still showed the issue."
	}
	log.Printf("%sBisection concluded: %s", LogInfoPrefix, conclusion)
	return conclusion
}

// recordTestOutcomeDetails logs the outcome of a specific test group and updates LastKnownBadEffectiveSet if the issue occurred.
// b.lastTestCausedIssue is assumed to be already set by the caller (ProcessUserFeedback)
// based on the direct outcome of the user's test.
func (b *Bisector) recordTestOutcomeDetails(testGroupCausedIssue bool, groupTested string, effectiveSetTested map[string]bool) {
	if testGroupCausedIssue {
		b.LastKnownBadEffectiveSet = cloneMap(effectiveSetTested)
		log.Printf("%sIssue PRESENT with Group %s. Effective set (%d mods): %v",
			LogInfoPrefix, groupTested, len(effectiveSetTested), mapKeys(effectiveSetTested))
	} else {
		log.Printf("%sIssue GONE with Group %s.", LogInfoPrefix, groupTested)
	}
}

func (b *Bisector) calculateEffectiveGroup(originalGroup []string, name string) map[string]bool {
	// The resolver was initialized with references to b.ForceEnabled and b.ForceDisabled,
	// so it will use their current values.
	effectiveSet, resolutionDetails := b.resolver.ResolveEffectiveSet(originalGroup)

	// Log the resolution path for debugging/visualization prep
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
		// Store resolutionDetails on AppContext or Bisector if it needs to be accessed by UI later
		// For now, just logging it.
	}
	return effectiveSet
}

func (b *Bisector) calculateIntersectionSearchSpace(iterationInitialSearchSpace []string, groupAEffective, groupBEffective map[string]bool) []string {
	candidatePool := make(map[string]bool)
	for _, modID := range iterationInitialSearchSpace {
		candidatePool[modID] = true
	}
	for modID := range b.ForceEnabled {
		candidatePool[modID] = true
	}

	var intersectedAndFiltered []string
	if groupAEffective != nil && groupBEffective != nil { // Ensure maps are not nil
		for modID := range groupAEffective {
			if groupBEffective[modID] && candidatePool[modID] {
				intersectedAndFiltered = append(intersectedAndFiltered, modID)
			}
		}
	}
	sort.Strings(intersectedAndFiltered)
	log.Printf("%sIntersection result for search space: %v", LogInfoPrefix, intersectedAndFiltered)
	return intersectedAndFiltered
}

func (b *Bisector) applyModSet(effectiveModsToEnable map[string]bool) {
	log.Printf("%sApplying mod set. Effective mods to enable (%d): %v", LogInfoPrefix, len(effectiveModsToEnable), mapKeys(effectiveModsToEnable))
	var appliedChanges, renameErrors []string

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
		actionTaken := false
		if shouldBeActive && !mod.IsCurrentlyActive {
			err = mod.Enable(b.ModsDir)
			if err == nil {
				appliedChanges = append(appliedChanges, mod.FriendlyName()+" (Enabled)")
			}
			actionTaken = true
		} else if !shouldBeActive && mod.IsCurrentlyActive {
			err = mod.Disable(b.ModsDir)
			if err == nil {
				appliedChanges = append(appliedChanges, mod.FriendlyName()+" (Disabled)")
			}
			actionTaken = true
		}
		if err != nil {
			log.Printf("%sError applying state for %s (active: %t): %v", LogErrorPrefix, mod.ModID(), shouldBeActive, err)
			renameErrors = append(renameErrors, fmt.Sprintf("%s: %v", mod.FriendlyName(), err))
		} else if actionTaken {
			// Action successful
		}
	}
	if len(appliedChanges) > 0 {
		log.Printf("%sMod state changes applied: %d actions.", LogInfoPrefix, len(appliedChanges))
	}
	if len(renameErrors) > 0 {
		log.Printf("%sWARNING: Renaming errors encountered: %s.", LogWarningPrefix, strings.Join(renameErrors, "; "))
	}
}

func (b *Bisector) UndoLastStep() (possible bool, message string) {
	if len(b.History) == 0 {
		return false, "Cannot undo: No history available."
	}

	b.History = b.History[:len(b.History)-1]
	if b.IterationCount > 0 {
		b.IterationCount--
	}
	b.testingPhase = PhasePrepareA

	if len(b.History) > 0 {
		lastValidStep := b.History[len(b.History)-1]
		b.CurrentSearchSpace = slices.Clone(lastValidStep.SearchSpace)
		b.ForceEnabled = cloneMap(lastValidStep.ForceEnabledSnapshot)
		b.ForceDisabled = cloneMap(lastValidStep.ForceDisabledSnapshot)
		for _, mod := range b.AllMods {
			mod.ConfirmedGood = false
		}
		log.Printf("%sUndo: Reverted to state before Iteration %d. Search space: %d.", LogInfoPrefix, b.IterationCount+1, len(b.CurrentSearchSpace))
	} else {
		b.CurrentSearchSpace = []string{}
		for _, modID := range b.AllModIDsSorted {
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
	case PhaseTestingA:
		effectiveGroupToReapply = b.calculateEffectiveGroup(b.CurrentGroupAOriginal, "A")
		b.CurrentGroupAEffective = effectiveGroupToReapply
		currentOperationDesc = "current Group A test"
	case PhaseTestingB:
		effectiveGroupToReapply = b.calculateEffectiveGroup(b.CurrentGroupBOriginal, "B")
		b.CurrentGroupBEffective = effectiveGroupToReapply
		currentOperationDesc = "current Group B test"
	case PhasePrepareA:
		if b.IterationCount == 0 && len(b.CurrentSearchSpace) > 0 {
			// Before first iteration, apply a neutral set (all enabled by default, respecting forces)
			effectiveGroupToReapply = make(map[string]bool)
			for id := range b.AllMods {
				if !b.ForceDisabled[id] {
					effectiveGroupToReapply[id] = true
				}
			}
			for id := range b.ForceEnabled {
				if !b.ForceDisabled[id] {
					effectiveGroupToReapply[id] = true
				}
			}
			currentOperationDesc = "initial pre-bisection state"
		} else if len(b.CurrentSearchSpace) > 0 {
			// About to prepare Group A for the *next* iteration, so apply its calculated state.
			tempMid := len(b.CurrentSearchSpace) / 2
			if len(b.CurrentSearchSpace) == 1 {
				tempMid = 1
			}
			tempGroupAOrig := slices.Clone(b.CurrentSearchSpace[:tempMid])
			effectiveGroupToReapply = b.calculateEffectiveGroup(tempGroupAOrig, "A")
			currentOperationDesc = "pending Group A of next iteration"
		} else { // Bisection likely concluded or search space became empty through other means
			effectiveGroupToReapply = make(map[string]bool) // Default to all forced enabled, nothing else
			for id := range b.ForceEnabled {
				if !b.ForceDisabled[id] {
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
		if err != nil && !os.IsNotExist(err) {
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

func (b *Bisector) GetCurrentTestingPhase() int                { return b.testingPhase }
func (b *Bisector) GetIterationCount() int                     { return b.IterationCount }
func (b *Bisector) GetCurrentGroupAOriginal() []string         { return b.CurrentGroupAOriginal }
func (b *Bisector) GetCurrentGroupBOriginal() []string         { return b.CurrentGroupBOriginal }
func (b *Bisector) GetCurrentGroupAEffective() map[string]bool { return b.CurrentGroupAEffective }
func (b *Bisector) GetCurrentGroupBEffective() map[string]bool { return b.CurrentGroupBEffective }
func (b *Bisector) FormatQuestion(groupDesignator string, originalGroup []string, effectiveGroup map[string]bool) string {
	return b.formatQuestion(groupDesignator, originalGroup, effectiveGroup)
}
