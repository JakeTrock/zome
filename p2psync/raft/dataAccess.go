package raft

import (
	"database/sql"
	"strconv"
	"sync/atomic"

	"github.com/jaketrock/zome/sync/zproto"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"
)

// Raft Persistent State accessors / getters / functions.
func (raftServer *Server) GetPersistentRaftLog() []*zproto.DiskLogEntry {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()
	return raftServer.raftState.persistentState.log
}

func (raftServer *Server) GetPersistentRaftLogEntryAt(index int64) *zproto.DiskLogEntry {
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

func (raftServer *Server) AddPersistentLogEntry(newValue *zproto.LogEntry) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.AddPersistentLogEntryLocked(newValue)
}

func (raftServer *Server) AddPersistentLogEntryLocked(newValue *zproto.LogEntry) {
	nextIndex := int64(len(raftServer.raftState.persistentState.log)) + 1
	diskEntry := zproto.DiskLogEntry{
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

// Returns other nodes client connections
func (raftServer *Server) GetOtherNodes() []zproto.RaftClient {
	return raftServer.otherNodes
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

// Returns the index of the last entry in the raft log. Index is 1-based.
func (raftServer *Server) GetLastLogIndex() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.GetLastLogIndexLocked()
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

func (raftServer *Server) appendCommandToLocalLog(event *RaftClientCommandRpcEvent) {
	currentTerm := raftServer.RaftCurrentTerm()
	newLogEntry := zproto.LogEntry{
		Data: event.request.Command,
		Term: currentTerm,
	}
	raftServer.AddPersistentLogEntry(&newLogEntry)
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
