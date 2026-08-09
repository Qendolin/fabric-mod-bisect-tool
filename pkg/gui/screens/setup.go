package screens

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/gui/probe"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/gui/theme"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
	"github.com/ncruces/zenity"
)

type SetupScreen struct {
	app         App
	pathEditor  widget.Editor
	browseClick widget.Clickable
	startClick  widget.Clickable
}

func NewSetupScreen(app App) *SetupScreen {
	s := &SetupScreen{app: app}
	s.pathEditor.SingleLine = true
	s.pathEditor.Submit = true
	return s
}

func (s *SetupScreen) Layout(gtx layout.Context, th *material.Theme) layout.Dimensions {
	// Process button clicks
	if s.browseClick.Clicked(gtx) {
		initial := s.pathEditor.Text()
		go func() {
			opts := []zenity.Option{
				zenity.Title("Select Mods Folder"),
				zenity.Directory(),
				zenity.Modal(),
				zenity.Filename(initial),
			}
			if id := s.app.WindowAttachID(); id != nil {
				opts = append(opts, zenity.Attach(id))
			}
			path, err := zenity.SelectFile(opts...)
			if err == nil && path != "" {
				s.app.Run(func() {
					s.pathEditor.SetText(path)
				})
			}
		}()
	}

	if s.startClick.Clicked(gtx) {
		path := s.pathEditor.Text()
		if path == "" {
			// ShowErrorDialog blocks on a native dialog; do not run it on the
			// gio frame goroutine.
			go func() {
				defer logging.HandlePanic()
				s.app.ShowErrorDialog("Error", "Please select a mods folder", nil)
			}()
		} else {
			go func() {
				res := probe.ProbeModsDirectory(path)
				s.app.Run(func() {
					vm := s.app.GetViewModel()
					s.app.StartLoadingProcess(path, vm.ForceQuiltSupport || res.QuiltSupport, vm.ForceNeoForgeSupport || res.NeoForgeSupport)
				})
			}()
		}
	}

	// Centered layout configuration
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, layout.Spacer{}.Layout), // top spacer
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// Center the form horizontally with a maximum width of 480dp
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					w := gtx.Dp(540)
					if w > gtx.Constraints.Max.X {
						w = gtx.Constraints.Max.X
					}
					gtx.Constraints.Min.X = w
					gtx.Constraints.Max.X = w

					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
						// Title
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							title := material.H4(th, "Mod Bisect Tool")
							title.Color = theme.PrimaryColor
							title.Alignment = text.Middle
							title.Font.Weight = font.Bold
							return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, title.Layout)
						}),
						// Instruction
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							desc := material.Body1(th, "Select your mods folder to begin.")
							desc.Color = theme.TextMutedColor
							desc.Alignment = text.Middle
							return layout.Inset{Bottom: unit.Dp(32)}.Layout(gtx, desc.Layout)
						}),
						// Path entry and browse button
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									// Editor wrapped in a styled container
									ed := material.Editor(th, &s.pathEditor, "Enter or browse mods folder path...")
									ed.TextSize = unit.Sp(12)
									ed.Color = theme.FgColor
									ed.HintColor = theme.TextMutedColor
									border := widget.Border{
										Color:        theme.BorderColor,
										CornerRadius: unit.Dp(4),
										Width:        unit.Dp(1),
									}
									return border.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(unit.Dp(10)).Layout(gtx, ed.Layout)
									})
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									btn := material.Button(th, &s.browseClick, "Browse...")
									btn.Background = theme.CardBgColor
									btn.Color = theme.FgColor
									return btn.Layout(gtx)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
						// Start Button
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							btn := material.Button(th, &s.startClick, "Start Bisection")
							btn.Background = theme.PrimaryColor
							btn.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
							btn.TextSize = unit.Sp(16)
							btn.Inset = layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12)}
							return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
								layout.Flexed(1, btn.Layout),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							desc := material.Body1(th, "by Qendolin")
							desc.Color = theme.TextMutedColor
							desc.Alignment = text.Middle
							desc.TextSize = unit.Sp(10)
							return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, desc.Layout)
						}),
					)
				}),
				layout.Flexed(1, layout.Spacer{}.Layout),
			)
		}),
		layout.Flexed(1.2, layout.Spacer{}.Layout), // visually balanced slightly upward
	)
}
