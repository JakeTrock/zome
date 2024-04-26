package zcrypto

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jaketrock/zome/sharedInterfaces"
	"github.com/libp2p/go-libp2p-kad-dht/dual"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	ipfslite "github.com/hsanjuan/ipfs-lite"

	multiaddr "github.com/multiformats/go-multiaddr"
)

type Zstate struct {
	Ctx    context.Context
	Host   host.Host
	Dht    *dual.DHT
	Psub   *pubsub.PubSub
	topics map[string]*pubsub.Topic
}

type discoveryNotifee struct {
	h host.Host
}

func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	fmt.Printf("discovered new peer %s\n", pi.ID.String())
	err := n.h.Connect(context.Background(), pi)
	if err != nil {
		fmt.Printf("error connecting to peer %s: %s\n", pi.ID.String(), err)
	}
}

func (z *Zstate) InitP2P(privateKey crypto.PrivKey) error {
	listen, err := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/0")
	if err != nil {
		return err
	}

	// Bootstrappers are using 1024 keys. See:
	// https://github.com/ipfs/infra/issues/378

	h, dht, err := ipfslite.SetupLibp2p(
		z.Ctx,
		privateKey,
		nil,
		[]multiaddr.Multiaddr{listen},
		nil,
		ipfslite.Libp2pOptionsExtra...,
	)
	if err != nil {
		return err
	}

	// MDNS Discovery
	discoveryService := mdns.NewMdnsService(h, sharedInterfaces.AppNameSpace, &discoveryNotifee{h: h})
	err = discoveryService.Start()
	if err != nil {
		return err
	}
	defer discoveryService.Close()
	//=====

	psub, err := pubsub.NewGossipSub(z.Ctx, h)
	if err != nil {
		return err
	}

	z.Host = h
	z.Dht = dht
	z.Psub = psub
	z.topics = make(map[string]*pubsub.Topic)

	return nil
}

func (z *Zstate) AddCommTopic(topicName string, listener func(msg *pubsub.Message)) error {
	topic, err := z.Psub.Join(topicName + "-znet") //topic used just for comms, not crdt
	if err != nil {
		return err
	}

	netSubs, err := topic.Subscribe()
	if err != nil {
		return err
	}

	// Use a special pubsub topic to avoid disconnecting
	// from globaldb peers.
	go func() {
		for {
			if (len(topic.ListPeers())) != 0 {
				fmt.Println("peers in topic")
				fmt.Println(topic.ListPeers())
			}
			msg, err := netSubs.Next(z.Ctx)
			if err != nil {
				fmt.Println(err)
				break
			}
			z.Host.ConnManager().TagPeer(msg.ReceivedFrom, "keep", 100)
			listener(msg)
		}
	}()

	z.topics[topicName] = topic

	return nil
}

func (z *Zstate) RemoveTopic(topicName string) {
	if pst, ok := z.topics[topicName]; !ok {
		return
	} else {
		pst.Close()
		delete(z.topics, topicName)
	}
}

func (z *Zstate) Close() {
	for _, topic := range z.topics {
		topic.Close()
	}
	z.Host.Close()
	z.Dht.Close()
}

func (z *Zstate) SelfId(topic string) string {
	return string(z.Host.ID())
}

var zSocketType = protocol.ID("/zomeCtrSocket/" + sharedInterfaces.ZomeVersion)

func (z *Zstate) DirectConnect(topicName, peerId string) (network.Stream, error) {
	if topic, ok := z.topics[topicName]; ok {
		var peerAddr peer.ID
		for _, p := range topic.ListPeers() {
			if p.String() == peerId {
				peerAddr = p
			}
		}
		stream, err := z.Host.NewStream(z.Ctx, peerAddr, zSocketType)
		if err != nil {
			return nil, err
		}
		return stream, nil
	} else {
		return nil, fmt.Errorf("topic not found")
	}
}

func (z *Zstate) PingAllPeers(topicName string) {
	if topic, ok := z.topics[topicName]; ok {
		fmt.Println("pinging peers")
		topic.Publish(z.Ctx, []byte("ping"))
	} else {
		fmt.Println("topic not found")
	}
}

type CleanPeer struct {
	ID     string
	Name   string
	Uptime string
	Space  string
}

func (z *Zstate) ConnectedPeersClean(topicsInput []string) []CleanPeer {
	dirtyPeers := z.ConnectedPeersFull(topicsInput)
	var cleanPeers []CleanPeer
	for _, p := range dirtyPeers {
		cleanPeers = append(cleanPeers, z.GetOnePeerInfo(p.ID))
	}
	return cleanPeers
}

func (z *Zstate) AddStoreInfo(key string, val interface{}) error {
	err := z.Host.Peerstore().Put(z.Host.ID(), key, val)
	if err != nil {
		return err
	}
	return nil
}

func (z *Zstate) ConnectedPeersFull(topicsInput []string) []*peer.AddrInfo {
	var pinfos []*peer.AddrInfo
	var topics []*pubsub.Topic
	if len(topicsInput) == 0 {
		for topic := range z.topics {
			topics = append(topics, z.topics[topic])
		}
	} else {
		for _, topicName := range topicsInput {
			if topic, ok := z.topics[topicName]; ok {
				println("topic found" + topicName)
				topics = append(topics, topic)
			}
		}
	}

	fmt.Println("topics" + fmt.Sprint(topics))

	// iterate over peers in each topic
	for _, topic := range topics {
		fmt.Println("peers in topic" + fmt.Sprint(topic.ListPeers()))
		for _, p := range topic.ListPeers() {
			pinfos = append(pinfos, &peer.AddrInfo{
				ID:    p,
				Addrs: z.Host.Peerstore().Addrs(p),
			})
		}
	}

	return pinfos
}

func (z *Zstate) GetOnePeerInfo(peerID peer.ID) CleanPeer {
	name, err := z.Host.Peerstore().Get(peerID, "name")
	if err != nil {
		//TODO: bad that this falls thru the crax?
		log.Println(err)
	}
	uname := "UNERR"
	if name, ok := name.(string); ok {
		uname = name
	}
	startTime, err := z.Host.Peerstore().Get(peerID, "start")
	if err != nil {
		log.Println(err)
	}
	uptime := "UTERR"
	if startTime, ok := startTime.(time.Time); ok {
		uptime = fmt.Sprint(time.Since(startTime).String())
	}
	space, err := z.Host.Peerstore().Get(peerID, "space")
	if err != nil {
		//log to host
		log.Println(err)
	}
	spaceStr := "SPERR"
	if space, ok := space.(string); ok {
		spaceStr = space
	}

	return CleanPeer{
		ID:     peerID.String()[len(peerID.String())-6:],
		Name:   uname,
		Uptime: uptime,
		Space:  spaceStr,
	}
}

func (z *Zstate) GetAddrForShortID(shortIDs []string) []peer.AddrInfo {
	var pinfos []peer.AddrInfo
	for _, c := range z.Host.Network().Conns() {
		pid := c.RemotePeer()
		if contains(shortIDs, pid.String()[len(pid.String())-6:]) {
			pinfos = append(pinfos, peer.AddrInfo{
				ID:    pid,
				Addrs: []multiaddr.Multiaddr{c.RemoteMultiaddr()},
			})
		}
	}
	return pinfos
}

// Helper function to check if a slice contains a string
func contains(slice []string, str string) bool {
	for _, v := range slice {
		if v == str {
			return true
		}
	}
	return false
}
