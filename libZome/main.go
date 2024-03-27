package libzome

import (
	"context"
	"flag"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
)

var (
	topicNameFlag = flag.String("topicName", "crossPlatTopic", "name of topic to join")
)

type Zstate struct {
	ctx  context.Context
	host host.Host
}

//TODO: behavior:

// 1. initialize app to the topic
// 2. discover peers, use peerstore to find one where ID begins with your zome number(6char uuid)
// inherent security risk here, how to fix?
// 3. connect to that peer, exchange secure keys
// 4. send message only to that peer

func (z *Zstate) initialize() {
	flag.Parse()
	ctx := context.Background()

	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"))

	z.host = h
	z.ctx = ctx

	if err != nil {
		panic(err)
	}
	go z.discoverPeers()

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		panic(err)
	}
	topic, err := ps.Join(*topicNameFlag)
	if err != nil {
		panic(err)
	}
	go z.streamConsoleTo(topic)

	sub, err := topic.Subscribe()
	if err != nil {
		panic(err)
	}
	z.printMessagesFrom(sub)
}

func (z *Zstate) initDHT() *dht.IpfsDHT {
	// Start a DHT, for use in peer discovery. We can't just make a new DHT
	// client because we want each peer to maintain its own local copy of the
	// DHT, so that the bootstrapping node of the DHT can go down without
	// inhibiting future peer discovery.
	kademliaDHT, err := dht.New(z.ctx, z.host)
	if err != nil {
		panic(err)
	}
	if err = kademliaDHT.Bootstrap(z.ctx); err != nil {
		panic(err)
	}
	var wg sync.WaitGroup
	for _, peerAddr := range dht.DefaultBootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := z.host.Connect(z.ctx, *peerinfo); err != nil {
				fmt.Println("Bootstrap warning:", err)
			}
		}()
	}
	wg.Wait()

	return kademliaDHT
}

func (z *Zstate) discoverPeers() {
	kademliaDHT := z.initDHT()
	routingDiscovery := drouting.NewRoutingDiscovery(kademliaDHT)
	dutil.Advertise(z.ctx, routingDiscovery, *topicNameFlag)

	// Look for others who have announced and attempt to connect to them
	anyConnected := false
	for !anyConnected {
		fmt.Println("Searching for peers...")
		peerChan, err := routingDiscovery.FindPeers(z.ctx, *topicNameFlag)
		if err != nil {
			panic(err)
		}
		for peer := range peerChan {
			if peer.ID == z.host.ID() {
				continue // No self connection
			}
			err := z.host.Connect(z.ctx, peer)
			if err != nil {
				fmt.Printf("Failed connecting to %s, error: %s\n", peer.ID, err)
			} else {
				fmt.Println("Connected to:", peer.ID)
				anyConnected = true
			}
		}
	}
	fmt.Println("Peer discovery complete")
}

func (z *Zstate) streamConsoleTo(topic *pubsub.Topic) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-z.ctx.Done():
			return
		case <-ticker.C:
			println("SENDping")
			if err := topic.Publish(z.ctx, []byte("ping")); err != nil {
				fmt.Println("### Publish error:", err)
			}
		}
	}
}

func (z *Zstate) printMessagesFrom(sub *pubsub.Subscription) {
	for {
		m, err := sub.Next(z.ctx)
		if err != nil {
			panic(err)
		}
		fmt.Println(m.ReceivedFrom, ": ", string(m.Message.Data))
	}
}
