//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/jaketrock/zome/sync/raft"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const logLevel = zerolog.TraceLevel

func mkNode(nodesList string) *raft.Server {
	nodes := ParseNodes(nodesList)
	port := GetLocalPort(nodes)
	otherNodes := GetOtherNodes(nodes)
	localNode := GetLocalNode(nodes)
	log.Info().Msgf(" Starting Raft Server listening at: %v", port)
	log.Info().Msgf("All Node addresses: %v", nodes)
	log.Info().Msgf("Other Node addresses: %v", otherNodes)
	rs := raft.GetInitialServer()
	rs.StartServer(localNode, otherNodes)
	return rs
}

func mkClient(file string) *zomeClient {
	zClient := InitializeClient("", file, false, 5, false)
	zClient.HandleSignals()
	return zClient
}

func Test_main(t *testing.T) { //TODO: spoof stdout
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(logLevel)
	log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	port1 := 52051
	port2 := 52052
	port3 := 52053

	// servers
	mkNode(fmt.Sprintf(
		"localhost:%v,localhost:%v,localhost:%v",
		port1, port2, port3))
	mkNode(fmt.Sprintf(
		"localhost:%v,localhost:%v,localhost:%v",
		port2, port1, port3))
	mkNode(fmt.Sprintf(
		"localhost:%v,localhost:%v,localhost:%v",
		port3, port2, port1))
	// clients
	go mkClient("./data/clientImport.txt")

	// clean up after

	//remove all logs
	os.RemoveAll(fmt.Sprintf("./data/log_%v", port1))
	os.RemoveAll(fmt.Sprintf("./data/log_%v", port2))
	os.RemoveAll(fmt.Sprintf("./data/log_%v", port3))
	// remove all dbs
	os.RemoveAll(fmt.Sprintf("./sqlite-db-localhost-%v.db", port1))
	os.RemoveAll(fmt.Sprintf("./sqlite-db-localhost-%v.db", port2))
	os.RemoveAll(fmt.Sprintf("./sqlite-db-localhost-%v.db", port3))
	//remove all raftlogs
	os.RemoveAll(fmt.Sprintf("./sqlite-raft-log-localhost-%v.db", port1))
	os.RemoveAll(fmt.Sprintf("./sqlite-raft-log-localhost-%v.db", port2))
	os.RemoveAll(fmt.Sprintf("./sqlite-raft-log-localhost-%v.db", port3))

}
