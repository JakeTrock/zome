//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jaketrock/zome/sync/util/raft"
)

const (
	selectQueryPrefix    = "SELECT"
	defaultServerAddress = "localhost:50051"
)

func main() {
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

	if *client {
		// client mode
		fmt.Print("Starting in client mode")
		raftServer, conn := Connect(*serverAddress)
		if *cmdFile != "" {

			content, err := os.ReadFile(*cmdFile)
			if err != nil {
				log.Fatal(err)
			}

			Batch(string(content), raftServer)

			if !*interactive {
				os.Exit(0)
			}
		}
		Repl(conn, raftServer)
	} else {
		// server mode
		nodes := ParseNodes(*nodesPtr)
		port := GetLocalPort(nodes)
		fmt.Println("Starting in server mode")
		otherNodes := GetOtherNodes(nodes)
		localNode := GetLocalNode(nodes)
		log.Printf(" Starting Raft Server listening at: %v", port)
		log.Printf("All Node addresses: %v", nodes)
		log.Printf("Other Node addresses: %v", otherNodes)
		raft.StartServer(localNode, otherNodes)
	}
}
