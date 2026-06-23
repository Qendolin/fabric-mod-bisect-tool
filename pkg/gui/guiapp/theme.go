package guiapp

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type CustomTheme struct {
}

var _ fyne.Theme = (*CustomTheme)(nil)

func (t *CustomTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if variant == theme.VariantDark {
		switch name {
		// --- Core Structural Colors ---
		case theme.ColorNameBackground:
			return color.NRGBA{R: 26, G: 29, B: 36, A: 255} // Deep Slate
		case theme.ColorNameHeaderBackground:
			return color.NRGBA{R: 33, G: 37, B: 48, A: 255}
		case theme.ColorNameMenuBackground:
			return color.NRGBA{R: 31, G: 34, B: 43, A: 255}
		case theme.ColorNameOverlayBackground:
			return color.NRGBA{R: 29, G: 32, B: 41, A: 255}

		// --- Text & Typography ---
		case theme.ColorNameForeground:
			return color.NRGBA{R: 225, G: 228, B: 234, A: 255} // Soft White
		case theme.ColorNamePlaceHolder:
			return color.NRGBA{R: 107, G: 115, B: 138, A: 255}
		case theme.ColorNameDisabled:
			return color.NRGBA{R: 90, G: 98, B: 117, A: 255}
		case theme.ColorNameHyperlink:
			return color.NRGBA{R: 100, G: 181, B: 246, A: 255}

		// --- Interactive Elements (Buttons/Input) ---
		case theme.ColorNameButton:
			return color.NRGBA{R: 40, G: 44, B: 55, A: 255} // Slate Grey
		case theme.ColorNameDisabledButton:
			return color.NRGBA{R: 31, G: 34, B: 43, A: 255}
		case theme.ColorNameInputBackground:
			return color.NRGBA{R: 30, G: 33, B: 41, A: 255}
		case theme.ColorNameInputBorder:
			return color.NRGBA{R: 63, G: 68, B: 84, A: 255}

		// --- Interaction Overlays (Translucent so they blend on top of colors) ---
		case theme.ColorNameHover:
			return color.NRGBA{R: 255, G: 255, B: 255, A: 15} // 6% white overlay
		case theme.ColorNamePressed:
			return color.NRGBA{R: 255, G: 255, B: 255, A: 30} // 12% white overlay

		// --- Brand & Semantics ---
		case theme.ColorNamePrimary:
			return color.NRGBA{R: 76, G: 120, B: 230, A: 255} // Blue Accent
		case theme.ColorNameFocus:
			return color.NRGBA{R: 76, G: 120, B: 230, A: 255}
		case theme.ColorNameSelection:
			return color.NRGBA{R: 76, G: 120, B: 230, A: 60} // Semi-transparent blue
		case theme.ColorNameSuccess:
			return color.NRGBA{R: 36, G: 100, B: 58, A: 255} // Forest Green
		case theme.ColorNameWarning:
			return color.NRGBA{R: 255, G: 167, B: 38, A: 255} // Amber Orange
		case theme.ColorNameError:
			return color.NRGBA{R: 158, G: 40, B: 46, A: 255} // Crimson Red

		// --- Semantics Contrast Foreground (Fyne 2.5+) ---
		case theme.ColorNameForegroundOnPrimary:
			return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		case theme.ColorNameForegroundOnSuccess:
			return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		case theme.ColorNameForegroundOnWarning:
			return color.NRGBA{R: 26, G: 29, B: 36, A: 255} // Dark text on amber
		case theme.ColorNameForegroundOnError:
			return color.NRGBA{R: 255, G: 255, B: 255, A: 255}

		// --- Primary Palette Variants ---
		case theme.ColorRed:
			return color.NRGBA{R: 198, G: 40, B: 40, A: 255}
		case theme.ColorOrange:
			return color.NRGBA{R: 230, G: 81, B: 0, A: 255}
		case theme.ColorYellow:
			return color.NRGBA{R: 245, G: 124, B: 0, A: 255}
		case theme.ColorGreen:
			return color.NRGBA{R: 36, G: 100, B: 58, A: 255}
		case theme.ColorBlue:
			return color.NRGBA{R: 76, G: 120, B: 230, A: 255}
		case theme.ColorPurple:
			return color.NRGBA{R: 123, G: 31, B: 162, A: 255}
		case theme.ColorBrown:
			return color.NRGBA{R: 93, G: 64, B: 55, A: 255}
		case theme.ColorGray:
			return color.NRGBA{R: 90, G: 98, B: 117, A: 255}

		// --- Layout Utility Details ---
		case theme.ColorNameSeparator:
			return color.NRGBA{R: 45, G: 50, B: 62, A: 255}
		case theme.ColorNameShadow:
			return color.NRGBA{R: 0, G: 0, B: 0, A: 80}
		case theme.ColorNameScrollBar:
			return color.NRGBA{R: 90, G: 98, B: 117, A: 150}
		case theme.ColorNameScrollBarBackground:
			return color.NRGBA{R: 0, G: 0, B: 0, A: 0}
		}
	} else {
		// ==========================================
		// Light Mode Layout Configs
		// ==========================================
		switch name {
		// --- Core Structural Colors ---
		case theme.ColorNameBackground:
			return color.NRGBA{R: 255, G: 255, B: 255, A: 255} // Clean Pure White
		case theme.ColorNameHeaderBackground:
			return color.NRGBA{R: 243, G: 244, B: 246, A: 255}
		case theme.ColorNameMenuBackground:
			return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		case theme.ColorNameOverlayBackground:
			return color.NRGBA{R: 250, G: 250, B: 252, A: 255}

		// --- Text & Typography ---
		case theme.ColorNameForeground:
			return color.NRGBA{R: 30, G: 30, B: 36, A: 255} // Deep Charcoal
		case theme.ColorNamePlaceHolder:
			return color.NRGBA{R: 117, G: 117, B: 117, A: 255}
		case theme.ColorNameDisabled:
			return color.NRGBA{R: 158, G: 158, B: 158, A: 255}
		case theme.ColorNameHyperlink:
			return color.NRGBA{R: 21, G: 101, B: 192, A: 255}

		// --- Interactive Elements (Buttons/Input) ---
		case theme.ColorNameButton:
			return color.NRGBA{R: 208, G: 212, B: 220, A: 255} // Noticeable Darker Slate Grey (Sharp Contrast)
		case theme.ColorNameDisabledButton:
			return color.NRGBA{R: 235, G: 236, B: 240, A: 255}
		case theme.ColorNameInputBackground:
			return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		case theme.ColorNameInputBorder:
			return color.NRGBA{R: 190, G: 195, B: 205, A: 255}

		// --- Interaction Overlays (Translucent so they blend on top of colors) ---
		case theme.ColorNameHover:
			return color.NRGBA{R: 0, G: 0, B: 0, A: 15} // 6% black overlay
		case theme.ColorNamePressed:
			return color.NRGBA{R: 0, G: 0, B: 0, A: 30} // 12% black overlay

		// --- Brand & Semantics ---
		case theme.ColorNamePrimary:
			return color.NRGBA{R: 25, G: 118, B: 210, A: 255} // Pure Cobalt Blue
		case theme.ColorNameFocus:
			return color.NRGBA{R: 25, G: 118, B: 210, A: 255}
		case theme.ColorNameSelection:
			return color.NRGBA{R: 25, G: 118, B: 210, A: 50}
		case theme.ColorNameSuccess:
			return color.NRGBA{R: 46, G: 125, B: 50, A: 255} // Grass Green
		case theme.ColorNameWarning:
			return color.NRGBA{R: 245, G: 124, B: 0, A: 255} // Deep Amber
		case theme.ColorNameError:
			return color.NRGBA{R: 198, G: 40, B: 40, A: 255} // Pure Red

		// --- Semantics Contrast Foreground (Fyne 2.5+) ---
		case theme.ColorNameForegroundOnPrimary:
			return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		case theme.ColorNameForegroundOnSuccess:
			return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		case theme.ColorNameForegroundOnWarning:
			return color.NRGBA{R: 30, G: 30, B: 36, A: 255}
		case theme.ColorNameForegroundOnError:
			return color.NRGBA{R: 255, G: 255, B: 255, A: 255}

		// --- Primary Palette Variants ---
		case theme.ColorRed:
			return color.NRGBA{R: 198, G: 40, B: 40, A: 255}
		case theme.ColorOrange:
			return color.NRGBA{R: 245, G: 124, B: 0, A: 255}
		case theme.ColorYellow:
			return color.NRGBA{R: 251, G: 192, B: 45, A: 255}
		case theme.ColorGreen:
			return color.NRGBA{R: 46, G: 125, B: 50, A: 255}
		case theme.ColorBlue:
			return color.NRGBA{R: 25, G: 118, B: 210, A: 255}
		case theme.ColorPurple:
			return color.NRGBA{R: 106, G: 27, B: 154, A: 255}
		case theme.ColorBrown:
			return color.NRGBA{R: 78, G: 52, B: 46, A: 255}
		case theme.ColorGray:
			return color.NRGBA{R: 117, G: 117, B: 117, A: 255}

		// --- Layout Utility Details ---
		case theme.ColorNameSeparator:
			return color.NRGBA{R: 224, G: 224, B: 224, A: 255}
		case theme.ColorNameShadow:
			return color.NRGBA{R: 0, G: 0, B: 0, A: 20}
		case theme.ColorNameScrollBar:
			return color.NRGBA{R: 188, G: 188, B: 188, A: 150}
		case theme.ColorNameScrollBarBackground:
			return color.NRGBA{R: 0, G: 0, B: 0, A: 0}
		}
	}

	return theme.DefaultTheme().Color(name, variant)
}

func (t *CustomTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *CustomTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *CustomTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}
