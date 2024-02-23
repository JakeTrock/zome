package libzome

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"time"
)

//https://github.com/libp2p/go-libp2p/tree/master/examples/peer-with-mdns

// NewApp creates a new App application struct
func NewApp(overrides map[string]string) *App {
	return &App{
		Overrides: overrides,
	}
}

func (a *App) Startup(ctx context.Context) {
	// Perform your setup here
	a.ctx = ctx
	a.Abilities = []string{"database", "p2p", "encryption", "fs"} //TODO: in the future, all of these should be pluggable

	a.FsLoadConfig(a.Overrides)

	a.DbInit(a.Overrides)

	a.p2pInit(ctx)

	a.HandleEvents(ctx)
}

// main event handler
func (a *App) HandleEvents(ctx context.Context) {
	peerRefreshTicker := time.NewTicker(time.Second)
	defer peerRefreshTicker.Stop()

	for {
		select {
		case input := <-a.PeerRoom.inputCh:
			// when the user inputs, publish it to the peer room and print to the message window
			goodKeysList := []func(totalLen int, readWrite bufio.ReadWriter) error{}
			for k, v := range a.globalConfig.knownKeypairs {
				if v.approved {
					goodKeysList = append(goodKeysList, func(totalLen int, readWrite bufio.ReadWriter) error {
						return a.EcEncrypt(k, totalLen, readWrite)
					})
				}
			}
			err := a.PeerRoom.Publish(input)
			if err != nil {
				log.Fatal(err)
			}

		// case m := <-a.PeerRoom.Messages:
		// 	// when we receive a message from the peer room, print it to the message window
		// 	fmt.Println("msgJson", m)
		// 	mJSON, err := json.Marshal(m)
		// 	if err != nil {
		// 		log.Fatal(err)
		// 	}
		// 	mStr := string(mJSON)
		// 	fmt.Println("msgJson", mStr)
		// 	runtime.EventsEmit(ctx, "system-message", m)

		case <-peerRefreshTicker.C:
			// refresh the list of peers in the peer room periodically
			peerRaw := a.PeerRoom.ListPeers()
			peerStr := make([]string, len(peerRaw)) //TODO: peer access control?
			for i, p := range peerRaw {
				peerStr[i] = p.String()
			}
			fmt.Println("peersJson", peerStr)

			// a.FsSaveConfig() //TODO: make this save when it changes

			// runtime.EventsEmit(ctx, "system-peers", peerStr) //TODO: restify this

		case <-a.PeerRoom.ctx.Done():
			return
		}
	}
}

//https://archive.org/details/youtube-l4Kijuav3ts
