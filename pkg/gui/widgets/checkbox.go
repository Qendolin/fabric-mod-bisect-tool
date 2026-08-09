package widgets

import (
	"image"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// CustomCheckBox renders a Material checkbox icon alongside custom content.
// The entire row is clickable via material.Clickable, so the ink ripple
// expands outward from the tap position and the label can be clicked too.
func CustomCheckBox(gtx layout.Context, th *material.Theme, check *widget.Bool, click *widget.Clickable, content layout.Widget) layout.Dimensions {
	// Toggle the boolean state when clicked.
	if click.Clicked(gtx) {
		check.Value = !check.Value
	}

	// material.Clickable wraps the child layout with ink ripple and hover effects.
	return material.Clickable(gtx, click, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				icon := th.Icon.CheckBoxUnchecked
				iconColor := th.Palette.Fg
				if check.Value {
					icon = th.Icon.CheckBoxChecked
					iconColor = th.Palette.ContrastBg
				}

				size := gtx.Dp(unit.Dp(24))
				gtx.Constraints.Min = image.Pt(size, size)
				return icon.Layout(gtx, iconColor)
			}),

			layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),

			layout.Flexed(1, content),
		)
	})
}
