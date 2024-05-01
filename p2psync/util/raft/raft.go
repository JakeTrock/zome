//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

package raft

import (
	"net"

	"math/rand"
	"time"

	pb "github.com/jaketrock/zome/sync/util/proto"
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

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/protobuf/proto"
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

// server is used to implement pb.RaftServer
type Server struct {
	//TODO: Implement the RaftServer interface backwards compat
	pb.UnimplementedRaftServer

	serverState ServerState

	raftConfig RaftConfig

	raftState RaftState

	// RPC clients for interacting with other nodes in the raft cluster.
	otherNodes []pb.RaftClient

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
	request *pb.RequestVoteRequest
	// Channel for event loop to communicate back response to client.
	responseChan chan<- *pb.RequestVoteResponse
}

// Type for append entries rpc event.
type RaftAppendEntriesRpcEvent struct {
	request *pb.AppendEntriesRequest
	// Channel for event loop to communicate back response to client.
	responseChan chan<- *pb.AppendEntriesResponse
}

// Type for client command rpc event.
type RaftClientCommandRpcEvent struct {
	request *pb.ClientCommandRequest
	// Channel for event loop to communicate back response to client.
	responseChan chan<- *pb.ClientCommandResponse
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
	log         []*pb.DiskLogEntry
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

// Raft Persistent State accessors / getters / functions.
func (raftServer *Server) GetPersistentRaftLog() []*pb.DiskLogEntry {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()
	return raftServer.raftState.persistentState.log
}

func (raftServer *Server) GetPersistentRaftLogEntryAt(index int64) *pb.DiskLogEntry {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()
	return raftServer.raftState.persistentState.log[index]
}

func (raftServer *Server) GetPersistentVotedFor() string {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()
	return raftServer.GetPersistentVotedForLocked()
}

// Locked means caller already holds appropriate lock.
func (raftServer *Server) GetPersistentVotedForLocked() string {
	return raftServer.raftState.persistentState.votedFor
}

func (raftServer *Server) SetPersistentVotedFor(newValue string) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.SetPersistentVotedForLocked(newValue)
}

func (raftServer *Server) SetPersistentVotedForLocked(newValue string) {
	// Want to write to stable storage (database).
	// First determine if we're updating or inserting value.

	rows, err := raftServer.raftLogDb.Query("SELECT value FROM RaftKeyValue WHERE key = 'votedFor'")
	if err != nil {
		log.Debug().Msgf("Failed to read persisted voted for while trying to update it. err: %v", err)
	}
	defer rows.Close()
	votedForExists := false
	for rows.Next() {
		var valueStr string
		err = rows.Scan(&valueStr)
		if err != nil {
			log.Error().Msgf("Error  reading persisted voted for row while try to update: %v", err)
		}
		votedForExists = true
	}

	// Now proceed with the update/insertion.
	tx, err := raftServer.raftLogDb.Begin()
	if err != nil {
		log.Error().Msgf("Failed to begin db tx to update voted for. err:%v", err)
	}

	needUpdate := votedForExists
	var statement *sql.Stmt
	if needUpdate {
		statement, err = tx.Prepare("UPDATE RaftKeyValue SET value = ? WHERE key = ?")
		if err != nil {
			log.Error().Msgf("Failed to prepare stmt to update voted for. err: %v", err)
		}
		_, err = statement.Exec(newValue, "votedFor")
		if err != nil {
			log.Error().Msgf("Failed to update voted for value. err: %v", err)
		} else {
			log.Debug().Msgf("Successfully updated voted for value.")
		}
	} else {
		statement, err = tx.Prepare("INSERT INTO RaftKeyValue(key, value) values(?, ?)")
		if err != nil {
			log.Error().Msgf("Failed to create stmt to insert voted for. err: %v", err)
		}
		_, err = statement.Exec("votedFor", newValue)
		if err != nil {
			log.Error().Msgf("Failed to insert voted for value. err: %v", err)
		} else {
			log.Debug().Msgf("Successfully inserted voted for value.")
		}
	}
	defer statement.Close()
	err = tx.Commit()
	if err != nil {
		log.Error().Msgf("Failed to commit tx to update voted for. err: %v", err)
	}

	// Then update in-memory state last.
	raftServer.raftState.persistentState.votedFor = newValue
}

func (raftServer *Server) GetPersistentCurrentTerm() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.GetPersistentCurrentTermLocked()
}

func (raftServer *Server) GetPersistentCurrentTermLocked() int64 {
	return raftServer.raftState.persistentState.currentTerm
}

func (raftServer *Server) SetPersistentCurrentTerm(newValue int64) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.SetPersistentCurrentTermLocked(newValue)
}

func (raftServer *Server) SetPersistentCurrentTermLocked(newValue int64) {
	// Write to durable storage (database) first.
	// First determine if we're updating or inserting brand new value.

	rows, err := raftServer.raftLogDb.Query("SELECT value FROM RaftKeyValue WHERE key = 'currentTerm'")
	if err != nil {
		log.Error().Msgf("Failed to read persisted current term while trying to update it. err: %v", err)
	}
	defer rows.Close()
	valueExists := false
	for rows.Next() {
		var valueStr string
		err = rows.Scan(&valueStr)
		if err != nil {
			log.Error().Msgf("Error reading persisted currentTerm row while try to update: %v", err)
		}
		valueExists = true
	}

	// Now proceed with the update/insertion.

	tx, err := raftServer.raftLogDb.Begin()
	if err != nil {
		log.Error().Msgf("Failed to begin db tx to update current term. err:%v", err)
	}

	newValueStr := strconv.FormatInt(newValue, 10)
	var statement *sql.Stmt
	if valueExists {
		// Then update it.
		statement, err = tx.Prepare("UPDATE RaftKeyValue SET value = ? WHERE key = ?")
		if err != nil {
			log.Error().Msgf("Failed to prepare stmt to update current term. err: %v", err)
		}
		_, err = statement.Exec(newValueStr, "currentTerm")
		if err != nil {
			log.Error().Msgf("Failed to update current term value. err: %v", err)
		}
	} else {
		// Insert a brand new one.
		statement, err = tx.Prepare("INSERT INTO RaftKeyValue(key, value) values(?, ?)")
		if err != nil {
			log.Error().Msgf("Failed to create stmt to insert current term. err: %v", err)
		}
		_, err = statement.Exec("currentTerm", newValueStr)
		if err != nil {
			log.Error().Msgf("Failed to insert currentTerm value. err: %v", err)
		}
	}
	defer statement.Close()
	err = tx.Commit()
	if err != nil {
		log.Error().Msgf("Failed to commit tx to update current term. err: %v", err)
	}

	// Then update in memory state last.
	raftServer.raftState.persistentState.currentTerm = newValue
}

func (raftServer *Server) AddPersistentLogEntry(newValue *pb.LogEntry) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.AddPersistentLogEntryLocked(newValue)
}

