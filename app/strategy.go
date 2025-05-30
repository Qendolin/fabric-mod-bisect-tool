package app

import (
	"fmt"
	"log"
)

// NextActionOutcome details what the Bisector should do after a strategy processes a test result.
type NextActionOutcome struct {
	TestB            bool   // Should Group B be tested next?
	Conclude         bool   // Should bisection conclude now?
	NextQuestionForB string // Question for Group B (if TestB is true).
	Message          string // Status or Conclusion message for the UI.
}

// BisectionStrategy defines the interface for different bisection approaches.
// Implementations of this interface will directly modify b.CurrentSearchSpace
// and call b.markModsAsGood as needed based on their logic.
type BisectionStrategy interface {
	// DetermineNextActionAfterA is called after Group A has been tested.
	// issueOccurredInA: The result of testing Group A.
	// b: The Bisector instance, providing access to its state and methods.
	DetermineNextActionAfterA(issueOccurredInA bool, b *Bisector) NextActionOutcome

	// DetermineNextActionAfterB is called after Group B has been tested (if it was).
	// issueOccurredInB: The result of testing Group B.
	// issueWasPresentInA: The result of the Group A test in the same iteration.
	// b: The Bisector instance.
	DetermineNextActionAfterB(issueOccurredInB bool, issueWasPresentInA bool, b *Bisector) NextActionOutcome
}

type fastStrategy struct{}

// NewFastStrategy creates a new instance of the fast bisection strategy.
func NewFastStrategy() BisectionStrategy {
	return &fastStrategy{}
}

func (s *fastStrategy) DetermineNextActionAfterA(issueOccurredInA bool, b *Bisector) NextActionOutcome {

	// Check if this is potentially a final confirmation step
	if !issueOccurredInA && len(b.CurrentGroupBOriginal) <= 1 {
		// We NEED to test group B to confirm that the issue is still present at all.
		log.Printf("%sFastStrategy: Group A (size %d) passed. Potentially final step. Testing Group B (size %d) for confirmation.",
			LogInfoPrefix, len(b.CurrentGroupAOriginal), len(b.CurrentGroupBOriginal))

		questionForB := b.formatQuestion("B (Final)", b.CurrentGroupBOriginal, b.CurrentGroupBEffective)
		statusMsg := fmt.Sprintf("Fast Strategy: Iteration %d. Group A passed. Confirming with Group B.", b.IterationCount)
		return NextActionOutcome{TestB: true, NextQuestionForB: questionForB, Message: statusMsg}
	}

	if issueOccurredInA { // Issue in A -> problem is in A original candidates
		log.Printf("%sFastStrategy: Issue in A. Assuming problem in Group A original candidates (%v). Marking Group B original (%v) as good.",
			LogInfoPrefix, b.CurrentGroupAOriginal, b.CurrentGroupBOriginal)
		b.markModsAsGood(b.CurrentGroupBOriginal)
		b.CurrentSearchSpace = b.CurrentGroupAOriginal
	} else { // No issue in A -> problem is in B original candidates
		log.Printf("%sFastStrategy: No issue in A. Assuming problem in Group B original candidates (%v). Marking Group A original (%v) as good.",
			LogInfoPrefix, b.CurrentGroupBOriginal, b.CurrentGroupAOriginal)
		b.markModsAsGood(b.CurrentGroupAOriginal)
		b.CurrentSearchSpace = b.CurrentGroupBOriginal
	}
	msg := fmt.Sprintf("Fast Strategy: Iteration %d. Search space is now %d mods.", b.IterationCount, len(b.CurrentSearchSpace))
	return NextActionOutcome{TestB: false, Conclude: false, Message: msg}
}

