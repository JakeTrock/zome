package main

import (
	"context"
	"time"

	ds "github.com/ipfs/go-datastore"
	crdt "github.com/ipfs/go-ds-crdt"

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
	keys := make([]string, len(a.subTopics))

	i := 0
	for k := range a.subTopics { //TODO: sub to all of these
		keys[i] = k
		i++
	}
	var topicName = keys[0]
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

	topic, err := psub.Join(topicName + "-net")
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
				time.Sleep(20 * time.Second)
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
}

func connectedPeers(h host.Host) []*peer.AddrInfo {
	var pinfos []*peer.AddrInfo
	for _, c := range h.Network().Conns() {
		pinfos = append(pinfos, &peer.AddrInfo{
			ID:    c.RemotePeer(),
			Addrs: []multiaddr.Multiaddr{c.RemoteMultiaddr()},
		})
	}
	return pinfos
}
