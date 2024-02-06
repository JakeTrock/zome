package libzome

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//https://github.com/libp2p/go-libp2p/tree/master/examples/chat-with-mdns

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

func (a *App) Startup(ctx context.Context) {
	// Perform your setup here
	a.ctx = ctx
	a.InitP2P(ctx)

	a.HandleEvents(ctx)
}

func (a *App) Count() { //TODO: removeme
	go func() {
		count := 1

		for {
			if a.ctx == nil {
				log.Printf("ctx is nil")
				time.Sleep(1 * time.Second)
				continue
			}

			log.Printf("emitting count: %v", count)
			runtime.EventsEmit(a.ctx, "count", count)
			time.Sleep(1 * time.Second)
			count += 1
		}
	}()
}

// main event handler
func (a *App) HandleEvents(ctx context.Context) {
	peerRefreshTicker := time.NewTicker(time.Second)
	defer peerRefreshTicker.Stop()

	for {
		select {
		case input := <-a.peerRoom.inputCh:
			// when the user types in a line, publish it to the chat room and print to the message window
			err := a.peerRoom.Publish(input)
			if err != nil {
				log.Fatal(err)
			}

		case m := <-a.peerRoom.Messages:
			// when we receive a message from the chat room, print it to the message window
			fmt.Println("msgJson", m)
			mJSON, err := json.Marshal(m)
			if err != nil {
				log.Fatal(err)
			}
			mStr := string(mJSON)
			fmt.Println("msgJson", mStr)
			runtime.EventsEmit(ctx, "system-message", m)

		case <-peerRefreshTicker.C:
			// refresh the list of peers in the chat room periodically
			peerRaw := a.peerRoom.ListPeers()
			peerStr := make([]string, len(peerRaw))
			for i, p := range peerRaw {
				peerStr[i] = p.String()
			}
			fmt.Println("peersJson", peerStr)
			runtime.EventsEmit(ctx, "system-peers", peerStr)

		case <-a.peerRoom.ctx.Done():
			return
		}
	}
}

//https://archive.org/details/youtube-l4Kijuav3ts
