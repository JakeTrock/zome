package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	ds "github.com/ipfs/go-datastore"
	crdt "github.com/ipfs/go-ds-crdt"
	"github.com/lucsky/cuid"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	ipfslite "github.com/hsanjuan/ipfs-lite"

	multiaddr "github.com/multiformats/go-multiaddr"
)

func (a *App) InitP2P() {

	var listen, err = multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/33123")
	if err != nil {
		logger.Fatal(err)
	}

	herdTopicKey := ds.NewKey("herdTopic")
	herdTopic, err := a.store.Get(a.ctx, herdTopicKey)
	topicName := string(herdTopic)
	if err != nil && err != ds.ErrNotFound {
		logger.Fatal(err)
	} else if err == ds.ErrNotFound {
		randomUUID := cuid.New()

		err = a.store.Put(a.ctx, herdTopicKey, []byte(randomUUID))
		if err != nil {
			logger.Fatal(err)
		}
		topicName = randomUUID
	}

	// Bootstrappers are using 1024 keys. See:
	// https://github.com/ipfs/infra/issues/378

	h, dht, err := ipfslite.SetupLibp2p(
		a.ctx,
		a.privateKey,
		nil,
		[]multiaddr.Multiaddr{listen},
		nil,
		ipfslite.Libp2pOptionsExtra...,
	)
	if err != nil {
		logger.Fatal(err)
	}
	defer h.Close()
	defer dht.Close()

	psub, err := pubsub.NewGossipSub(a.ctx, h)
	if err != nil {
		logger.Fatal(err)
	}

	topic, err := psub.Join(topicName + "-znet")
	if err != nil {
		logger.Fatal(err)
	}

	netSubs, err := topic.Subscribe()
	if err != nil {
		logger.Fatal(err)
	}

	// Use a special pubsub topic to avoid disconnecting
	// from globaldb peers.
	go func() {
		for {
			msg, err := netSubs.Next(a.ctx)
			if err != nil {
				logger.Info(err)
				break
			}
			h.ConnManager().TagPeer(msg.ReceivedFrom, "keep", 100)
		}
	}()

	go func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			default:
				topic.Publish(a.ctx, []byte("ping"))
				h.Peerstore().Put(a.peerId, "name", a.friendlyName) //user friendly name
				time.Sleep(20 * time.Second)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			default:
				// send this out less frequently
				h.Peerstore().Put(a.peerId, "start", a.startTime.String())
				dataDirSize, err := DirSize(a.operatingPath)
				if err != nil {
					logger.Error(err)
				}
				dbSize, err := ds.DiskUsage(a.ctx, a.store)
				if err != nil {
					logger.Error(err)
				}
				h.Peerstore().Put(a.peerId, "space", strconv.FormatInt(dataDirSize+int64(dbSize), 10))

				time.Sleep(200 * time.Second)
			}
		}
	}()

	ipfs, err := ipfslite.New(a.ctx, a.store, nil, h, dht, nil)
	if err != nil {
		logger.Fatal(err)
	}

	psubctx, psubCancel := context.WithCancel(a.ctx)
	pubsubBC, err := crdt.NewPubSubBroadcaster(psubctx, psub, topicName)
	if err != nil {
		logger.Fatal(err)
	}

	opts := crdt.DefaultOptions()
	opts.Logger = logger
	opts.RebroadcastInterval = 5 * time.Second
	opts.PutHook = func(k ds.Key, v []byte) {
		logger.Info("Added: [%s] -> %s\n", k, string(v))
	}
	opts.DeleteHook = func(k ds.Key) {
		logger.Info("Removed: [%s]\n", k)
	}

	crdt, err := crdt.New(a.store, ds.NewKey("crdt"), ipfs, pubsubBC, opts)
	if err != nil {
		logger.Fatal(err)
	}
	defer crdt.Close()
	defer psubCancel()

	logger.Info("Bootstrapping...")

	//TODO: what the hell is this?
	bstr, _ := multiaddr.NewMultiaddr("/ip4/94.130.135.167/tcp/33123/ipfs/12D3KooWFta2AE7oiK1ioqjVAKajUJauZWfeM7R413K7ARtHRDAu")
	inf, _ := peer.AddrInfoFromP2pAddr(bstr)
	list := append(ipfslite.DefaultBootstrapPeers(), *inf)
	ipfs.Bootstrap(list)
	h.ConnManager().TagPeer(inf.ID, "keep", 100)
	logger.Infof(`
Peer ID: %s
Listen address: %s
Topic: %s
Data Folder: %s
`,
		a.peerId, listen, topicName, a.operatingPath,
	)

	a.host = h

	a.restrictedTopic = topic
}

type cleanPeer struct {
	ID     string
	Name   string
	Uptime string
	Space  string
}

func connectedPeersClean(h host.Host) []cleanPeer {
	var pinfos []cleanPeer

	for _, c := range h.Network().Conns() {
		pinfos = append(pinfos, getOnePeerInfo(h, c.RemotePeer()))
	}
	return pinfos
}

func getOnePeerInfo(h host.Host, peerID peer.ID) cleanPeer {
	name, err := h.Peerstore().Get(peerID, "name")
	if err != nil {
		logger.Error(err)
	}
	uname := "UNERR"
	if name, ok := name.(string); ok {
		uname = name
	}
	startTime, err := h.Peerstore().Get(peerID, "start")
	if err != nil {
		logger.Error(err)
	}
	uptime := "UTERR"
	if startTime, ok := startTime.(time.Time); ok {
		uptime = fmt.Sprint(time.Since(startTime).String())
	}
	space, err := h.Peerstore().Get(peerID, "space")
	if err != nil {
		logger.Error(err)
	}
	spaceStr := "SPERR"
	if space, ok := space.(string); ok {
		spaceStr = space
	}

	return cleanPeer{
		ID:     peerID.String()[len(peerID.String())-6:],
		Name:   uname,
		Uptime: uptime,
		Space:  spaceStr,
	}
}

func getAddrForShortID(h host.Host, shortIDs []string) []peer.AddrInfo {
	var pinfos []peer.AddrInfo
	for _, c := range h.Network().Conns() {
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

func connectedPeersFull(h host.Host) []*peer.AddrInfo {
	var pinfos []*peer.AddrInfo
	for _, c := range h.Network().Conns() {
		pinfos = append(pinfos, &peer.AddrInfo{
			ID:    c.RemotePeer(),
			Addrs: []multiaddr.Multiaddr{c.RemoteMultiaddr()},
		})
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
