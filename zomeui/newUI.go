package zomeui

import (
	"image/color"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (st *UiState) InitUi(w *app.Window) error { //TODO: move to folder https://github.com/planetdecred/godcr/tree/master/ui
	if st.app.peerRoom == nil {
		panic("App not initialized")
	}

	// title a.peerRoom.roomName
	// you a.peerRoom.nick

	th := st.uiStyle

	var ops op.Ops
	for {
		switch e := w.NextEvent().(type) {
		case system.DestroyEvent:
			return e.Err
		case system.FrameEvent:
			// This graphics context is used for managing the rendering state.
			gtx := layout.NewContext(&ops, e)

			// Define an large label with an appropriate text:
			title := material.H1(th, "Zome V0.1")

			// Change the color of the label.
			maroon := color.NRGBA{R: 127, G: 0, B: 0, A: 255}
			title.Color = maroon

			// Change the position of the label.
			title.Alignment = text.Middle

			// Draw the label to the graphics context.
			title.Layout(gtx)

			// create a text box to contain our chat messages
			msgBox := material.Editor(th, new(widget.Editor), "Room: "+a.peerRoom.roomName)
			msgBox.Layout(gtx)

			// Define a button with the label "Click me".
			// btn := material.Button(th, new(widget.Clickable), "Click me")
			//make button small

			// Draw the button to the graphics context.
			// btn.Layout(gtx)

			// Pass the drawing operations to the GPU.
			e.Frame(gtx.Ops)
		}
	}
}