func (s *fastStrategy) DetermineNextActionAfterB(issueOccurredInB bool, issueWasPresentInA bool, b *Bisector) NextActionOutcome {

	if issueWasPresentInA || len(b.CurrentGroupBOriginal) != 1 {
		// This should not happen
		log.Printf("%sFastStrategy: DetermineNextActionAfterB called unexpectedly. This indicates an issue with bisection flow control.", LogErrorPrefix)
		msg := fmt.Sprintf("Fast Strategy (Error): Iteration %d. Unexpected Group B test. Search space: %d mods.", b.IterationCount, len(b.CurrentSearchSpace))
		return NextActionOutcome{Conclude: false, Message: msg}
	}

	// This path is only used in the final confirmation step

	if issueOccurredInB { // B FAILED (A passed, B failed) -> B is the culprit
		log.Printf("%sFastStrategy (Confirmation): Group A passed, Group B failed. Problem is in Group B original (%v). Marking Group A original (%v) as good.",
			LogInfoPrefix, b.CurrentGroupBOriginal, b.CurrentGroupAOriginal)
		b.markModsAsGood(b.CurrentGroupAOriginal)
		b.CurrentSearchSpace = b.CurrentGroupBOriginal
		modID := b.CurrentSearchSpace[0]
		conclusion := fmt.Sprintf("Problematic mod identified: %s (%s). Group A passed, Group B (containing it) failed.",
			b.AllMods[modID].FriendlyName(), modID)
		return NextActionOutcome{Conclude: true, Message: conclusion}
	} else { // B PASSED (A passed, B passed) -> Both passed, inconclusive
		log.Printf("%sFastStrategy (Confirmation): Both A and B passed. Bisection inconclusive for this step.", LogInfoPrefix)
		b.markModsAsGood(b.CurrentGroupAOriginal)
		b.markModsAsGood(b.CurrentGroupBOriginal)
		b.CurrentSearchSpace = []string{}
		conclusion := "Bisection inconclusive (Fast Strategy): Issue disappeared in both Group A and Group B."
		return NextActionOutcome{Conclude: true, Message: conclusion}
	}
}

type partialStrategy struct{}

// NewPartialStrategy creates a new instance of the partial bisection strategy.
func NewPartialStrategy() BisectionStrategy {
	return &partialStrategy{}
}

func (s *partialStrategy) DetermineNextActionAfterA(issueOccurredInA bool, b *Bisector) NextActionOutcome {
	if !issueOccurredInA { // Group A SUCCESS (issue GONE) -> problem is in Group B original
		log.Printf("%sPartialStrategy: Group A passed. Problem assumed in Group B original candidates (%v). Marking Group A original (%v) as good.",
			LogInfoPrefix, b.CurrentGroupBOriginal, b.CurrentGroupAOriginal)
		b.markModsAsGood(b.CurrentGroupAOriginal)
		b.CurrentSearchSpace = b.CurrentGroupBOriginal
		msg := fmt.Sprintf("Partial Strategy: Iteration %d. Group A passed. Search space is now %d mods (from Group B).", b.IterationCount, len(b.CurrentSearchSpace))
		return NextActionOutcome{TestB: false, Conclude: false, Message: msg}
	}

	// Group A FAILED (issue PRESENT)
	log.Printf("%sPartialStrategy: Group A failed.", LogInfoPrefix)
	if len(b.CurrentGroupBOriginal) == 0 {
		log.Printf("%sPartialStrategy: Group A failed, and Group B is empty. Problem must be in Group A original candidates (%v).",
			LogInfoPrefix, b.CurrentGroupAOriginal)
		b.CurrentSearchSpace = b.CurrentGroupAOriginal // No change to Group B as it's empty
		if len(b.CurrentSearchSpace) == 1 {
			conclusion := fmt.Sprintf("Problematic mod identified: %s (%s). It was in the failing Group A, and Group B was empty.",
				b.AllMods[b.CurrentSearchSpace[0]].FriendlyName(), b.CurrentSearchSpace[0])
			return NextActionOutcome{Conclude: true, Message: conclusion}
		}
		msg := fmt.Sprintf("Partial Strategy: Iteration %d. Group A failed (B empty). Search space is now %d mods (from Group A).", b.IterationCount, len(b.CurrentSearchSpace))
		return NextActionOutcome{TestB: false, Conclude: false, Message: msg}
	}

	// Group A failed, Group B exists, proceed to test B
	log.Printf("%sPartialStrategy: Proceeding to test Group B.", LogInfoPrefix)
	question := b.formatQuestion("B", b.CurrentGroupBOriginal, b.CurrentGroupBEffective)
	msg := fmt.Sprintf("Partial Strategy: Iteration %d. Group A failed. Now testing Group B.", b.IterationCount)
	return NextActionOutcome{TestB: true, NextQuestionForB: question, Message: msg}
}

