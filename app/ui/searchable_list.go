package ui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// SearchableList is a custom widget that combines an input field for searching
// with a list that displays filtered results. It implements Focusable.
type SearchableList struct {
	*tview.Flex
	list        *tview.List
	searchField *tview.InputField
	allItems    []string
}

// NewSearchableList creates a new SearchableList.
func NewSearchableList() *SearchableList {
	s := &SearchableList{
		Flex:        tview.NewFlex().SetDirection(tview.FlexRow),
		list:        tview.NewList().ShowSecondaryText(false),
		searchField: tview.NewInputField().SetPlaceholder("Search..."),
		allItems:    []string{},
	}

	s.list.SetBorder(false)

	s.AddItem(s.searchField, 1, 0, true).
		AddItem(s.list, 0, 1, false)

	s.searchField.SetChangedFunc(func(text string) {
		s.filter(text)
	})

	searchFocusedStyle := s.searchField.GetFieldStyle().Foreground(tcell.ColorBlack)
	searchBlurredStyle := searchFocusedStyle.Background(tcell.ColorDarkSlateGray)

	s.searchField.SetFocusFunc(func() {
		s.searchField.SetFieldStyle(searchFocusedStyle)
		s.searchField.SetPlaceholderStyle(searchFocusedStyle)
		s.updateFocusWithin()
	})

	s.searchField.SetBlurFunc(func() {
		s.searchField.SetFieldStyle(searchBlurredStyle)
		s.searchField.SetPlaceholderStyle(searchBlurredStyle)
		s.updateFocusWithin()
	})
	s.searchField.Blur()

	s.list.SetFocusFunc(func() {
		s.updateFocusWithin()
		s.list.SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorBlue))
	})

	s.list.SetBlurFunc(func() {
		s.updateFocusWithin()
		s.list.SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
	})
	s.list.Blur()

	s.updateFocusWithin()

	return s
}

func (s *SearchableList) updateFocusWithin() {
	if s.HasFocus() {
		s.list.SetMainTextColor(tcell.ColorWhite)
	} else {
		s.list.SetMainTextColor(tcell.ColorLightGray)
	}
}

func (s *SearchableList) Blur() {
	s.Flex.Blur()
	s.updateFocusWithin()
}

// GetFocusablePrimitives implements the Focusable interface.
func (s *SearchableList) GetFocusablePrimitives() []tview.Primitive {
	return []tview.Primitive{s.searchField, s.list}
}

// Focus delegates focus to the search field by default.
func (s *SearchableList) Focus(delegate func(p tview.Primitive)) {
	s.searchField.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			if s.list.GetItemCount() > 0 {
				delegate(s.list)
			}
		}
	})
	delegate(s.searchField)
	s.updateFocusWithin()
}

// SetItems clears the list and sets new items.
func (s *SearchableList) SetItems(items []string) {
	s.allItems = items
	s.filter(s.searchField.GetText())
}

// filter updates the list to show only items matching the query.
func (s *SearchableList) filter(query string) {
	s.list.Clear()
	query = strings.ToLower(query)
	for _, item := range s.allItems {
		if query == "" || strings.Contains(strings.ToLower(item), query) {
			s.list.AddItem(item, "", 0, nil)
		}
	}
}
