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
			raftServer.SendHeartBeatRpcsToFollowers()
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

// Overall loop for the server.
func (raftServer *Server) StartServerLoop() {
	for {
		serverState := raftServer.GetServerState()
		switch serverState {
			case Leader:
				raftServer.LeaderLoop()
			case Follower:
				raftServer.FollowerLoop()
			case Candidate:
				raftServer.CandidateLoop()
			default:
				log.Error().Msgf("Unexpected / unknown server state: %v", serverState)
		}
	}
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

// Returns the index of the last entry in the raft log. Index is 1-based.
func (raftServer *Server) GetLastLogIndex() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.GetLastLogIndexLocked()
}