func (s *partialStrategy) DetermineNextActionAfterB(issueOccurredInB bool, issueWasPresentInA bool, b *Bisector) NextActionOutcome {
	// This path is taken when Group A failed (issueWasPresentInA is true).
	if !issueOccurredInB { // Group B SUCCESS (issue GONE) -> A failed, B passed
		log.Printf("%sPartialStrategy: Group A failed, Group B passed. Problem is in Group A original candidates (%v). Marking Group B original (%v) as good.",
			LogInfoPrefix, b.CurrentGroupAOriginal, b.CurrentGroupBOriginal)
		b.markModsAsGood(b.CurrentGroupBOriginal)
		b.CurrentSearchSpace = b.CurrentGroupAOriginal
		if len(b.CurrentSearchSpace) == 1 {
			conclusion := fmt.Sprintf("Problematic mod identified: %s (%s). Group A (containing it) failed, and Group B passed.",
				b.AllMods[b.CurrentSearchSpace[0]].FriendlyName(), b.CurrentSearchSpace[0])
			return NextActionOutcome{Conclude: true, Message: conclusion}
		}
	} else { // Group B FAILED (issue PRESENT) - Both A and B failed
		log.Printf("%sPartialStrategy: Both Groups A and B failed. Problem in shared elements or dependencies. Calculating intersection.", LogInfoPrefix)
		// SearchSpace for this iteration was stored in history before split
		iterationInitialSearchSpace := b.History[len(b.History)-1].SearchSpace
		b.CurrentSearchSpace = b.calculateIntersectionSearchSpace(
			iterationInitialSearchSpace,
			b.CurrentGroupAEffective,
			b.CurrentGroupBEffective,
		)
		if len(b.CurrentSearchSpace) == 1 {
			conclusion := fmt.Sprintf("Problematic mod identified: %s (%s). It was common to both failing Group A and Group B (effective sets).",
				b.AllMods[b.CurrentSearchSpace[0]].FriendlyName(), b.CurrentSearchSpace[0])
			return NextActionOutcome{Conclude: true, Message: conclusion}
		}
	}
	msg := fmt.Sprintf("Partial Strategy: Iteration %d. Finished Group B test. Search space is now %d mods.", b.IterationCount, len(b.CurrentSearchSpace))
	return NextActionOutcome{Conclude: false, Message: msg}
}

type fullStrategy struct{}

// NewFullStrategy creates a new instance of the full bisection strategy.
func NewFullStrategy() BisectionStrategy {
	return &fullStrategy{}
}

func (s *fullStrategy) DetermineNextActionAfterA(issueOccurredInA bool, b *Bisector) NextActionOutcome {
	statusMsg := fmt.Sprintf("Full Strategy: Iteration %d. Group A tested.", b.IterationCount)

	if len(b.CurrentGroupBOriginal) == 0 { // No Group B to test
		log.Printf("%sFullStrategy: Group A tested. Group B is empty.", LogInfoPrefix)
		if issueOccurredInA {
			log.Printf("%sFullStrategy: Issue in A, B empty. Problem in Group A original candidates (%v).", LogInfoPrefix, b.CurrentGroupAOriginal)
			b.CurrentSearchSpace = b.CurrentGroupAOriginal
			if len(b.CurrentSearchSpace) == 1 {
				conclusion := fmt.Sprintf("Problematic mod identified: %s (%s). It was in the failing Group A, and Group B was empty.",
					b.AllMods[b.CurrentSearchSpace[0]].FriendlyName(), b.CurrentSearchSpace[0])
				return NextActionOutcome{Conclude: true, Message: conclusion}
			}
		} else { // No issue in A, B is empty. Bisection inconclusive or issue resolved.
			log.Printf("%sFullStrategy: No issue in A, B empty. Bisection ends, issue not found in remaining set.", LogInfoPrefix)
			b.CurrentSearchSpace = []string{} // Clear search space
			conclusion := "Bisection inconclusive: Issue disappeared after testing Group A, and Group B was empty."
			return NextActionOutcome{Conclude: true, Message: conclusion}
		}
		return NextActionOutcome{TestB: false, Conclude: false, Message: statusMsg + fmt.Sprintf(" Search space: %d.", len(b.CurrentSearchSpace))}
	}

	// Group B exists, always test it
	log.Printf("%sFullStrategy: Group A tested. Proceeding to test Group B.", LogInfoPrefix)
	question := b.formatQuestion("B", b.CurrentGroupBOriginal, b.CurrentGroupBEffective)
	return NextActionOutcome{TestB: true, NextQuestionForB: question, Message: statusMsg + " Now testing Group B."}
}

