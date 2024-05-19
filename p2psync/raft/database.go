package raft

import (
	"bytes"
	"database/sql"
	"strconv"
	"strings"

	"github.com/jaketrock/zome/sync/zproto"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

func (raftServer *Server) InitializeDatabases() { //TODO: abstract this so you can use any db/location
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

		var parsedLogEntry zproto.LogEntry
		err := proto.Unmarshal([]byte(logEntryProtoText), &parsedLogEntry)
		if err != nil {
			log.Error().Msgf("Error parsing log entry proto: %v", logEntryProtoText)
		}
		diskLogEntry := zproto.DiskLogEntry{
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

func (raftServer *Server) handleClientQueryCommand(event *RaftClientCommandRpcEvent) {
	sqlQuery := event.request.Query
	log.Debug().Msgf("Servicing SQL query: %v", sqlQuery)

	result := zproto.ClientCommandResponse{}

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
	result := zproto.ClientCommandResponse{}
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
