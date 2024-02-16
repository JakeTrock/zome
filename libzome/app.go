package libzome

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"time"

	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/group/edwards25519"
	encoder "go.dedis.ch/kyber/v3/util/encoding"
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
	//TODO: selectively start up the abilities based on the config
	a.DbInit(a.Overrides)
	a.FsLoadConfig(a.Overrides)
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
			err := a.PeerRoom.PublishCrypt(input, goodKeysList)
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
			peerStr := make([]string, len(peerRaw))
			for i, p := range peerRaw {
				peerStr[i] = p.String()
			}
			fmt.Println("peersJson", peerStr)

			novelPeers := []string{}
			for _, p := range peerStr {
				_, ok := a.globalConfig.knownKeypairs[p]
				if !ok {
					novelPeers = append(novelPeers, p)
				}
			}
			if len(novelPeers) == 0 {
				continue
			}
			//create pubkeys from peerStr
			suite := edwards25519.NewBlakeSHA256Ed25519()
			newPubKeys := make(map[string]kyber.Point)
			for _, p := range novelPeers {
				publicKeyBytes, err := encoder.StringHexToPoint(suite, p)
				if err == nil {
					newPubKeys[p] = publicKeyBytes
				}
			}
			for k, v := range newPubKeys {
				a.globalConfig.knownKeypairs[k] = PeerState{key: v, approved: true} //TODO: URGENT this should be false as soon as key whitelist implemented
			}
			a.FsSaveConfig()

			// runtime.EventsEmit(ctx, "system-peers", peerStr) //TODO: restify this

		case <-a.PeerRoom.ctx.Done():
			return
		}
	}
}

//https://archive.org/details/youtube-l4Kijuav3ts
