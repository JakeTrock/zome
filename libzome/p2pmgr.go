package libzome

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

// DiscoveryInterval is how often we re-publish our mDNS records.
const DiscoveryInterval = time.Hour

// DiscoveryServiceTag is used in our mDNS advertisements to discover other peer peers.
const DiscoveryServiceTag = "libzome-sync"

func (a *App) p2pInit(appContext context.Context) {

	if a.globalConfig.uuid == "" || a.globalConfig.poolId == "" {
		log.Fatal("App not initialized")
	}

	ctx := context.Background()

	// create a new libp2p Host that listens on a random TCP port
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"))
	if err != nil {
		log.Fatal(err)
	}

	// create a new PubSub service using the GossipSub router
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		panic(err)
	}

	// setup local mDNS discovery
	if err := setupDiscovery(h); err != nil {
		panic(err)
	}

	// use the nickname from the cli flag, or a default if blank
	nick := a.globalConfig.userName

	// join the room from the cli flag, or the flag default
	room := a.globalConfig.poolId

	fmt.Println("nickname:", nick)
	fmt.Println("room:", room)

	// join the peer room
	cr, err := JoinPeerRoom(ctx, ps, h.ID(), nick, room)
	if err != nil {
		panic(err)
	}

	fmt.Printf("joined peer room %s as %s\n", room, nick)
	a.PeerRoom = cr

}

func (a *App) P2PPushMessage(message string, appId string) {
	sendObject := PeerMessagePre{
		Message: message,
		AppId:   appId,
	}

	fmt.Println("frontend-message", string(sendObject.Message), string(sendObject.AppId))
	a.PeerRoom.inputCh <- sendObject
}

func (a *App) P2PGetPeers() []string {
	peerIDs := a.PeerRoom.ListPeers()
	peers := make([]string, len(peerIDs))
	for i, id := range peerIDs {
		peers[i] = id.String() //TODO: you can check this agains public keys and get the safety
	}
	return peers
}

// discoveryNotifee gets notified when we find a new peer via mDNS discovery
type discoveryNotifee struct {
	h host.Host
}

// HandlePeerFound connects to peers discovered via mDNS. Once they're connected,
// the PubSub system will automatically start interacting with them if they also
// support PubSub.
func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	fmt.Printf("discovered new peer %s\n", pi.ID)
	err := n.h.Connect(context.Background(), pi)
	if err != nil {
		fmt.Printf("error connecting to peer %s: %s\n", pi.ID, err)
	}
}

// setupDiscovery creates an mDNS discovery service and attaches it to the libp2p Host.
// This lets us automatically discover peers on the same LAN and connect to them.
func setupDiscovery(h host.Host) error {
	// setup mDNS discovery to find local peers
	s := mdns.NewMdnsService(h, DiscoveryServiceTag, &discoveryNotifee{h: h})
	return s.Start()
}
