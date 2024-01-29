package libzome

import (
	"context"
	"flag"
	"fmt"

	"github.com/ipfs/go-log/v2"
)

//https://github.com/libp2p/go-libp2p/tree/master/examples/chat-with-rendezvous

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// TODO: revise the below into helper fns
var logger = log.Logger("zome")

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	log.SetAllLoggers(log.LevelWarn)
	log.SetLogLevel("zome", "info")
	help := flag.Bool("h", false, "Display Help")
	config, err := ParseFlags()
	if err != nil {
		panic(err)
	}

	if *help {
		fmt.Println("zome under construction")
		fmt.Println()
		flag.PrintDefaults()
		return
	}

	initP2P(config)
	a.loadConfig()

}

// shutdown is called when the app is shutting down
func (a *App) Shutdown() {
	//todo: send save config signal
	a.terminateConnection()
}

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}

//https://archive.org/details/youtube-l4Kijuav3ts
