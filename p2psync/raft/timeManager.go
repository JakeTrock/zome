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
	return elapsedMs > timeoutMs
}

// Resets the election time out. This restarts amount of time that has to pass
// before an election timeout occurs. Election timeouts lead to new elections.
func (raftServer *Server) ResetElectionTimeOut() {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.lastHeartbeatTimeMillis = UnixMillis()
}

// Returns the configured interval at which leader sends heartbeat rpcs.
func (raftServer *Server) GetHeartbeatIntervalMillis() int64 {
	return raftServer.raftConfig.heartBeatIntervalMillis
}

// Returns the current time since unix epoch in milliseconds.
func UnixMillis() int64 {
	now := time.Now()
	unixNano := now.UnixNano()
	unixMillis := unixNano / 1000000
	return unixMillis
}