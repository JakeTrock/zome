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

func (raftServer *Server) appendCommandToLocalLog(event *RaftClientCommandRpcEvent) {
	currentTerm := raftServer.RaftCurrentTerm()
	newLogEntry := pb.LogEntry{
		Data: event.request.Command,
		Term: currentTerm,
	}
	raftServer.AddPersistentLogEntry(&newLogEntry)
}

// Returns other nodes client connections
func (raftServer *Server) GetOtherNodes() []pb.RaftClient {
	return raftServer.otherNodes
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
