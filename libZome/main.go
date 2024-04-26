package libzome

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"sync"
	"time"

	"github.com/jaketrock/zome/sharedInterfaces"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
)

type ResBody struct {
	Code   int `json:"code,omitempty"`
	Status any `json:"status,omitempty"`
}

type Zstate struct {
	ctx    context.Context
	host   host.Host
	topics map[string]*pubsub.Topic
}

//TODO: behavior:

// 1. initialize app to the topic
// 2. discover peers, use peerstore to find one where ID begins with your zome number(6char uuid)
// inherent security risk here, how to fix?
// 3. connect to that peer, exchange secure keys
// 4. send message only to that peer

func (z *Zstate) Initialize(topicName string) {
	flag.Parse()
	ctx := context.Background()

	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"))

	z.host = h
	z.ctx = ctx

	if err != nil {
		panic(err)
	}
	go z.discoverPeers(topicName)
}

func (z *Zstate) AddTopic(topicName string) {
	ps, err := pubsub.NewGossipSub(z.ctx, z.host)
	if err != nil {
		panic(err)
	}
	topic, err := ps.Join(topicName)
	if err != nil {
		panic(err)
	}

	sub, err := topic.Subscribe()
	if err != nil {
		panic(err)
	}
	z.printMessagesFrom(sub)
	z.topics[topicName] = topic
}

// TODO: get better data here, how do I know it's a server etc
func (z *Zstate) ListPeers(topic string) []string {
	//get all topics for host
	var peers []peer.ID
	if topic == "" {
		peers = z.host.Peerstore().Peers()
	} else {
		topics, ok := z.topics[topic]
		if !ok {
			fmt.Println(z.topics)
			// handle error: no such topic
			return nil
		}
		peers = topics.ListPeers()
	}
	peerList := make([]string, len(peers))
	for i, peer := range peers {
		peerList[i] = peer.String()
	}
	return peerList
}

type OpType int

const (
	ControlOp OpType = iota
	UploadOp
	DownloadOp
)

var ctrSocketType = protocol.ID("/zomeCtrSocket/1.0.0")
var dlSocketType = protocol.ID("/zomeDlSocket/1.0.0")
var ulSocketType = protocol.ID("/zomeUlSocket/1.0.0")

func (z *Zstate) ConnectServer(topic, peerId string, optype OpType) any {
	// find peer by ID
	if _, ok := z.topics[topic]; !ok {
		fmt.Println("Topic not found") //TODO: switch out for logger
		return nil
	}

	peers := z.host.Peerstore().Peers()
	var peerAddr peer.ID
	for _, peer := range peers {
		if peer.String() == peerId {
			peerAddr = peer
		}
	}
	peerinfo := z.host.Peerstore().PeerInfo(peerAddr)

	switch optype {
	// ADmin routes
	case ControlOp:
		// open stream to peer
		stream, err := z.host.NewStream(z.ctx, peerinfo.ID, ctrSocketType)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		return ZomeController{stream}
	case UploadOp:
		// open stream to peer
		stream, err := z.host.NewStream(z.ctx, peerinfo.ID, ulSocketType)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		return ZomeUploader{stream} //TODO: how to make em all extend one type
	case DownloadOp:
		// open stream to peer
		stream, err := z.host.NewStream(z.ctx, peerinfo.ID, dlSocketType)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		return ZomeDownloader{stream}
	default:
		fmt.Println("Improper operation type")
		return nil
	}
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

func (z *Zstate) discoverPeers(topicFlag string) {
	kademliaDHT := z.initDHT()
	routingDiscovery := drouting.NewRoutingDiscovery(kademliaDHT)
	dutil.Advertise(z.ctx, routingDiscovery, topicFlag)

	// Look for others who have announced and attempt to connect to them
	anyConnected := false
	for !anyConnected {
		fmt.Println("Searching for peers...")
		peerChan, err := routingDiscovery.FindPeers(z.ctx, topicFlag)
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

func (z *Zstate) SelfId(topic string) string {
	return string(z.host.ID())
}

func (z *Zstate) Close() {
	for _, topic := range z.topics {
		topic.Close()
	}
	z.host.Close()
}

func genericRequest(ns network.Stream, routeName string, inputJson interface{}) (interface{}, error) {
	rqJson := sharedInterfaces.Request{
		Action: routeName,
		Data:   inputJson,
	}
	//json to bytes
	rqBytes, err := json.Marshal(rqJson)
	if err != nil {
		return nil, err
	}
	_, err = ns.Write(rqBytes)
	if err != nil {
		return nil, err
	}
	unmarshalledResponse := struct {
		Code   int         `json:"code"`
		Status interface{} `json:"status"`
	}{}
	var sResp []byte
	_, err = ns.Read(sResp)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(sResp, &unmarshalledResponse)
	if err != nil {
		return nil, err
	}
	if unmarshalledResponse.Code != 200 {
		if status, ok := unmarshalledResponse.Status.(map[string]interface{}); ok && status["Error"] != nil {
			return nil, fmt.Errorf("error %d: %v", unmarshalledResponse.Code, status["Error"])
		} else {
			return nil, fmt.Errorf("error %d: %v", unmarshalledResponse.Code, unmarshalledResponse.Status)
		}
	}
	return unmarshalledResponse.Status, nil
}

var genericTypeError = fmt.Errorf("invalid response type")