func (raftServer *Server) AddPersistentLogEntryLocked(newValue *pb.LogEntry) {
	nextIndex := int64(len(raftServer.raftState.persistentState.log)) + 1
	diskEntry := pb.DiskLogEntry{
		LogEntry: newValue,
		LogIndex: nextIndex,
	}

	// Update database (stable storage first).
	tx, err := raftServer.raftLogDb.Begin()
	if err != nil {
		log.Error().Msgf("Failed to begin db tx. Err: %v", err)
	}
	statement, err := tx.Prepare("INSERT INTO RaftLog(log_index, log_entry) values(?, ?)")
	if err != nil {
		log.Error().Msgf("Failed to prepare sql statement to add log entry. err: %v", err)
	}
	defer statement.Close()
	protoByte, err := proto.Marshal(newValue)
	if err != nil {
		log.Error().Msgf("Failed to marshal log entry proto. err: %v", err)
	}
	protoByteText := string(protoByte)
	_, err = statement.Exec(nextIndex, protoByteText)
	if err != nil {
		log.Error().Msgf("Failed to execute sql statement to add log entry. err: %v", err)
	}
	err = tx.Commit()
	if err != nil {
		log.Error().Msgf("Failed to commit tx to add log entry. err: %v", err)
	}

	raftServer.raftState.persistentState.log = append(raftServer.raftState.persistentState.log, &diskEntry)
}

func (raftServer *Server) DeletePersistentLogEntryInclusive(startDeleteLogIndex int64) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()
	raftServer.DeletePersistentLogEntryInclusiveLocked(startDeleteLogIndex)
}

// Delete all log entries starting from the given log index.
// Note: input log index is 1-based.
func (raftServer *Server) DeletePersistentLogEntryInclusiveLocked(startDeleteLogIndex int64) {

	// Delete first from database storage.
	tx, err := raftServer.raftLogDb.Begin()
	if err != nil {
		log.Error().Msgf("Failed to begin db tx for delete log entries. err: %v", err)
	}

	statement, err := tx.Prepare("DELETE FROM RaftLog WHERE log_index >= ?")
	if err != nil {
		log.Error().Msgf("Failed to prepare sql statement to delete log entry. err: %v", err)
	}
	defer statement.Close()

	_, err = statement.Exec(startDeleteLogIndex)
	if err != nil {
		log.Error().Msgf("Failed to execute sql statement to delete log entry. err: %v", err)
	}
	err = tx.Commit()
	if err != nil {
		log.Error().Msgf("Failed to commit tx to delete log entry. err: %v", err)
	}

	// Finally update the in-memory state.
	zeroBasedDeleteIndex := startDeleteLogIndex - 1
	raftServer.raftState.persistentState.log = raftServer.raftState.persistentState.log[:zeroBasedDeleteIndex]
}

func (raftServer *Server) IncrementPersistentCurrentTerm() {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.IncrementPersistentCurrentTermLocked()
}

func (raftServer *Server) IncrementPersistentCurrentTermLocked() {
	val := raftServer.GetPersistentCurrentTermLocked()
	newVal := val + 1
	raftServer.SetPersistentCurrentTermLocked(newVal)
}

func (raftServer *Server) SetReceivedHeartbeat(newVal bool) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.SetReceivedHeartbeatLocked(newVal)
}

func (raftServer *Server) SetReceivedHeartbeatLocked(newVal bool) {
	raftServer.receivedHeartbeat = newVal
}

// Raft Volatile State.
func (raftServer *Server) GetLeaderIdLocked() string {
	return raftServer.raftState.volatileState.leaderId
}

func (raftServer *Server) GetLeaderId() string {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.GetLeaderIdLocked()
}

func (raftServer *Server) SetLeaderIdLocked(newVal string) {
	raftServer.raftState.volatileState.leaderId = newVal
}

func (raftServer *Server) SetLeaderId(newVal string) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()
	raftServer.SetLeaderIdLocked(newVal)
}

// AppendEntries implementation for pb.RaftServer
func (raftServer *Server) AppendEntries(ctx context.Context, in *pb.AppendEntriesRequest) (*pb.AppendEntriesResponse, error) {
	replyChan := make(chan *pb.AppendEntriesResponse)
	event := Event{
		rpc: RpcEvent{
			appendEntries: &RaftAppendEntriesRpcEvent{
				request:      proto.Clone(in).(*pb.AppendEntriesRequest),
				responseChan: replyChan,
			},
		},
	}
	raftServer.events <- event

	result := <-replyChan
	return result, nil
}

// RequestVote implementation for raft.RaftServer
func (raftServer *Server) RequestVote(ctx context.Context, in *pb.RequestVoteRequest) (*pb.RequestVoteResponse, error) {
	replyChan := make(chan *pb.RequestVoteResponse)
	event := Event{
		rpc: RpcEvent{
			requestVote: &RaftRequestVoteRpcEvent{
				request:      proto.Clone(in).(*pb.RequestVoteRequest),
				responseChan: replyChan,
			},
		},
	}
	raftServer.events <- event

	result := <-replyChan
	return result, nil
}

// Client Command implementation for raft.RaftServer
func (raftServer *Server) ClientCommand(ctx context.Context, in *pb.ClientCommandRequest) (*pb.ClientCommandResponse, error) {
	replyChan := make(chan *pb.ClientCommandResponse)
	event := Event{
		rpc: RpcEvent{
			clientCommand: &RaftClientCommandRpcEvent{
				request:      proto.Clone(in).(*pb.ClientCommandRequest),
				responseChan: replyChan,
			},
		},
	}
	raftServer.events <- event

	result := <-replyChan
	return result, nil
}

// Specification for a node
type Node struct {
	// A hostname of the node either in DNS or IP form e.g. localhost
	Hostname string
	// A port number for the node. e.g. :50051
	Port string
}

