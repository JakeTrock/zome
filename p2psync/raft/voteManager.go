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

// Returns true if this node already voted for a node to be a leader.
func (raftServer *Server) AlreadyVoted() bool { 
	return raftServer.GetPersistentVotedFor() != ""
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
	return raftServer.GetVoteCount() >= raftServer.GetQuorumSize();
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

func (raftServer *Server) ChangeToFollowerIfTermStale(theirTerm int64) {
	if theirTerm > raftServer.RaftCurrentTerm() {
		log.Debug().Msgf("Changing to follower status because term stale")
		raftServer.ChangeToFollowerStatus()
		raftServer.SetRaftCurrentTerm(theirTerm)
	}
}

// Increments the number of received votes.
func (raftServer *Server) IncrementVoteCount() {
	atomic.AddInt64(&raftServer.receivedVoteCount, 1)
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


