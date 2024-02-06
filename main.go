package main

import (
	"embed"
	"flag"
	"fmt"

	libzome "zome/libzome"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

func main() {
	zomeBackend := libzome.NewApp()

	zomeBackend.Count() //TODO: removeme

	deviceIdOverride := flag.String("overrideId", "", "overrides default device id")
	poolIdOverride := flag.String("overridePool", "", "overrides default pool id")
	help := flag.Bool("h", false, "Display Help")
	flag.Parse()
	if *help {
		fmt.Println("zome under construction")
		fmt.Println()
		flag.PrintDefaults()
		return
	}

	// zomeBackend.InitDb()

	zomeBackend.LoadConfig(map[string]string{
		"uuid":   *deviceIdOverride,
		"poolId": *poolIdOverride,
	})

	//TODO: add back signals api

	//BEGIN OLDPART

	// Create an instance of the app structure
	// app := NewApp()
	// app.Run()
	// Create application with options
	err := wails.Run(&options.App{
		Title:            "wails-events",
		Width:            1024,
		Height:           768,
		Assets:           assets,
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        zomeBackend.Startup,
		Bind: []interface{}{
			zomeBackend,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