// Variables

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
func GetInitialServer(logLevel zerolog.Level) *Server {
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
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	zerolog.SetGlobalLevel(logLevel)

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

func (raftServer *Server) InitializeDatabases() {
	raftDbLog, err := sql.Open("sqlite3", raftServer.GetSqliteRaftLogPath())
	if err != nil {
		log.Error().Msgf("Failed to open raft db log.")
	}
	replicatedStateMachineDb, err := sql.Open("sqlite3", raftServer.GetSqliteReplicatedStateMachineOpenPath())
	if err != nil {
		log.Error().Msgf("Failed to open replicated state machine database.")
	}

	raftDbLog.Exec("pragma database_list")
	replicatedStateMachineDb.Exec("pragma database_list")

	// Create Tables for the raft log persistence. We want two tables:
	// 1) RaftLog with columns of log_index, log entry proto
	// 2) RaftKeyValue with columns of key,value. This is used to keep two properties
	//    votedFor and currentTerm.

	raftLogTableCreateStatement :=
		` CREATE TABLE IF NOT EXISTS RaftLog (
             log_index INTEGER NOT NULL PRIMARY KEY,
             log_entry TEXT);
        `
	_, err = raftDbLog.Exec(raftLogTableCreateStatement)
	if err != nil {
		log.Error().Msgf("Created Raft Log table.")
	}

	raftKeyValueCreateStatement :=
		` CREATE TABLE IF NOT EXISTS RaftKeyValue (
              key TEXT NOT NULL PRIMARY KEY,
              value TEXT);
        `
	_, err = raftDbLog.Exec(raftKeyValueCreateStatement)
	if err != nil {
		log.Error().Msgf("Created Raft Log and Raft Key Value tables.")
	}

	raftServer.sqlDb = replicatedStateMachineDb
	raftServer.raftLogDb = raftDbLog

	raftServer.LoadPersistentStateIntoMemory()
}

// Loads the on disk persistent state into memory.
func (raftServer *Server) LoadPersistentStateIntoMemory() {
	log.Debug().Msgf("Before load. Raft Persistent State: %v ", raftServer.raftState.persistentState)
	raftServer.LoadPersistentLog()
	raftServer.LoadPersistentKeyValues()
	log.Debug().Msgf("After load. Raft Persistent State: %v ", raftServer.raftState.persistentState)
}

func (raftServer *Server) LoadPersistentKeyValues() {
	// Load value for the current term.
	raftServer.LoadPersistedCurrentTerm()
	// Load value for the voted for into memory
	raftServer.LoadPersistedVotedFor()
}
func (raftServer *Server) LoadPersistedVotedFor() {
	rows, err := raftServer.raftLogDb.Query("SELECT value FROM RaftKeyValue WHERE key = 'votedFor';")
	if err != nil {
		log.Error().Msgf("Failed to read votedFor value into memory.")
	}
	defer rows.Close()
	for rows.Next() {
		var valueStr string
		err = rows.Scan(&valueStr)
		if err != nil {
			log.Error().Msgf("Failed to read votedFor row entry.")
		}

		log.Debug().Msgf("Restoring votedFor value to memory: %v", valueStr)
		raftServer.raftState.persistentState.votedFor = valueStr
	}
}

func (raftServer *Server) LoadPersistedCurrentTerm() {
	rows, err := raftServer.raftLogDb.Query("SELECT value FROM RaftKeyValue WHERE key = 'currentTerm';")
	if err != nil {
		log.Error().Msgf("Failed to read current term value into memory.")
	}
	defer rows.Close()
	for rows.Next() {
		var valueStr string
		err = rows.Scan(&valueStr)
		if err != nil {
			log.Error().Msgf("Failed to read current term row entry.")
		}
		restoredTerm, err := strconv.ParseInt(valueStr, 10, 64)
		if err != nil {
			log.Error().Msgf("Failed to parse current term as integer.")
		}
		log.Debug().Msgf("Restoring current term to memory: %v", restoredTerm)
		raftServer.raftState.persistentState.currentTerm = restoredTerm
	}
}

func (raftServer *Server) LoadPersistentLog() {
	rows, err := raftServer.raftLogDb.Query("SELECT log_index, log_entry FROM RaftLog ORDER BY log_index ASC;")
	if err != nil {
		log.Error().Msgf("Failed to load raft persistent state into memory.")
	}
	defer rows.Close()
	for rows.Next() {
		var logIndex int64
		var logEntryProtoText string
		err = rows.Scan(&logIndex, &logEntryProtoText)
		if err != nil {
			log.Error().Msgf("Failed to read raft log row entry.")
		}
		log.Debug().Msgf("Restoring raft log to memory: %v, %v", logIndex, logEntryProtoText)

		var parsedLogEntry pb.LogEntry
		err := proto.Unmarshal([]byte(logEntryProtoText), &parsedLogEntry)
		if err != nil {
			log.Error().Msgf("Error parsing log entry proto: %v", logEntryProtoText)
		}
		diskLogEntry := pb.DiskLogEntry{
			LogIndex: logIndex,
			LogEntry: &parsedLogEntry,
		}

		raftServer.raftState.persistentState.log = append(raftServer.raftState.persistentState.log, &diskLogEntry)
	}
}

// Moves the commit index forward from the current value to the given index.
// Note: newIndex should be the log index value which is 1-based.
func (raftServer *Server) MoveCommitIndexTo(newIndex int64) {
	log.Info().Msgf("MoveCommitIndexTo newIndex: %v", newIndex)

	startCommitIndex := raftServer.GetCommitIndex()
	newCommitIndex := newIndex

	if newCommitIndex < startCommitIndex {
		log.Error().Msgf("Commit index trying to move backwards.")
	}

	startCommitIndexZeroBased := startCommitIndex - 1
	newCommitIndexZeroBased := newCommitIndex - 1

	raftLog := raftServer.GetPersistentRaftLog()
	commands := raftLog[startCommitIndexZeroBased+1 : newCommitIndexZeroBased+1]
	for _, cmd := range commands {
		if newCommitIndex > raftServer.GetLastApplied() {
			log.Debug().Msgf("Applying log entry: %v", cmd.String())
			raftServer.ApplySqlCommand(cmd.LogEntry.Data)
			raftServer.SetLastApplied(cmd.LogIndex)
		}
	}
	raftServer.SetLastApplied(newCommitIndex)
	// Finally update the commit index.
	raftServer.SetCommitIndex(newCommitIndex)
}

func (raftServer *Server) GetLastHeartbeatTimeMillis() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.lastHeartbeatTimeMillis
}

// Returns duration of time in milliseconds since the last successful heartbeat.
func (raftServer *Server) TimeSinceLastHeartBeatMillis() int64 {
	now := UnixMillis()
	diffMs := now - raftServer.GetLastHeartbeatTimeMillis()
	if diffMs < 0 {
		log.Warn().Msgf("Negative time since last heartbeat. Assuming 0.")
		diffMs = 0
	}
	return diffMs
}

// Returns true if the election timeout has already passed for this node.
func (raftServer *Server) IsElectionTimeoutElapsed() bool {
	timeoutMs := raftServer.GetConfigElectionTimeoutMillis()
	elapsedMs := raftServer.TimeSinceLastHeartBeatMillis()
	if elapsedMs > timeoutMs {
		return true
	} else {
		return false
	}
}

// Resets the election time out. This restarts amount of time that has to pass
// before an election timeout occurs. Election timeouts lead to new elections.
func (raftServer *Server) ResetElectionTimeOut() {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.lastHeartbeatTimeMillis = UnixMillis()
}

// Returns true if this node already voted for a node to be a leader.
func (raftServer *Server) AlreadyVoted() bool {

	if raftServer.GetPersistentVotedFor() != "" {
		return true
	} else {
		return false
	}
}

func (raftServer *Server) ChangeToCandidateStatus() {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.serverState = Candidate
}

func (raftServer *Server) GetReceivedHeartbeat() bool {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.receivedHeartbeat
}

func (raftServer *Server) ResetReceivedVoteCount() {
	// Using atomics -- so no need to lock.
	atomic.StoreInt64(&raftServer.receivedVoteCount, 0)
}

// Increments election term and also resets the relevant raft state.
func (raftServer *Server) IncrementElectionTerm() {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.SetPersistentVotedForLocked("")
	raftServer.IncrementPersistentCurrentTermLocked()
	raftServer.ResetReceivedVoteCount()
	raftServer.SetReceivedHeartbeatLocked(false)
}

func (raftServer *Server) VoteForSelf() {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	myId := raftServer.GetLocalNodeId()
	raftServer.SetPersistentVotedForLocked(myId)
	raftServer.IncrementVoteCount()
}

// Votes for the given server node.
func (raftServer *Server) VoteForServer(serverToVoteFor Node) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	serverId := raftServer.GetNodeId(serverToVoteFor)
	raftServer.SetPersistentVotedForLocked(serverId)
}

// Returns the size of the raft cluster.
func (raftServer *Server) GetRaftClusterSize() int64 {
	// No lock here because otherNote is unchanged after server init.

	return int64(len(raftServer.otherNodes) + 1)
}

// Returns the number of votes needed to have a quorum in the cluster.
func (raftServer *Server) GetQuorumSize() int64 {
	// Total := 2(N+1/2), where N is number of allowed failures.
	// Need N+1 for a quorum.
	// N: = (Total/2 - 0.5) = floor(Total/2)
	numTotalNodes := raftServer.GetRaftClusterSize()
	quorumSize := (numTotalNodes / 2) + 1
	return quorumSize
}

func (raftServer *Server) GetVoteCount() int64 {
	return atomic.LoadInt64(&raftServer.receivedVoteCount)
}

// Returns true if this node has received sufficient votes to become a leader
func (raftServer *Server) HaveEnoughVotes() bool {
	if raftServer.GetVoteCount() >= raftServer.GetQuorumSize() {
		return true
	} else {
		return false
	}
}

