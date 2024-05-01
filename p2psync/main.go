//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/jaketrock/zome/sync/util"
	"github.com/jaketrock/zome/sync/util/raft"
)

const (
	selectQuery          = "SELECT"
	defaultServerAddress = "localhost:50051"

	clientAttempts          = 5
	clientRetryOnBadCommand = false
)

func main() {
	// server state
	client := flag.Bool("client", false, "Whether to start in client mode.")
	// client flags
	serverAddress := flag.String("server", defaultServerAddress, "Address of Raft Cluster Leader.")
	cmdFile := flag.String("batch", "", "Relative path to a command file to run in batch mode.")
	interactive := flag.Bool("interactive", false, "whether batch mode should transition to interactive mode.")
	// server flags
	nodesPtr := flag.String("nodes", "",
		"A comma separated list of node IP:port addresses."+
			" The first node is presumed to be this node and the port number"+
			" is what used to start the local raft server.")
	flag.Parse()

	logger := util.Logger{
		Level: util.EXTRA_VERBOSE,
	}

	if *client {
		// client mode
		fmt.Println("Starting in client mode")

		zClient := InitializeClient(&logger, *serverAddress, *cmdFile, *interactive)
		zClient.HandleSignals()
	} else {
		// server mode
		fmt.Println("Starting in server mode")

		nodes := ParseNodes(*nodesPtr)
		port := GetLocalPort(nodes)
		otherNodes := GetOtherNodes(nodes)
		localNode := GetLocalNode(nodes)
		log.Printf(" Starting Raft Server listening at: %v", port)
		log.Printf("All Node addresses: %v", nodes)
		log.Printf("Other Node addresses: %v", otherNodes)
		rs := raft.GetInitialServer(&logger)
		rs.StartServer(localNode, otherNodes)
	}
}
