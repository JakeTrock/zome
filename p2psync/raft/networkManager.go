//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

package raft

import (
	"net"

	"math/rand"
	"time"

	pb "github.com/jaketrock/zome/sync/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"bytes"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc/codes"

	"database/sql"

	// Import for sqlite3 support.
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/protobuf/proto"
)

// Handle to the raft server.

// Connects to a Raft server listening at the given address and returns a client
// to talk to this server.
func (raftServer *Server) ConnectToServer(address string) pb.RaftClient {
	// Set up a connection to the server. Note: this is not a blocking call.
	// Connection will be setup in the background.
	//TODO: remove deprecated grpc.WithInsecure() option
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Error().Msgf("Failed to connect to server at: %v", address)
	}
	c := pb.NewRaftClient(conn)

	return c
}

// Starts a Raft Server listening at the specified local node.
// otherNodes contain contact information for other nodes in the cluster.
func (raftServer *Server) StartServer(localNode Node, otherNodes []Node) *grpc.Server {
	addressPort := ":" + localNode.Port
	lis, err := net.Listen("tcp", addressPort)
	if err != nil {
		log.Error().Msgf("Failed to listen on: %v", addressPort)
	}
	log.Debug().Msgf("Created Raft server at: %v", lis.Addr().String())
	s := grpc.NewServer()

	log.Printf("Initial Server state: %v", raftServer.serverState)

	pb.RegisterRaftServer(s, raftServer)
	// Register reflection service on gRPC server.
	reflection.Register(s)

	// Initialize raft cluster.
	raftServer.otherNodes = raftServer.ConnectToOtherNodes(otherNodes)
	raftServer.localNode = localNode
	go raftServer.InitializeRaft(addressPort, otherNodes)

	// Note: the Serve call is blocking.
	if err := s.Serve(lis); err != nil {
		log.Error().Msgf("Failed to serve on: %v", addressPort)
	}

	return s
}

// Returns true iff server is the leader for the cluster.
func (raftServer *Server) IsLeader() bool {
	return raftServer.GetServerState() == Leader
}

// Returns true iff server is a follower in the cluster
func (raftServer *Server) IsFollower() bool {
	return raftServer.GetServerState() == Follower
}

// Returns true iff server is a candidate
func (raftServer *Server) IsCandidate() bool {
	return raftServer.GetServerState() == Candidate
}

// Returns initial server state.
func GetInitialServer() *Server {
	result := Server{
		serverState: Follower,
		raftConfig: RaftConfig{
			electionTimeoutMillis:   PickElectionTimeOutMillis(),
			heartBeatIntervalMillis: 10,
		},
		events: make(chan Event),
		// We initialize last heartbeat time at startup because all servers start out
		// in follower and this allows a node to determine when it should be a candidate.
		lastHeartbeatTimeMillis: UnixMillis(),
	}

	return &result
}

// Returns a go channel that blocks for specified amount of time.
func (raftServer *Server) GetTimeoutWaitChannel(timeoutMs int64) *time.Timer {
	return time.NewTimer(time.Millisecond * time.Duration(timeoutMs))
}

func (raftServer *Server) GetConfigElectionTimeoutMillis() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.raftConfig.electionTimeoutMillis
}

// Get amount of time remaining in millis before last heartbeat received is considered
// to have expired and thus we no longer have a leader.
func (raftServer *Server) GetRemainingHeartbeatTimeMs() int64 {
	timeoutMs := raftServer.GetConfigElectionTimeoutMillis()
	elapsedMs := raftServer.TimeSinceLastHeartBeatMillis()
	remainingMs := timeoutMs - elapsedMs
	if remainingMs < 0 {
		remainingMs = 0
	}
	return remainingMs
}

// Returns a go channel that blocks for a randomized election timeout time.
func (raftServer *Server) RandomizedElectionTimeout() *time.Timer {
	timeoutMs := PickElectionTimeOutMillis()
	return raftServer.GetTimeoutWaitChannel(timeoutMs)
}

// Picks a randomized time for the election timeout.
func PickElectionTimeOutMillis() int64 {
	baseTimeMs := int64(300)
	// Go random number is deterministic by default so we re-seed to get randomized behavior we want.
	rand.Seed(time.Now().Unix())
	randomOffsetMs := int64(rand.Intn(300))
	return baseTimeMs + randomOffsetMs
}

func (raftServer *Server) NodeToAddressString(input Node) string {
	return input.Hostname + ":" + input.Port
}

// Connects to the other Raft nodes and returns array of Raft Client connections.
func (raftServer *Server) ConnectToOtherNodes(otherNodes []Node) []pb.RaftClient {
	result := make([]pb.RaftClient, 0)
	for _, node := range otherNodes {
		serverAddress := raftServer.NodeToAddressString(node)
		log.Printf("Connecting to server: %v", serverAddress)
		client := raftServer.ConnectToServer(serverAddress)
		result = append(result, client)
	}
	return result
}

func (raftServer *Server) TestNodeConnections(nodeConns []pb.RaftClient) {
	// Try a test RPC call to other nodes.
	log.Printf("Have client conns: %v", nodeConns)
	for _, nodeConn := range nodeConns {
		result, err := nodeConn.RequestVote(context.Background(), &pb.RequestVoteRequest{})
		if err != nil {
			log.Printf("Error on connection: %v", err)
		}
		log.Printf("Got Response: %v", result)
	}

}

// Returns the Hostname/IP:Port info for the local node. This serves as the
// identifier for the node.
func (raftServer *Server) GetNodeId(node Node) string {
	return raftServer.NodeToAddressString(node)
}

func (raftServer *Server) GetLocalNode() Node {
	// No lock on localNode as it's unchanged after server init.

	return raftServer.localNode
}

// Returns identifier for this server.
func (raftServer *Server) GetLocalNodeId() string {
	return raftServer.GetNodeId(raftServer.GetLocalNode())
}

// Initializes Raft on server startup.
func (raftServer *Server) InitializeRaft(addressPort string, otherNodes []Node) {
	raftServer.InitializeDatabases()
	raftServer.StartServerLoop()
}