// Returns current raft term.
func (raftServer *Server) RaftCurrentTerm() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.GetPersistentCurrentTermLocked()
}

func (raftServer *Server) SetReceivedHeartBeat() {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.receivedHeartbeat = true
}

// Instructions that followers would be processing.
func (raftServer *Server) FollowerLoop() {

	// - Check if election timeout expired.
	// - If so, change to candidate status only.
	// Note(jmuindi):  The requirement to check that we have not already voted
	// as specified on figure 2 is covered because when after becoming a candidate
	// we vote for our self and the event loop code structure for rpcs processing
	// guarantees we won't vote for anyone else.
	log.Debug().Msgf("Starting  follower loop")
	raftServer.ResetElectionTimeOut()
	rpcCount := 0
	for {
		if raftServer.GetServerState() != Follower {
			return
		}

		remainingHeartbeatTimeMs := raftServer.GetRemainingHeartbeatTimeMs()
		timeoutTimer := raftServer.GetTimeoutWaitChannel(remainingHeartbeatTimeMs)

		select {
		case event := <-raftServer.events:
			log.Debug().Msgf("Processing rpc #%v event: %v", rpcCount, event)
			raftServer.handleRpcEvent(event)
			rpcCount++
		case <-timeoutTimer.C:
			// Election timeout occured w/o heartbeat from leader.
			raftServer.ChangeToCandidateStatus()
			return
		}
	}
}

func (raftServer *Server) handleRpcEvent(event Event) {
	if event.rpc.requestVote != nil {
		raftServer.handleRequestVoteRpc(event.rpc.requestVote)
	} else if event.rpc.appendEntries != nil {
		raftServer.handleAppendEntriesRpc(event.rpc.appendEntries)
	} else if event.rpc.clientCommand != nil {
		raftServer.handleClientCommandRpc(event.rpc.clientCommand)
	} else {
		log.Error().Msgf("Unexpected rpc event: %v", event)
	}
}

// Handles client command request.
func (raftServer *Server) handleClientCommandRpc(event *RaftClientCommandRpcEvent) {
	if !raftServer.IsLeader() {
		result := pb.ClientCommandResponse{}
		log.Warn().Msgf("Rejecting client command because not leader")
		result.ResponseStatus = uint32(codes.FailedPrecondition)
		result.NewLeaderId = raftServer.GetLeaderId()
		event.responseChan <- &result
		return
	}

	if event.request.GetCommand() != "" {
		raftServer.handleClientMutateCommand(event)
	} else if event.request.GetQuery() != "" {
		raftServer.handleClientQueryCommand(event)
	} else {
		// Invalid / unexpected request.
		result := pb.ClientCommandResponse{}
		log.Warn().Msgf("Invalid client command (not command/query): %v", event)
		result.ResponseStatus = uint32(codes.InvalidArgument)
		event.responseChan <- &result
		return
	}

}

func (raftServer *Server) handleClientQueryCommand(event *RaftClientCommandRpcEvent) {
	sqlQuery := event.request.Query
	log.Debug().Msgf("Servicing SQL query: %v", sqlQuery)

	result := pb.ClientCommandResponse{}

	rows, err := raftServer.sqlDb.Query(sqlQuery)
	if err != nil {
		log.Warn().Msgf("Sql query error: %v", err)
		result.ResponseStatus = uint32(codes.Aborted)
		result.QueryResponse = err.Error()
		event.responseChan <- &result
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		log.Warn().Msgf("Sql cols query error: %v", err)
		result.ResponseStatus = uint32(codes.Aborted)
		result.QueryResponse = err.Error()
		event.responseChan <- &result
		return
	}

	columnData := make([]string, len(columns))

	rawData := make([][]byte, len(columns))
	tempData := make([]interface{}, len(columns))
	for i := range rawData {
		tempData[i] = &rawData[i]
	}

	var buf bytes.Buffer
	for rows.Next() {
		err = rows.Scan(tempData...)
		if err != nil {
			log.Warn().Msgf("Sql query error. Partial data return: %v", err)
			continue
		}

		for i, val := range rawData {
			if val == nil {
				columnData[i] = ""
			} else {
				columnData[i] = string(val)
			}
		}
		buf.WriteString(strings.Join(columnData, " | ") + "\n")
	}

	// Combine column data into one string.
	result.QueryResponse = buf.String()

	result.ResponseStatus = uint32(codes.OK)
	event.responseChan <- &result

}

func (raftServer *Server) handleClientMutateCommand(event *RaftClientCommandRpcEvent) {
	result := pb.ClientCommandResponse{}
	// From Section 5.3 We need to do the following
	// 1) Append command to our log as a new entry
	// 2) Issue AppendEntries RPC in parallel to each of the other other nodes
	// 3) When Get majority successful responses, apply the new entry to our state
	//   machine, and reply to client.
	// Note: If some other followers slow, we apply append entries rpcs indefinitely.
	raftServer.appendCommandToLocalLog(event)
	replicationSuccess := raftServer.IssueAppendEntriesRpcToMajorityNodes(event)
	if replicationSuccess {
		log.Debug().Msgf("Command replicated successfully")
		result.ResponseStatus = uint32(codes.OK)
		raftServer.ApplyCommandToStateMachine(event)
	} else {
		log.Error().Msgf("Failed to replicate command")
		result.ResponseStatus = uint32(codes.Aborted)
	}
	event.responseChan <- &result
}

// Applies the command to the local state machine. For us this, this is to apply the
// sql command.
func (raftServer *Server) ApplyCommandToStateMachine(event *RaftClientCommandRpcEvent) {
	log.Debug().Msgf("Update State machine with command: %v", event.request.Command)
	raftServer.ApplySqlCommand(event.request.Command)
}

func (raftServer *Server) ApplySqlCommand(sqlCommand string) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.ApplySqlCommandLocked(sqlCommand)
}

func (raftServer *Server) ApplySqlCommandLocked(sqlCommand string) {
	_, err := raftServer.sqlDb.Exec(sqlCommand)
	if err != nil {
		log.Warn().Msgf("Sql application execution warning: %v", err)
	}
}

// sql database which is the state machine for this node.
func (raftServer *Server) GetSqliteReplicatedStateMachineOpenPath() string {
	// Want this to point to in-memory database. We'll replay raft log entries
	// to bring db upto speed.
	//const sqlOpenPath = "file::memory:?mode=memory&cache=shared"
	//return sqlOpenPath

	// uncomment to get persisted file.
	return "./sqlite-db-" + strings.Replace(raftServer.GetLocalNodeId(), ":", "-", -1) + ".db"
}

// Returns database path to use for the raft log.
func (raftServer *Server) GetSqliteRaftLogPath() string {
	localId := raftServer.GetLocalNodeId()
	localId = strings.Replace(localId, ":", "-", -1)
	return "./sqlite-raft-log-" + localId + ".db"
}

