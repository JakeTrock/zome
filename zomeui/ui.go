package zomeui

import (
	"runtime"

	libzome "zome/libzome"

	"gioui.org/widget/material"
)

type Platform int

const (
	PlatformUnknown Platform = iota
	PlatformAndroid
	PlatformLinsux
	PlatformCrapple
	PlatformWinblows
)

type UiState struct {
	app      *libzome.App
	uiStyle  *material.Theme
	platform Platform
}

// https://github.com/psanford/wormhole-william-mobile/tree/main
func NewUi(app *libzome.App) *UiState {
	theme := material.NewTheme()

	platform := PlatformUnknown
	//get platform type
	if runtime.GOOS == "android" {
		platform = PlatformAndroid
	} else if runtime.GOOS == "linux" {
		platform = PlatformLinsux
	} else if runtime.GOOS == "darwin" {
		platform = PlatformCrapple
	} else if runtime.GOOS == "windows" {
		platform = PlatformWinblows
	}

	return &UiState{
		app:      app,
		uiStyle:  theme,
		platform: platform,
	}
}

// func NewTheme() *Theme {//TODO: custom theme
// 	t := &Theme{Shaper: &text.Shaper{}}
// 	t.Palette = Palette{
// 		Fg:         rgb(0x000000),
// 		Bg:         rgb(0xffffff),
// 		ContrastBg: rgb(0x3f51b5),
// 		ContrastFg: rgb(0xffffff),
// 	}
// 	t.TextSize = 16

// 	t.Icon.CheckBoxChecked = mustIcon(widget.NewIcon(icons.ToggleCheckBox))
// 	t.Icon.CheckBoxUnchecked = mustIcon(widget.NewIcon(icons.ToggleCheckBoxOutlineBlank))
// 	t.Icon.RadioChecked = mustIcon(widget.NewIcon(icons.ToggleRadioButtonChecked))
// 	t.Icon.RadioUnchecked = mustIcon(widget.NewIcon(icons.ToggleRadioButtonUnchecked))

// 	// 38dp is on the lower end of possible finger size.
// 	t.FingerSize = 38

// 	return t
// }
