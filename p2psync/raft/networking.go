package raft

import (
	"sync"
	"sync/atomic"

	"github.com/jaketrock/zome/sync/zproto"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/proto"
)

// AppendEntries implementation for zproto.RaftServer
func (raftServer *Server) AppendEntries(ctx context.Context, in *zproto.AppendEntriesRequest) (*zproto.AppendEntriesResponse, error) {
	replyChan := make(chan *zproto.AppendEntriesResponse)
	event := Event{
		rpc: RpcEvent{
			appendEntries: &RaftAppendEntriesRpcEvent{
				request:      proto.Clone(in).(*zproto.AppendEntriesRequest),
				responseChan: replyChan,
			},
		},
	}
	raftServer.events <- event

	result := <-replyChan
	return result, nil
}

// RequestVote implementation for raft.RaftServer
func (raftServer *Server) RequestVote(ctx context.Context, in *zproto.RequestVoteRequest) (*zproto.RequestVoteResponse, error) {
	replyChan := make(chan *zproto.RequestVoteResponse)
	event := Event{
		rpc: RpcEvent{
			requestVote: &RaftRequestVoteRpcEvent{
				request:      proto.Clone(in).(*zproto.RequestVoteRequest),
				responseChan: replyChan,
			},
		},
	}
	raftServer.events <- event

	result := <-replyChan
	return result, nil
}

// Client Command implementation for raft.RaftServer
func (raftServer *Server) ClientCommand(ctx context.Context, in *zproto.ClientCommandRequest) (*zproto.ClientCommandResponse, error) {
	replyChan := make(chan *zproto.ClientCommandResponse)
	event := Event{
		rpc: RpcEvent{
			clientCommand: &RaftClientCommandRpcEvent{
				request:      proto.Clone(in).(*zproto.ClientCommandRequest),
				responseChan: replyChan,
			},
		},
	}
	raftServer.events <- event

	result := <-replyChan
	return result, nil
}

// Variables

// Handle to the raft server.

// Connects to a Raft server listening at the given address and returns a client
// to talk to this server.
func (raftServer *Server) ConnectToServer(address string) zproto.RaftClient {
	_, c := raftServer.GetConnection(address)
	return c
}

// Starts a Raft Server listening at the specified local node.
// otherNodes contain contact information for other nodes in the cluster.
func (raftServer *Server) StartServer(localNode Node, otherNodes []Node) *grpc.Server {
	lis, err := raftServer.GetListener(localNode.Port)
	if err != nil {
		log.Error().Msgf("Failed to get listener for server at: %v", localNode)
	}
	log.Debug().Msgf("Created Raft server at: %v", lis.Addr().String())
	s := grpc.NewServer()

	log.Printf("Initial Server state: %v", raftServer.serverState)

	zproto.RegisterRaftServer(s, raftServer)
	// Register reflection service on gRPC server.
	reflection.Register(s)

	// Initialize raft cluster.
	raftServer.otherNodes = raftServer.ConnectToOtherNodes(otherNodes)
	raftServer.localNode = localNode //TODO: stop using/revise node type, be more abstract for future nw abstractions
	go func() {
		raftServer.InitializeDatabases()
		raftServer.StartElectionLoop()
	}()

	// Note: the Serve call is blocking.
	if err := s.Serve(lis); err != nil {
		log.Error().Msgf("Failed to serve on: %v", localNode.Port)
	}

	return s
}

func (raftServer *Server) NodeToAddressString(input Node) string {
	return input.Hostname + ":" + input.Port
}

// Connects to the other Raft nodes and returns array of Raft Client connections.
func (raftServer *Server) ConnectToOtherNodes(otherNodes []Node) []zproto.RaftClient {

	result := make([]zproto.RaftClient, 0)
	for _, node := range otherNodes {
		serverAddress := raftServer.NodeToAddressString(node)
		log.Printf("Connecting to server: %v", serverAddress)
		client := raftServer.ConnectToServer(serverAddress)
		result = append(result, client)
	}
	return result
}