// Issues append entries rpc to replicate command to majority of nodes and returns
// true on success.
func (raftServer *Server) IssueAppendEntriesRpcToMajorityNodes(event *RaftClientCommandRpcEvent) bool {

	otherNodes := raftServer.GetOtherNodes()

	// Make RPCs in parallel but wait for all of them to complete.
	var waitGroup sync.WaitGroup
	waitGroup.Add(len(otherNodes))

	numOtherNodeSuccessRpcs := int32(0)
	leaderLastLogIndex := raftServer.GetLastLogIndex()
	for i, node := range otherNodes {
		// Pass a copy of node to avoid a race condition.
		go func(i int, node pb.RaftClient) {
			defer waitGroup.Done()
			success := raftServer.IssueAppendEntriesRpcToNode(event.request, node)
			if success {
				atomic.AddInt32(&numOtherNodeSuccessRpcs, 1)
				raftServer.SetMatchIndexForServerAt(i, leaderLastLogIndex)
				raftServer.SetNextIndexForServerAt(i, leaderLastLogIndex+1)
			}
		}(i, node)
	}

	waitGroup.Wait()

	// +1 to include the copy at the primary as well.
	numReplicatedData := int64(numOtherNodeSuccessRpcs) + 1
	return numReplicatedData >= raftServer.GetQuorumSize()
}

// Issues an append entries rpc to given raft client and returns true upon success
func (raftServer *Server) IssueAppendEntriesRpcToNode(request *pb.ClientCommandRequest, client pb.RaftClient) bool {
	if !raftServer.IsLeader() {
		return false
	}
	currentTerm := raftServer.RaftCurrentTerm()
	appendEntryRequest := pb.AppendEntriesRequest{}
	appendEntryRequest.Term = currentTerm
	appendEntryRequest.LeaderId = raftServer.GetLocalNodeId()
	appendEntryRequest.PrevLogIndex = raftServer.GetLeaderPreviousLogIndex()
	appendEntryRequest.PrevLogTerm = raftServer.GetLeaderPreviousLogTerm()
	appendEntryRequest.LeaderCommit = raftServer.GetCommitIndex()

	newEntry := pb.LogEntry{}
	newEntry.Term = currentTerm
	newEntry.Data = request.Command

	appendEntryRequest.Entries = append(appendEntryRequest.Entries, &newEntry)

	log.Debug().Msgf("Sending appending entry RPC: %v", appendEntryRequest.String())
	result, err := client.AppendEntries(context.Background(), &appendEntryRequest)
	if err != nil {
		log.Error().Msgf("Error issuing append entry to node: %v err:%v", client, err)
		return false
	}
	if result.ResponseStatus != uint32(codes.OK) {
		log.Error().Msgf("Error issuing append entry to node: %v response code:%v", client, result.ResponseStatus)
		return false
	}

	log.Debug().Msgf("AppendEntry Response from node: %v response: %v", client, result.String())
	if result.Term > raftServer.RaftCurrentTerm() {
		raftServer.ChangeToFollowerIfTermStale(result.Term)
		return false
	}

	// Return result of whether node accepted new log entry.
	return result.Success
}

func (raftServer *Server) ChangeToFollowerIfTermStale(theirTerm int64) {
	if theirTerm > raftServer.RaftCurrentTerm() {
		log.Debug().Msgf("Changing to follower status because term stale")
		raftServer.ChangeToFollowerStatus()
		raftServer.SetRaftCurrentTerm(theirTerm)
	}
}

func (raftServer *Server) appendCommandToLocalLog(event *RaftClientCommandRpcEvent) {
	currentTerm := raftServer.RaftCurrentTerm()
	newLogEntry := pb.LogEntry{
		Data: event.request.Command,
		Term: currentTerm,
	}
	raftServer.AddPersistentLogEntry(&newLogEntry)
}

// Handles request vote rpc.
func (raftServer *Server) handleRequestVoteRpc(event *RaftRequestVoteRpcEvent) {
	result := pb.RequestVoteResponse{}
	currentTerm := raftServer.RaftCurrentTerm()

	theirTerm := event.request.Term
	if theirTerm > currentTerm {
		raftServer.ChangeToFollowerStatus()
		raftServer.SetRaftCurrentTerm(theirTerm)
		currentTerm = theirTerm
	}

	result.Term = currentTerm
	if event.request.Term < currentTerm {
		result.VoteGranted = false
	} else if raftServer.AlreadyVoted() {
		result.VoteGranted = false
	} else {
		// Only grant vote if candidate log at least uptodate
		// as receivers (Section 5.2; 5.4)
		if raftServer.GetLastLogTerm() > event.request.LastLogTerm {
			// Node is more up to date, deny vote.
			result.VoteGranted = false
		} else if raftServer.GetLastLogTerm() < event.request.LastLogTerm {
			// Their term is more current, grant vote
			result.VoteGranted = true
		} else {
			// Terms match. Let's check log index to see who has longer log history.
			if raftServer.GetLastLogIndex() > event.request.LastLogIndex {
				// Node is more current, deny vote
				result.VoteGranted = false
			} else {
				result.VoteGranted = true
			}
		}
		log.Debug().Msgf("Grant vote to other server (%v) at term: %v ? %v", event.request.CandidateId, currentTerm, result.VoteGranted)
	}
	result.ResponseStatus = uint32(codes.OK)
	event.responseChan <- &result
}

// Heartbeat sent by leader. Special case of Append Entries with no log entries.
func (raftServer *Server) handleHeartBeatRpc(event *RaftAppendEntriesRpcEvent) {
	result := pb.AppendEntriesResponse{}
	currentTerm := raftServer.RaftCurrentTerm()
	result.Term = currentTerm
	// Main thing is to reset the election timeout.
	result.ResponseStatus = uint32(codes.OK)
	raftServer.ResetElectionTimeOut()
	raftServer.SetReceivedHeartBeat()

	// And update our leader id if necessary.
	raftServer.SetLeaderId(event.request.LeaderId)

	// Also advance commit pointer as appropriate.
	if event.request.LeaderCommit > raftServer.GetCommitIndex() {
		newCommitIndex := min(event.request.LeaderCommit, int64(len(raftServer.GetPersistentRaftLog())))
		raftServer.MoveCommitIndexTo(newCommitIndex)
	}

	result.Success = true
	event.responseChan <- &result
}

