//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jaketrock/zome/sync/util/raft"

	"google.golang.org/grpc"

	proto "github.com/jaketrock/zome/sync/util/proto"
)

const (
	selectQuery          = "SELECT"
	defaultServerAddress = "localhost:50051"

	clientAttempts          = 5
	clientRetryOnBadCommand = false
)

var isServer bool

// server state
var nodes []raft.Node
var port string

// client state
var raftServer proto.RaftClient
var conn *grpc.ClientConn
var interactive bool
var cmdFile string
var serverAddress string

func ParseFlags() {
	client := flag.Bool("client", false, "Whether to start in client mode.")
	// client flags
	flag.StringVar(&serverAddress, "server", defaultServerAddress, "Address of Raft Cluster Leader.")
	flag.StringVar(&cmdFile, "batch", "", "Relative path to a command file to run in batch mode.")
	flag.BoolVar(&interactive, "interactive", false, "whether batch mode should transition to interactive mode.")
	// server flags
	nodesPtr := flag.String("nodes", "",
		"A comma separated list of node IP:port addresses."+
			" The first node is presumed to be this node and the port number"+
			" is what used to start the local raft server.")
	flag.Parse()
	isServer = !*client
	if isServer {
		// server mode
		nodes = ParseNodes(*nodesPtr)
		port = GetLocalPort(nodes)
		fmt.Println("Starting in server mode")
	} else {
		// client mode
		fmt.Print("Starting in client mode")
	}
}

func main() {
	ParseFlags()
	if isServer {
		otherNodes := GetOtherNodes()
		localNode := GetLocalNode(nodes)
		log.Printf(" Starting Raft Server listening at: %v", port)
		log.Printf("All Node addresses: %v", nodes)
		log.Printf("Other Node addresses: %v", otherNodes)
		raft.StartServer(localNode, otherNodes)
	} else {
		Connect(serverAddress)
		if cmdFile != "" {

			content, err := os.ReadFile(cmdFile)
			if err != nil {
				log.Fatal(err)
			}

			Batch(string(content))

			if !interactive {
				os.Exit(0)
			}
		}
		Repl()
	}
}
