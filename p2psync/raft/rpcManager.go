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

func min(a, b int64) int64 {
	if a < b {
		return a
	} else {
		return b
	}
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
func (raftServer *Server) SendHeartBeatRpcsToFollowers() {
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
