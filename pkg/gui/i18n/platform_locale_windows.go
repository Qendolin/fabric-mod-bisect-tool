//go:build windows

package i18n

import "golang.org/x/sys/windows"

// platformLocale returns the first language in the Windows user's preferred
// UI language list. Unlike environment variables, this is available for GUI
// applications launched from Explorer.
func platformLocale() string {
	languages, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil || len(languages) == 0 {
		return ""
	}
	return languages[0]
}