// Handles append entries rpc.
func (raftServer *Server) handleAppendEntriesRpc(event *RaftAppendEntriesRpcEvent) {

	// Convert to follower status if term in rpc is newer than ours.
	currentTerm := raftServer.RaftCurrentTerm()
	theirTerm := event.request.Term
	if theirTerm > currentTerm {
		log.Warn().Msgf("Append Entries Rpc processing: switching to follower")
		raftServer.ChangeToFollowerStatus()
		raftServer.SetRaftCurrentTerm(theirTerm)
		currentTerm = theirTerm
	} else if theirTerm < currentTerm {
		log.Warn().Msgf("Reject Append Entries Rpc because leader term stale")
		// We want to reply false here as leader term is stale.
		result := pb.AppendEntriesResponse{}
		result.Term = currentTerm
		result.ResponseStatus = uint32(codes.OK)
		result.Success = false
		event.responseChan <- &result
		return
	}

	isHeartBeatRpc := len(event.request.Entries) == 0
	if isHeartBeatRpc {
		raftServer.handleHeartBeatRpc(event)
		return
	}

	// Otherwise process regular append entries rpc (receiver impl).
	log.Debug().Msgf("Processing received AppendEntry rpc: %v", event.request.String())
	if len(event.request.Entries) > 1 {
		log.Warn().Msgf("Server sent more than one log entry in append entries rpc")
	}

	result := pb.AppendEntriesResponse{}
	result.Term = currentTerm
	result.ResponseStatus = uint32(codes.OK)

	// Want to reply false if log does not contain entry at prevLogIndex
	// whose term matches prevLogTerm.
	prevLogIndex := event.request.PrevLogIndex
	prevLogTerm := event.request.PrevLogTerm

	raftLog := raftServer.GetPersistentRaftLog()
	if prevLogIndex > 0 {
		// Note: log index is 1-based, and so is prevLogIndex.
		containsEntryAtPrevLogIndex := prevLogIndex <= int64(len(raftLog)) // prevLogIndex <= len(raftLog)
		if !containsEntryAtPrevLogIndex {
			log.Debug().Msgf("Rejecting append entries rpc because we don't have previous log entry at index: %v", prevLogIndex)
			result.Success = false
			event.responseChan <- &result
			return
		}
		// So, we have an entry at that position. Confirm that the terms match.
		// We want to reply false if the terms do not match at that position.
		prevLogIndexZeroBased := prevLogIndex - 1 // -1 because log index is 1-based.
		ourLogEntryTerm := raftLog[prevLogIndexZeroBased].LogEntry.Term
		entryTermsMatch := ourLogEntryTerm == prevLogTerm
		if !entryTermsMatch {
			log.Debug().Msgf("Rejecting append entries rpc because log terms don't match. Ours: %v, theirs: %v", ourLogEntryTerm, prevLogTerm)
			result.Success = false
			event.responseChan <- &result
			return
		}
	}
	// Delete log entries that conflict with those from leader.
	newEntry := event.request.Entries[0]
	newLogIndex := prevLogIndex + 1
	newLogIndexZeroBased := newLogIndex - 1
	containsEntryAtNewLogIndex := newLogIndex <= int64(len(raftLog))
	if containsEntryAtNewLogIndex {
		// Check whether we have a conflict (terms differ).
		ourEntryTerm := raftLog[newLogIndexZeroBased].LogEntry.Term
		theirEntry := newEntry

		haveConflict := ourEntryTerm != theirEntry.Term
		if haveConflict {
			// We must make our logs match the leader. Thus, we need to delete
			// all our entries starting from the new entry position.
			raftServer.DeletePersistentLogEntryInclusive(newLogIndex)
			raftServer.AddPersistentLogEntry(newEntry)
			result.Success = true
		} else {
			// We do not need to add any new entries to our log, because existing
			// one already matches the leader.
			result.Success = true
		}
	} else {
		// We need to insert new entry into the log.
		raftServer.AddPersistentLogEntry(newEntry)
		result.Success = true

	}

	// Last thing we do is advance our commit pointer.

	if event.request.LeaderCommit > raftServer.GetCommitIndex() {
		newCommitIndex := min(event.request.LeaderCommit, newLogIndex)
		raftServer.MoveCommitIndexTo(newCommitIndex)
	}

	event.responseChan <- &result
}

func (raftServer *Server) min(a, b int64) int64 {
	if a < b {
		return a
	} else {
		return b
	}
}

// Returns other nodes client connections
func (raftServer *Server) GetOtherNodes() []pb.RaftClient {
	return raftServer.otherNodes
}

// Increments the number of received votes.
func (raftServer *Server) IncrementVoteCount() {
	atomic.AddInt64(&raftServer.receivedVoteCount, 1)
}

// Returns the index of the last entry in the raft log. Index is 1-based.
func (raftServer *Server) GetLastLogIndex() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.GetLastLogIndexLocked()
}

func (raftServer *Server) GetLastLogIndexLocked() int64 {
	raftLog := raftServer.raftState.persistentState.log
	if len(raftLog) <= 0 {
		return 0
	}
	lastItem := raftLog[len(raftLog)-1].LogIndex

	if lastItem != int64(len(raftLog)) {
		// TODO: Remove this sanity check ...
		log.Error().Msgf("Mismatch between stored log index value and # of entries. Last stored log index: %v num entries: %v ", lastItem, len(raftLog))
	}
	return lastItem
}

// Returns the term for the last entry in the raft log.
func (raftServer *Server) GetLastLogTerm() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.GetLastLogTermLocked()
}

func (raftServer *Server) GetLastLogTermLocked() int64 {
	raftLog := raftServer.raftState.persistentState.log
	if len(raftLog) <= 0 {
		return 0
	}

	return raftLog[len(raftLog)-1].LogEntry.Term
}

// Updates raft current term to a new one.
func (raftServer *Server) SetRaftCurrentTerm(term int64) {
	currentTerm := raftServer.RaftCurrentTerm()
	if term < currentTerm {
		log.Error().Msgf("Trying to update to the  lesser term: %v current: %v", term, currentTerm)
	} else if term == currentTerm {
		// Concurrent rpcs can lead to duplicated attempts to update terms.
		return
	}

	// Note: Be wary of calling functions that also take the lock as
	// golang locks are not reentrant.
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.SetPersistentCurrentTermLocked(term)

	// Since it's a new term reset who voted for and if heard heartbeat from leader as candidate.
	raftServer.SetPersistentVotedForLocked("")
	raftServer.ResetReceivedVoteCount()
	raftServer.SetReceivedHeartbeatLocked(false)
}

// Requests votes from all the other nodes to make us a leader. Returns number of
// currently received votes
func (raftServer *Server) RequestVotesFromOtherNodes() int64 {

	log.Debug().Msgf("Have %v votes at start", raftServer.GetVoteCount())
	otherNodes := raftServer.GetOtherNodes()
	log.Debug().Msgf("Requesting votes from other nodes: %v", raftServer.GetOtherNodes())

	// Make RPCs in parallel but wait for all of them to complete.
	var waitGroup sync.WaitGroup
	waitGroup.Add(len(otherNodes))

	for _, node := range otherNodes {
		// Pass a copy of node to avoid a race condition.
		go func(node pb.RaftClient) {
			defer waitGroup.Done()
			raftServer.RequestVoteFromNode(node)
		}(node)
	}

	waitGroup.Wait()
	log.Debug().Msgf("Have %v votes at end", raftServer.GetVoteCount())
	return raftServer.GetVoteCount()
}

// Requests a vote from the given node.
func (raftServer *Server) RequestVoteFromNode(node pb.RaftClient) {
	if raftServer.GetServerState() != Candidate {
		return
	}

	voteRequest := pb.RequestVoteRequest{}
	voteRequest.Term = raftServer.RaftCurrentTerm()
	voteRequest.CandidateId = raftServer.GetLocalNodeId()
	voteRequest.LastLogIndex = raftServer.GetLastLogIndex()
	voteRequest.LastLogTerm = raftServer.GetLastLogTerm()

	result, err := node.RequestVote(context.Background(), &voteRequest)
	if err != nil {
		log.Error().Msgf("Error getting vote from node %v err: %v", node, err)
		return
	}
	if result.ResponseStatus != uint32(codes.OK) {
		log.Error().Msgf("Error with vote rpc entry to node: %v response code:%v", node, result.ResponseStatus)
		return
	}
	log.Debug().Msgf("Vote response: %v", result.ResponseStatus)
	if result.VoteGranted {
		raftServer.IncrementVoteCount()
	}
	// Change to follower status if our term is stale.
	if result.Term > raftServer.RaftCurrentTerm() {
		log.Debug().Msgf("Changing to follower status because term stale")
		raftServer.ChangeToFollowerStatus()
		raftServer.SetRaftCurrentTerm(result.Term)
	}
}

// Changes to Follower status.
func (raftServer *Server) ChangeToFollowerStatus() {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.serverState = Follower
}

// Converts the node to a leader status from a candidate
func (raftServer *Server) ChangeToLeaderStatus() {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.serverState = Leader

}

