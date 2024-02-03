package main

import (
	"image/color"
	"log"
	"os"

	"flag"
	"fmt"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/widget/material"
	libzome "github.com/jaketrock/zome/libzome"
)

func main() {
	go func() {
		//backend
		zomeBackend := libzome.NewApp()
		help := flag.Bool("h", false, "Display Help")
		config := libzome.ParseFlags()
		if *help {
			fmt.Println("zome under construction")
			fmt.Println()
			flag.PrintDefaults()
			return
		}
		zomeBackend.LoadConfig()

		zomeBackend.InitDb()

		naiveMessageDictionary := make(map[string]string)

		zomeBackend.InitP2P(config)

		//visual
		w := app.NewWindow()
		err := run(w)
		if err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(w *app.Window) error { //TODO: move to folder https://github.com/planetdecred/godcr/tree/master/ui
	th := material.NewTheme()
	var ops op.Ops
	for {
		switch e := w.NextEvent().(type) {
		case system.DestroyEvent:
			return e.Err
		case system.FrameEvent:
			// This graphics context is used for managing the rendering state.
			gtx := layout.NewContext(&ops, e)

			// Define an large label with an appropriate text:
			title := material.H1(th, "Hello, Gio")

			// Change the color of the label.
			maroon := color.NRGBA{R: 127, G: 0, B: 0, A: 255}
			title.Color = maroon

			// Change the position of the label.
			title.Alignment = text.Middle

			// Draw the label to the graphics context.
			title.Layout(gtx)

			// Pass the drawing operations to the GPU.
			e.Frame(gtx.Ops)
		}
	}
}
