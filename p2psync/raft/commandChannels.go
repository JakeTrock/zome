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