// Instructions that candidate would be processing.
func (raftServer *Server) CandidateLoop() {
	// High level notes overview:
	// Start an election process
	// - Increment current election term
	// - Vote for yourself
	// - Request votes in parallel from others nodes in cluster
	//
	// Remain a candidate until any of the following happens:
	// i) You win election (got enough votes) -> become leader
	// ii) Hear from another leader -> become follower
	// iii) A period of time goes by with no winner.
	log.Debug().Msgf("Starting candidate loop")
	for {
		if raftServer.GetServerState() != Candidate {
			log.Debug().Msgf("Stopping candidate loop")
			return
		}
		raftServer.IncrementElectionTerm()
		log.Debug().Msgf("Starting new election term: %v", raftServer.RaftCurrentTerm())
		raftServer.VoteForSelf()
		raftServer.RequestVotesFromOtherNodes()

		if raftServer.HaveEnoughVotes() {
			raftServer.ChangeToLeaderStatus()
			return
		}

		// If we don't have enough votes, it possible that:
		// a) Another node became a leader
		// b) Split votes, no node got majority.
		//
		// For both cases, wait out a little bit before starting another election.
		// This gives time to see if we hear from another leader (processing heartbeats)
		// and also reduces chance of continual split votes since each node has a random
		// timeout.
		log.Debug().Msgf("Potential split votes/not enough votes. Performing Randomized wait.")
		timeoutTimer := raftServer.RandomizedElectionTimeout()
		timeoutDone := false
	CandidateLoop:
		for {
			// While processing RPCs below, we may convert from candidate status to follower
			if raftServer.GetServerState() != Candidate {
				log.Debug().Msgf("Stopping candidate loop. Exit from inner loop")
				return
			}
			if timeoutDone {
				break
			}
			if raftServer.GetReceivedHeartbeat() {
				// We have another leader and should convert to follower status.
				log.Debug().Msgf("Heard from another leader. Converting to follower status")
				raftServer.ChangeToFollowerStatus()
				return
			}
			select {
			case event := <-raftServer.events:
				raftServer.handleRpcEvent(event)
			case <-timeoutTimer.C:
				break CandidateLoop
			}
		}
	}
}

// Reinitializes volatile leader state
func (raftServer *Server) ReinitVolatileLeaderState() {
	if !raftServer.IsLeader() {
		return
	}
	log.Debug().Msgf("Reinitialized Leader state")
	volatileLeaderState := &raftServer.raftState.volatileLeaderState

	// Reset match index to 0.
	numOtherNodes := len(raftServer.GetOtherNodes())
	volatileLeaderState.matchIndex = make([]int64, numOtherNodes)
	for i := range volatileLeaderState.matchIndex {
		volatileLeaderState.matchIndex[i] = 0
	}

	// Reset next index to leader last log index + 1.
	newVal := raftServer.GetLastLogIndex() + 1
	volatileLeaderState.nextIndex = make([]int64, numOtherNodes)
	for i := range volatileLeaderState.nextIndex {
		volatileLeaderState.nextIndex[i] = newVal
	}
	log.Debug().Msgf("After init: nextIndex len: %v", len(raftServer.raftState.volatileLeaderState.nextIndex))
}

// serverIndex is index into otherNodes array.
func (raftServer *Server) GetNextIndexForServerAt(serverIndex int) int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	log.Debug().Msgf("Get next index, serverIdex:%v nextIndex Len: %v", serverIndex, len(raftServer.raftState.volatileLeaderState.nextIndex))
	return raftServer.raftState.volatileLeaderState.nextIndex[serverIndex]
}

func (raftServer *Server) SetNextIndexForServerAt(serverIndex int, newValue int64) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.raftState.volatileLeaderState.nextIndex[serverIndex] = newValue
}

func (raftServer *Server) DecrementNextIndexForServerAt(serverIndex int) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.raftState.volatileLeaderState.nextIndex[serverIndex] -= 1
}

func (raftServer *Server) GetMatchIndexForServerAt(serverIndex int) int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.raftState.volatileLeaderState.matchIndex[serverIndex]
}

func (raftServer *Server) SetMatchIndexForServerAt(serverIndex int, newValue int64) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.raftState.volatileLeaderState.matchIndex[serverIndex] = newValue
}

func (raftServer *Server) GetServerState() ServerState {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.serverState
}

// Returns the configured interval at which leader sends heartbeat rpcs.
func (raftServer *Server) GetHeartbeatIntervalMillis() int64 {
	return raftServer.raftConfig.heartBeatIntervalMillis

}

// Instructions that leaders would be performing.
func (raftServer *Server) LeaderLoop() {
	// TODO: implement.
	// Overview:
	// - Reinitialize volatile leader state upon first leader succession.
	// - Send initial empty append entries rpcs to clients as heartbeats. Repeat
	//   to avoid election timeout.
	// - Process commands from end-user clients. Respond after data replicated on
	//   majority of nodes. i.e. append to local log, respond after entry applied to
	//   state machine.
	// - See Figure 2 from Raft paper for 2 other leader requirements.
	// - Also change to follower status if term is stale in rpc request/response
	log.Debug().Msgf("Starting leader loop")
	raftServer.ReinitVolatileLeaderState()

	// Send heartbeats to followers in the background.
	go func() {
		log.Debug().Msgf("Starting to Send heartbeats to followers in background")
		for {
			if raftServer.GetServerState() != Leader {
				log.Debug().Msgf("No longer leader. Stopping heartbeat rpcs")
				return
			}
			raftServer.SendHeartBeatsToFollowers()
			time.Sleep(time.Duration(raftServer.GetHeartbeatIntervalMillis()) * time.Millisecond)
		}
	}()

	// Send Append Entries rpcs to followers to replicate our logs.
	go func() {
		log.Debug().Msgf("Starting to Send append entries rpcs to followers in background")
		for {
			if raftServer.GetServerState() != Leader {
				log.Debug().Msgf("No longer leader. Stopping append entries replication rpcs")
				return
			}
			raftServer.SendAppendEntriesReplicationRpcToFollowers()
			time.Sleep(time.Duration(raftServer.GetHeartbeatIntervalMillis()) * time.Millisecond)
		}
	}()

	for {
		// While processing RPC, we may learn we no longer a valid leader.
		if raftServer.GetServerState() != Leader {
			log.Debug().Msgf("Stopping leader loop")
			return
		}
		raftServer.handleRpcEvent(<-raftServer.events)
	}
}

// Sends any append entries replication rpc needed to all followers to get their
// state to match ours.
func (raftServer *Server) SendAppendEntriesReplicationRpcToFollowers() {
	// TODO: implement
	otherNodes := raftServer.GetOtherNodes()

	// Make RPCs in parallel but wait for all of them to complete.
	var waitGroup sync.WaitGroup
	waitGroup.Add(len(otherNodes))

	for i, node := range otherNodes {
		// Pass a copy of node to avoid a race condition.
		go func(i int, node pb.RaftClient) {
			defer waitGroup.Done()
			raftServer.SendAppendEntriesReplicationRpcForFollower(i, node)
		}(i, node)
	}

	waitGroup.Wait()

	// After replicating, increment the commit index if its possible.
	raftServer.IncrementCommitIndexIfPossible()
}

