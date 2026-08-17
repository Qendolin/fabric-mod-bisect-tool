package util

import "github.com/rivo/tview"

func IsTextInput(p tview.Primitive) bool {
	switch p.(type) {
	case *tview.InputField, *tview.TextArea:
		return true
	default:
		return false
	}
}
