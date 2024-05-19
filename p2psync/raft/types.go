//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

package raft

import (
	"net"

	"github.com/jaketrock/zome/sync/zproto"
	"google.golang.org/grpc"

	"sync"

	"database/sql"

	// Import for sqlite3 support.

	_ "github.com/mattn/go-sqlite3"
)

// Enum for the possible server states.
type ServerState int

const (
	// Followers only respond to request from other servers
	Follower = iota
	// Candidate is vying to become leaders
	Candidate
	// Leaders accept/process client process and continue until they fail.
	Leader
)

// server is used to implement zproto.RaftServer
type Server struct {
	//TODO: Implement the RaftServer interface backwards compat
	zproto.UnimplementedRaftServer

	serverState ServerState

	raftConfig RaftConfig

	raftState RaftState

	// RPC clients for interacting with other nodes in the raft cluster.
	otherNodes []zproto.RaftClient

	// Address information for this raft server node.
	localNode Node

	// Queue of event messages to be processed.
	events chan Event

	// Unix time in millis for when last hearbeat received when in non-leader
	// follower mode or when election started in candidate mode. Used to determine
	// when election timeouts occur.
	lastHeartbeatTimeMillis int64

	// True if we have received a heartbeat from a leader. Primary Purpose of this field to
	// determine whether we hear from a leader while in candidate status.
	receivedHeartbeat bool

	// Counts number of nodes in cluster that have chosen this node to be a leader
	receivedVoteCount int64

	// Database containing the persistent raft log
	raftLogDb *sql.DB

	// sqlite Database of the replicated state machine.
	sqlDb *sql.DB

	// Mutex to synchronize concurrent access to data structure
	lock sync.Mutex

	// Abstracted networking
	GetConnection func(target string) (*grpc.ClientConn, zproto.RaftClient)

	// Abstracted nw listener
	GetListener func(identifier string) (net.Listener, error)
}

// Overall type for the messages processed by the event-loop.
type Event struct {
	// The RPC (Remote Procedure Call) to be handled.
	rpc RpcEvent
}

// Type holder RPC events to be processed.
type RpcEvent struct {
	requestVote   *RaftRequestVoteRpcEvent
	appendEntries *RaftAppendEntriesRpcEvent
	clientCommand *RaftClientCommandRpcEvent
}

// Type for request vote rpc event.
type RaftRequestVoteRpcEvent struct {
	request *zproto.RequestVoteRequest
	// Channel for event loop to communicate back response to client.
	responseChan chan<- *zproto.RequestVoteResponse
}

// Type for append entries rpc event.
type RaftAppendEntriesRpcEvent struct {
	request *zproto.AppendEntriesRequest
	// Channel for event loop to communicate back response to client.
	responseChan chan<- *zproto.AppendEntriesResponse
}

// Type for client command rpc event.
type RaftClientCommandRpcEvent struct {
	request *zproto.ClientCommandRequest
	// Channel for event loop to communicate back response to client.
	responseChan chan<- *zproto.ClientCommandResponse
}

// Contains all the inmemory state needed by the Raft algorithm
type RaftState struct {
	persistentState     RaftPersistentState
	volatileState       RaftVolatileState
	volatileLeaderState RaftLeaderState
}

type RaftPersistentState struct {
	currentTerm int64
	votedFor    string
	log         []*zproto.DiskLogEntry
}

type RaftVolatileState struct {
	commitIndex int64
	lastApplied int64

	// Custom fields

	// Id for who we believe to be the current leader of the cluster.
	leaderId string
}

type RaftLeaderState struct {

	// Index of the next log entry to send to that server. Should be initialized
	// at leader last log index + 1.
	nextIndex []int64

	// Index of the highest log entry known to be replicated on each server.
	matchIndex []int64
}

// Contains Raft configuration parameters
type RaftConfig struct {

	// Amount of time to wait before starting an election.
	electionTimeoutMillis int64

	// Amount of time in between heartbeat RPCs that leader sends. This should be
	// much less than electionTimeout to avoid risking starting new election due to slow
	// heartbeat RPCs.
	heartBeatIntervalMillis int64
}

// Specification for a node
type Node struct {
	// A hostname of the node either in DNS or IP form e.g. localhost
	Hostname string
	// A port number for the node. e.g. :50051
	Port string
}
