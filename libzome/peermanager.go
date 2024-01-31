package libzome

import (
	"bufio"
	"context"
	"fmt"

	"crypto/rand"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/multiformats/go-multiaddr"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/libp2p/go-libp2p/core/crypto"
)

func (a *App) terminateConnection() bool {
	//TODO:implement
	return true
}

func receiveData(ctx context.Context, rw *bufio.ReadWriter) {
	for {
		str, err := rw.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading from buffer")
			panic(err)
		}

		if str == "" {
			return
		}
		if str != "\n" {
			fmt.Printf("%s", str)
			wruntime.EventsEmit(ctx, "peerData", str) //TODO: no sanitization here or below
		}
	}
}

func sendData(ctx context.Context, rw *bufio.ReadWriter) {
	wruntime.EventsOn(ctx, "peerData", func(data ...interface{}) {
		_, err := rw.WriteString(fmt.Sprintf("%s\n", data))
		if err != nil {
			fmt.Println("Error writing to buffer")
			panic(err)
		}
		err = rw.Flush()
		if err != nil {
			fmt.Println("Error flushing buffer")
			panic(err)
		}
	})
}

func initP2P(wailsContext context.Context, config Config) {
	portChoice := config.listenPort
	//TODO: port collision handling

	fmt.Printf("[*] Listening on: %s with port: %d\n", config.listenHost, portChoice)

	ctx := context.Background()
	r := rand.Reader

	// Creates a new RSA key pair for this host.
	prvKey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		panic(err)
	}

	// 0.0.0.0 will listen on any interface device.
	sourceMultiAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", config.listenHost, portChoice))

	// libp2p.New constructs a new libp2p Host.
	// Other options can be added here.
	host, err := libp2p.New(
		libp2p.ListenAddrs(sourceMultiAddr),
		libp2p.Identity(prvKey),
	)
	if err != nil {
		panic(err)
	}

	// Set a function as stream handler.
	// This function is called when a peer initiates a connection and starts a stream with this peer.
	host.SetStreamHandler(protocol.ID(config.ProtocolID), func(stream network.Stream) {
		logger.Info("Got a new stream!")
		wruntime.EventsEmit(ctx, "system", "Got a new stream!")

		// Create a buffer stream for non-blocking read and write.
		rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

		go receiveData(wailsContext, rw)
		go sendData(wailsContext, rw)

		// 'stream' will stay open until you close it (or the other side closes it).
	})

	fmt.Printf("\n[*] Your Multiaddress Is: /ip4/%s/tcp/%v/p2p/%s\n", config.listenHost, portChoice, host.ID())

	peerChan := initMDNS(host, config.RendezvousString)
	for { // allows multiple peers to join
		peer := <-peerChan // will block until we discover a peer
		if peer.ID > host.ID() {
			// if other end peer id greater than us, don't connect to it, just wait for it to connect us
			fmt.Println("Found peer:", peer, " id is greater than us, wait for it to connect to us")
			continue
		}
		fmt.Println("Found peer:", peer, ", connecting")

		if err := host.Connect(ctx, peer); err != nil {
			fmt.Println("Connection failed:", err)
			continue
		}

		// open a stream, this stream will be handled by handleStream other end
		stream, err := host.NewStream(ctx, peer.ID, protocol.ID(config.ProtocolID))

		if err != nil {
			fmt.Println("Stream open failed", err)
		} else {
			rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

			go receiveData(wailsContext, rw)
			go sendData(wailsContext, rw)
			fmt.Println("Connected to:", peer)
		}
	}
}
