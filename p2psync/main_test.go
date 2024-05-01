//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

package main

import (
	"log"
	"testing"

	"github.com/jaketrock/zome/sync/util/raft"
	"github.com/rs/zerolog"
)

const logLevel = zerolog.TraceLevel

func mkNode(nodesList string) *raft.Server {
	nodes := ParseNodes(nodesList)
	port := GetLocalPort(nodes)
	otherNodes := GetOtherNodes(nodes)
	localNode := GetLocalNode(nodes)
	log.Printf(" Starting Raft Server listening at: %v", port)
	log.Printf("All Node addresses: %v", nodes)
	log.Printf("Other Node addresses: %v", otherNodes)
	rs := raft.GetInitialServer(logLevel)
	rs.StartServer(localNode, otherNodes)
	return rs
}

func mkClient(file string) *zomeClient {
	zClient := InitializeClient(logLevel, "", file, false)
	zClient.HandleSignals()
	return zClient
}

func Test_main(t *testing.T) { //TODO: spoof stdout
	// servers
	mkNode("localhost:50051,localhost:50052,localhost:50053")
	mkNode("localhost:50052,localhost:50051,localhost:50053")
	mkNode("localhost:50053,localhost:50052,localhost:50051")
	// clients
	mkClient("./data/clientImport.txt")

}
