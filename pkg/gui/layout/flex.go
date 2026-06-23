package layout

import "fyne.io/fyne/v2"

type FlexProp struct {
	Grow  float32 // e.g., 1 or 2 (like flex-grow). 0 means fixed/MinSize.
	Fixed float32 // Exact pixel size. If 0, uses the object's MinSize.
}

type FlexLayout struct {
	Horizontal bool
	Gap        float32
	Props      map[fyne.CanvasObject]FlexProp
}

func NewFlexLayout(horizontal bool, gap float32) *FlexLayout {
	return &FlexLayout{
		Horizontal: horizontal,
		Gap:        gap,
		Props:      make(map[fyne.CanvasObject]FlexProp),
	}
}

// Set configures how a specific object behaves in the flex layout
func (l *FlexLayout) Set(obj fyne.CanvasObject, grow, fixed float32) {
	l.Props[obj] = FlexProp{Grow: grow, Fixed: fixed}
}

func (l *FlexLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var minW, minH float32
	var count int

	for _, obj := range objects {
		if !obj.Visible() {
			continue
		}
		count++
		prop := l.Props[obj]
		objMin := obj.MinSize()

		main, cross := objMin.Width, objMin.Height
		if !l.Horizontal {
			main, cross = objMin.Height, objMin.Width
		}

		if prop.Fixed > 0 {
			main = prop.Fixed
		}

		if l.Horizontal {
			minW += main
			if cross > minH {
				minH = cross
			}
		} else {
			minH += main
			if cross > minW {
				minW = cross
			}
		}
	}

	if count > 1 {
		if l.Horizontal {
			minW += float32(count-1) * l.Gap
		} else {
			minH += float32(count-1) * l.Gap
		}
	}
	return fyne.NewSize(minW, minH)
}

func (l *FlexLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	var totalFixed, totalGrow float32
	var count int

	// First pass: Calculate fixed space and total fraction
	for _, obj := range objects {
		if !obj.Visible() {
			continue
		}
		count++
		prop := l.Props[obj]
		if prop.Grow > 0 {
			totalGrow += prop.Grow
		} else {
			if prop.Fixed > 0 {
				totalFixed += prop.Fixed
			} else {
				if l.Horizontal {
					totalFixed += obj.MinSize().Width
				} else {
					totalFixed += obj.MinSize().Height
				}
			}
		}
	}

	gaps := float32(0)
	if count > 1 {
		gaps = float32(count-1) * l.Gap
	}

	available := size.Width
	if !l.Horizontal {
		available = size.Height
	}
	available -= (totalFixed + gaps)
	if available < 0 {
		available = 0
	}

	// Second pass: Position and size
	var pos float32 = 0
	for _, obj := range objects {
		if !obj.Visible() {
			continue
		}
		prop := l.Props[obj]
		var itemMain float32

		if prop.Grow > 0 {
			if totalGrow > 0 {
				itemMain = available * (prop.Grow / totalGrow)
			}
		} else if prop.Fixed > 0 {
			itemMain = prop.Fixed
		} else {
			if l.Horizontal {
				itemMain = obj.MinSize().Width
			} else {
				itemMain = obj.MinSize().Height
			}
		}

		if l.Horizontal {
			obj.Resize(fyne.NewSize(itemMain, size.Height))
			obj.Move(fyne.NewPos(pos, 0))
		} else {
			obj.Resize(fyne.NewSize(size.Width, itemMain))
			obj.Move(fyne.NewPos(0, pos))
		}
		pos += itemMain + l.Gap
	}
}
