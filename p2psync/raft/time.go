package raft

import (
	"time"

	"github.com/rs/zerolog/log"
)

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

// Returns the current time since unix epoch in milliseconds.
func UnixMillis() int64 {
	now := time.Now()
	unixNano := now.UnixNano()
	unixMillis := unixNano / 1000000
	return unixMillis
}