func (s *fullStrategy) DetermineNextActionAfterB(issueOccurredInB bool, issueWasPresentInA bool, b *Bisector) NextActionOutcome {
	log.Printf("%sFullStrategy: Group B tested. Group A issue: %t, Group B issue: %t.", LogInfoPrefix, issueWasPresentInA, issueOccurredInB)

	if issueWasPresentInA { // Group A FAILED
		if !issueOccurredInB { // Group B PASSED (A failed, B passed) -> problem in A original
			log.Printf("%sFullStrategy: A failed, B passed. Problem in Group A original candidates (%v). Marking Group B original (%v) as good.",
				LogInfoPrefix, b.CurrentGroupAOriginal, b.CurrentGroupBOriginal)
			b.markModsAsGood(b.CurrentGroupBOriginal)
			b.CurrentSearchSpace = b.CurrentGroupAOriginal
			if len(b.CurrentSearchSpace) == 1 {
				conclusion := fmt.Sprintf("Problematic mod identified: %s (%s). Group A (containing it) failed, Group B passed.",
					b.AllMods[b.CurrentSearchSpace[0]].FriendlyName(), b.CurrentSearchSpace[0])
				return NextActionOutcome{Conclude: true, Message: conclusion}
			}
		} else { // Group B FAILED (A failed, B failed) -> problem in common (effective sets)
			log.Printf("%sFullStrategy: Both A and B failed. Problem in shared elements. Calculating intersection.", LogInfoPrefix)
			iterationInitialSearchSpace := b.History[len(b.History)-1].SearchSpace
			b.CurrentSearchSpace = b.calculateIntersectionSearchSpace(
				iterationInitialSearchSpace,
				b.CurrentGroupAEffective,
				b.CurrentGroupBEffective,
			)
			if len(b.CurrentSearchSpace) == 1 {
				conclusion := fmt.Sprintf("Problematic mod identified: %s (%s). It was common to both failing Group A and Group B (effective sets).",
					b.AllMods[b.CurrentSearchSpace[0]].FriendlyName(), b.CurrentSearchSpace[0])
				return NextActionOutcome{Conclude: true, Message: conclusion}
			}
		}
	} else { // Group A PASSED
		if issueOccurredInB { // Group B FAILED (A passed, B failed) -> problem in B original
			log.Printf("%sFullStrategy: A passed, B failed. Problem in Group B original candidates (%v). Marking Group A original (%v) as good.",
				LogInfoPrefix, b.CurrentGroupBOriginal, b.CurrentGroupAOriginal)
			b.markModsAsGood(b.CurrentGroupAOriginal)
			b.CurrentSearchSpace = b.CurrentGroupBOriginal
			if len(b.CurrentSearchSpace) == 1 {
				conclusion := fmt.Sprintf("Problematic mod identified: %s (%s). Group A passed, Group B (containing it) failed.",
					b.AllMods[b.CurrentSearchSpace[0]].FriendlyName(), b.CurrentSearchSpace[0])
				return NextActionOutcome{Conclude: true, Message: conclusion}
			}
		} else { // Group B PASSED (A passed, B passed) -> Bisection inconclusive
			log.Printf("%sFullStrategy: Both A and B passed. Bisection inconclusive for this step.", LogInfoPrefix)
			b.CurrentSearchSpace = []string{} // Clear search space
			conclusion := "Bisection inconclusive: Issue disappeared in both Group A and Group B."
			return NextActionOutcome{Conclude: true, Message: conclusion}
		}
	}
	msg := fmt.Sprintf("Full Strategy: Iteration %d. Finished Group B test. Search space is now %d mods.", b.IterationCount, len(b.CurrentSearchSpace))
	return NextActionOutcome{Conclude: false, Message: msg}
}
