package ui

import "github.com/rivo/tview"

// FocusChainEntry holds a primitive and its complete ancestry path from the root.
type FocusChainEntry struct {
	Primitive    tview.Primitive
	AncestryPath []tview.Primitive
}

// FocusManager handles cycling focus between a dynamic set of primitives.
type FocusManager struct {
	app AppInterface
}

// NewFocusManager creates a new focus manager.
func NewFocusManager(app AppInterface) *FocusManager {
	return &FocusManager{app: app}
}

// Cycle moves the focus to the next or previous primitive in the focus chain.
func (fm *FocusManager) Cycle(root tview.Primitive, forward bool) bool {
	focusableEntries := fm.buildFocusChain(root)
	if len(focusableEntries) == 0 {
		return false
	}

	currentFocus := fm.app.GetFocus()
	currentIndex := -1
	var currentFocusEntry *FocusChainEntry

	for i, entry := range focusableEntries {
		if entry.Primitive == currentFocus {
			currentIndex = i
			currentFocusEntry = &focusableEntries[i]
			break
		}
	}

	if currentIndex == -1 {
		for i := len(focusableEntries) - 1; i >= 0; i-- {
			entry := focusableEntries[i]
			if focusable, ok := entry.Primitive.(Focusable); ok {
				if fm.isDescendant(focusable, currentFocus, focusableEntries) {
					currentIndex = i
					currentFocusEntry = &focusableEntries[i]
					break
				}
			}
		}
	}

	if currentIndex == -1 {
		currentIndex = 0
		currentFocusEntry = nil
	}

	var nextIndex int
	if forward {
		nextIndex = (currentIndex + 1) % len(focusableEntries)
	} else {
		nextIndex = (currentIndex - 1 + len(focusableEntries)) % len(focusableEntries)
	}

	nextElementEntry := focusableEntries[nextIndex]
	nextElement := nextElementEntry.Primitive
	ancestryChain := nextElementEntry.AncestryPath

	mayDelegateFocus := false
	for _, ancestor := range ancestryChain {
		if _, container := ancestor.(Focusable); container && ancestor != nextElement && !ancestor.HasFocus() {
			ancestor.Focus(func(p tview.Primitive) {
				if mayDelegateFocus {
					fm.app.SetFocus(p)
				}
			})
		}
	}
	mayDelegateFocus = true

	fm.app.SetFocus(nextElement)

	if currentFocusEntry != nil {
		currentAncestry := currentFocusEntry.AncestryPath

		nextAncestrySet := make(map[tview.Primitive]struct{})
		for _, p := range ancestryChain {
			nextAncestrySet[p] = struct{}{}
		}

		for _, p := range currentAncestry {
			if _, found := nextAncestrySet[p]; !found {
				if blurrable, ok := p.(interface{ Blur() }); ok {
					blurrable.Blur()
				}
			}
		}
	}

	return true
}

// buildFocusChain performs a depth-first traversal to find all focusable primitives,
// including containers that implement the Focusable interface. It populates
// the AncestryPath for each FocusChainEntry.
func (fm *FocusManager) buildFocusChain(root tview.Primitive) []FocusChainEntry {
	var chain []FocusChainEntry

	if _, ok := root.(Focusable); !ok {
		return chain
	}

	var traverse func(p tview.Primitive, currentPath []tview.Primitive)

	traverse = func(p tview.Primitive, currentPath []tview.Primitive) {
		newPath := append(currentPath, p)

		if focusable, ok := p.(Focusable); ok {
			children := focusable.GetFocusablePrimitives()
			for _, child := range children {
				if child != p {
					traverse(child, newPath)
				}
			}
		} else {
			chain = append(chain, FocusChainEntry{Primitive: p, AncestryPath: newPath})
		}
	}

	traverse(root, []tview.Primitive{})

	return chain
}

// isDescendant checks if a primitive `p` is a descendant of a `Focusable` container,
// leveraging the pre-built `focusableEntries` for faster lookup.
func (fm *FocusManager) isDescendant(container Focusable, p tview.Primitive, allFocusableEntries []FocusChainEntry) bool {
	if container.(tview.Primitive) == p {
		return false
	}

	var pEntry *FocusChainEntry
	for _, entry := range allFocusableEntries {
		if entry.Primitive == p {
			pEntry = &entry
			break
		}
	}

	if pEntry == nil {
		return false
	}

	for _, ancestor := range pEntry.AncestryPath {
		if ancestor == container.(tview.Primitive) {
			return true
		}
	}
	return false
}
