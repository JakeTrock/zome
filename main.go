package main

import (
	"log"
	"os"

	"flag"
	"fmt"

	"gioui.org/app"
	libzome "github.com/jaketrock/zome/libzome"
	zomeui "github.com/jaketrock/zome/zomeui"
)

func main() {
	go func() {
		//backend
		zomeBackend := libzome.NewApp()
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

		zomeBackend.InitP2P()

		zomeFrontend := zomeui.NewUi(zomeBackend)

		//visual
		w := app.NewWindow()
		err := zomeui.InitUi(w)
		if err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}