func (raftServer *Server) TestNodeConnections(nodeConns []zproto.RaftClient) {
	// Try a test RPC call to other nodes.
	log.Printf("Have client conns: %v", nodeConns)
	for _, nodeConn := range nodeConns {
		result, err := nodeConn.RequestVote(context.Background(), &zproto.RequestVoteRequest{})
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

func EventString(rpcEvent RpcEvent) string {
	if rpcEvent.requestVote != nil {
		return "RequestVote"
	} else if rpcEvent.appendEntries != nil {
		return "AppendEntries"
	} else if rpcEvent.clientCommand != nil {
		return "ClientCommand"
	} else {
		return "Unknown"
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
		result := zproto.ClientCommandResponse{}
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
		result := zproto.ClientCommandResponse{}
		log.Warn().Msgf("Invalid client command (not command/query): %v", event)
		result.ResponseStatus = uint32(codes.InvalidArgument)
		event.responseChan <- &result
		return
	}

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
		go func(i int, node zproto.RaftClient) {
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
func (raftServer *Server) IssueAppendEntriesRpcToNode(request *zproto.ClientCommandRequest, client zproto.RaftClient) bool {
	if !raftServer.IsLeader() {
		return false
	}
	currentTerm := raftServer.RaftCurrentTerm()
	appendEntryRequest := zproto.AppendEntriesRequest{}
	appendEntryRequest.Term = currentTerm
	appendEntryRequest.LeaderId = raftServer.GetLocalNodeId()
	appendEntryRequest.PrevLogIndex = raftServer.GetLeaderPreviousLogIndex()
	appendEntryRequest.PrevLogTerm = raftServer.GetLeaderPreviousLogTerm()
	appendEntryRequest.LeaderCommit = raftServer.GetCommitIndex()

	newEntry := zproto.LogEntry{}
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

// Handles request vote rpc.
func (raftServer *Server) handleRequestVoteRpc(event *RaftRequestVoteRpcEvent) {
	result := zproto.RequestVoteResponse{}
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
	} else if raftServer.GetPersistentVotedFor() != "" { // check if this node has already voted
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
	result := zproto.AppendEntriesResponse{}
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
		result := zproto.AppendEntriesResponse{}
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

	result := zproto.AppendEntriesResponse{}
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

// Sends a heartbeat rpc to the given raft node.
func (raftServer *Server) SendHeartBeatRpc(node zproto.RaftClient) {
	request := zproto.AppendEntriesRequest{}
	request.Term = raftServer.RaftCurrentTerm()
	request.LeaderId = raftServer.GetLocalNodeId()
	request.LeaderCommit = raftServer.GetCommitIndex()

	// Log entries are empty/nil for heartbeat rpcs, so no need to
	// set previous log index, previous log term.
	request.Entries = nil

	result, err := node.AppendEntries(context.Background(), &request)
	if err != nil {
		log.Error().Msgf("Error sending heartbeat to node: %v Error: %v", node, err)
		return
	}
	log.Debug().Msgf("Heartbeat RPC Response from node: %v Response: %v", node, result.ResponseStatus)
}

// Requests votes from all the other nodes to make us a leader. Returns number of
// currently received votes
func (raftServer *Server) RequestVotesFromOtherNodes() int64 {
	log.Debug().Msgf("Have %v votes at start", raftServer.GetVoteCount())
	otherNodes := raftServer.GetOtherNodes()
	log.Debug().Msgf("Requesting votes from other nodes: %v", otherNodes)

	// Make RPCs in parallel but wait for all of them to complete.
	var waitGroup sync.WaitGroup
	waitGroup.Add(len(otherNodes))

	for _, node := range otherNodes {
		// Pass a copy of node to avoid a race condition.
		go func(node zproto.RaftClient) {
			defer waitGroup.Done()
			raftServer.RequestVoteFromNode(node)
		}(node)
	}

	waitGroup.Wait()
	log.Debug().Msgf("Have %v votes at end", raftServer.GetVoteCount())
	return raftServer.GetVoteCount()
}

// Requests a vote from the given node.
func (raftServer *Server) RequestVoteFromNode(node zproto.RaftClient) {
	if raftServer.GetServerState() != Candidate {
		return
	}

	voteRequest := zproto.RequestVoteRequest{}
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