func (raftServer *Server) IncrementCommitIndexIfPossible() {
	proposedCommitIndex := raftServer.GetCommitIndex() + 1
	if proposedCommitIndex > int64(len(raftServer.GetPersistentRaftLog())) {
		return
	}

	numNodesWithMatchingLogs := 0
	numNodes := len(raftServer.GetOtherNodes())
	for i := 0; i < numNodes; i++ {
		if raftServer.GetMatchIndexForServerAt(i) >= proposedCommitIndex {
			numNodesWithMatchingLogs++
		}
	}
	otherNodesMajority := raftServer.GetQuorumSize() - 1 // -1 because we don't count primary.
	majorityMet := int64(numNodesWithMatchingLogs) >= otherNodesMajority
	if !majorityMet {
		return
	}

	// Check that for proposed commit index, term is current.
	proposedCommitIndexZeroBased := proposedCommitIndex - 1
	proposedCommitLogEntry := raftServer.GetPersistentRaftLogEntryAt(proposedCommitIndexZeroBased)
	currentTerm := raftServer.RaftCurrentTerm()
	if proposedCommitLogEntry.LogEntry.Term == currentTerm {
		raftServer.SetCommitIndex(proposedCommitIndex)
	}

}

// If needed, sends append entries rpc to given "client" follower to make their logs match ours.
func (raftServer *Server) SendAppendEntriesReplicationRpcForFollower(serverIndex int, client pb.RaftClient) {
	if !raftServer.IsLeader() {
		return
	}

	lastLogIndex := raftServer.GetLastLogIndex()
	nextIndex := raftServer.GetNextIndexForServerAt(serverIndex)

	if lastLogIndex < nextIndex {
		// Nothing to do for this follower - it's already up to date.
		return
	}

	// Otherwise, We need to send append entry rpc with log entries starting
	// at nextIndex. For now, just send one at a time.
	// TODO: Consider batching the rpcs for improved efficiency.
	request := pb.AppendEntriesRequest{}
	nextIndexZeroBased := nextIndex - 1

	if nextIndexZeroBased < 0 || nextIndexZeroBased >= int64(len(raftServer.GetPersistentRaftLog())) {
		// TODO: See if we can avoid hack fix:
		log.Warn().Msgf("nextIndexZeroBased is out of range: %v", nextIndexZeroBased)
		return
	}
	logEntryToSend := raftServer.GetPersistentRaftLogEntryAt(nextIndexZeroBased)
	if nextIndexZeroBased >= 1 {
		priorLogEntry := raftServer.GetPersistentRaftLogEntryAt(nextIndexZeroBased - 1)
		request.PrevLogIndex = priorLogEntry.LogIndex
		request.PrevLogTerm = priorLogEntry.LogEntry.Term
	} else {
		// They don't exist.
		request.PrevLogIndex = 0
		request.PrevLogTerm = 0
	}

	currentTerm := raftServer.RaftCurrentTerm()
	request.Term = currentTerm
	request.LeaderId = raftServer.GetLocalNodeId()

	request.LeaderCommit = raftServer.GetCommitIndex()

	request.Entries = append(request.Entries, logEntryToSend.LogEntry)

	result, err := client.AppendEntries(context.Background(), &request)
	if err != nil {
		log.Error().Msgf("Error issuing append entry to get followers to match our state. note: %v, err: %v", client, err)
		return
	}
	if result.ResponseStatus != uint32(codes.OK) {
		log.Error().Msgf("Error response issuing append entry to get followers to match our state. note: %v, err: %v", client, err)
		return
	}
	if result.Term > currentTerm {
		raftServer.ChangeToFollowerIfTermStale(result.Term)
		return
	}

	if result.Success {
		// We can update nextIndex and matchIndex for the follower.
		raftServer.SetNextIndexForServerAt(serverIndex, logEntryToSend.LogIndex+1)
		raftServer.SetMatchIndexForServerAt(serverIndex, logEntryToSend.LogIndex)
	} else {
		// RPC failed, so decrement nextIndex. We will try again later automatically
		// replication attempts are called in a loop.
		raftServer.DecrementNextIndexForServerAt(serverIndex)
	}
}

// Send heart beat rpcs to followers in parallel and waits for them to all complete.
func (raftServer *Server) SendHeartBeatsToFollowers() {
	otherNodes := raftServer.GetOtherNodes()

	var waitGroup sync.WaitGroup
	waitGroup.Add(len(otherNodes))

	for _, node := range otherNodes {
		// Send RPCs in parallel. Pass copy of node to avoid race conditions.
		go func(node pb.RaftClient) {
			defer waitGroup.Done()
			raftServer.SendHeartBeatRpc(node)
		}(node)
	}
	waitGroup.Wait()
}

// Sends a heartbeat rpc to the given raft node.
func (raftServer *Server) SendHeartBeatRpc(node pb.RaftClient) {
	request := pb.AppendEntriesRequest{}
	request.Term = raftServer.RaftCurrentTerm()
	request.LeaderId = raftServer.GetLocalNodeId()
	request.LeaderCommit = raftServer.GetCommitIndex()

	// Log entries are empty/nil for heartbeat rpcs, so no need to
	// set previous log index, previous log term.
	request.Entries = nil

	result, err := node.AppendEntries(context.Background(), &request)
	if err != nil {
		log.Error().Msgf("Error sending hearbeat to node: %v Error: %v", node, err)
		return
	}
	log.Debug().Msgf("Heartbeat RPC Response from node: %v Response: %v", node, result.ResponseStatus)
}

// PrevLogTerm value  used in the appendentries rpc request. Should be called _after_ local local updated.
func (raftServer *Server) GetLeaderPreviousLogTerm() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	// Because we would have just stored a new entry to our local log, when this
	// method is called, the previous entry is the one before that.
	raftLog := raftServer.raftState.persistentState.log
	lastEntryIndex := len(raftLog) - 1
	if lastEntryIndex <= 0 {
		return 0
	}
	previousEntryIndex := lastEntryIndex - 1
	previousEntry := raftLog[previousEntryIndex]
	return previousEntry.LogEntry.Term
}

// PrevLogIndex value  used in the appendentries rpc request. Should be called _after_ local log
// already updated.
func (raftServer *Server) GetLeaderPreviousLogIndex() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	// Because we would have just stored a new entry to our local log, when this
	// method is called, the previous entry is the one before that.
	raftLog := raftServer.raftState.persistentState.log
	if len(raftLog) <= 1 {
		return 0
	}
	lastEntryIndex := len(raftLog) - 1
	previousEntryIndex := lastEntryIndex - 1
	previousEntry := raftLog[previousEntryIndex]
	return previousEntry.LogIndex
}

// Leader commit value used in the appendentries rpc request.
func (raftServer *Server) GetCommitIndex() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.raftState.volatileState.commitIndex
}

func (raftServer *Server) GetLastApplied() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.raftState.volatileState.lastApplied
}

func (raftServer *Server) SetLastApplied(newValue int64) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.raftState.volatileState.lastApplied = newValue
}

func (raftServer *Server) SetCommitIndex(newValue int64) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	if raftServer.raftState.volatileState.commitIndex > newValue {
		log.Error().Msgf("Trying to set commit index backwards")
	}
	raftServer.raftState.volatileState.commitIndex = newValue
}

// Overall loop for the server.
func (raftServer *Server) StartServerLoop() {

	for {
		serverState := raftServer.GetServerState()
		if serverState == Leader {
			raftServer.LeaderLoop()
		} else if serverState == Follower {
			raftServer.FollowerLoop()
		} else if serverState == Candidate {
			raftServer.CandidateLoop()
		} else {
			log.Error().Msgf("Unexpected / unknown server state: %v", serverState)
		}
	}
}

// Returns the current time since unix epoch in milliseconds.
func UnixMillis() int64 {
	now := time.Now()
	unixNano := now.UnixNano()
	unixMillis := unixNano / 1000000
	return unixMillis
}
