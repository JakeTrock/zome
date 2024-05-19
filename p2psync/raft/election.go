package raft

import (
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jaketrock/zome/sync/zproto"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
)

// Overall loop for the server.
func (raftServer *Server) StartElectionLoop() {
	for {
		serverState := raftServer.GetServerState()
		if serverState == Leader {
			raftServer.LeaderLoop()
		} else if serverState == Follower {
			raftServer.FollowerLoop()
		} else if serverState == Candidate {
			raftServer.CandidateLoop()
		} else {
			log.Error().Msgf("Unexpected / unknown server state: %v", serverState)
		}
	}
}

// Returns true if this node has received sufficient votes to become a leader
func (raftServer *Server) HaveEnoughVotes() bool {
	return raftServer.GetVoteCount() >= raftServer.GetQuorumSize()
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
			raftServer.SendHeartBeatsToFollowers()
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
			log.Debug().Msgf("Processing rpc #%v event: %v", rpcCount, EventString(event.rpc))
			raftServer.handleRpcEvent(event)
			rpcCount++
		case <-timeoutTimer.C:
			// Election timeout occured w/o heartbeat from leader.
			raftServer.ChangeToCandidateStatus()
			return
		}
	}
}

// Returns true iff server is the leader for the cluster.
func (raftServer *Server) IsLeader() bool {
	return raftServer.GetServerState() == Leader
}

// Returns true iff server is a follower in the cluster
func (raftServer *Server) IsFollower() bool {
	return raftServer.GetServerState() == Follower
}

// Returns true iff server is a candidate
func (raftServer *Server) IsCandidate() bool {
	return raftServer.GetServerState() == Candidate
}

func GetDefaultServer() *Server {
	return &Server{
		serverState: Follower,
		events:      make(chan Event),
		// We initialize last heartbeat time at startup because all servers start out
		// in follower and this allows a node to determine when it should be a candidate.
		lastHeartbeatTimeMillis: UnixMillis(),
		raftConfig: RaftConfig{
			electionTimeoutMillis:   PickElectionTimeOutMillis(),
			heartBeatIntervalMillis: 10,
		},
		GetConnection: func(target string) (*grpc.ClientConn, zproto.RaftClient) {
			// Set up a connection to the server. Note: this is not a blocking call.
			// Connection will be setup in the background.
			//TODO: remove deprecated grpc.WithInsecure() option
			conn, err := grpc.Dial(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Error().Msgf("Failed to connect to server at: %v", target)
				return nil, nil
			} else {
				log.Debug().Msgf("Connected to Raft server at: %v", target)
				c := zproto.NewRaftClient(conn)
				return conn, c
			}
		},
		GetListener: func(identifier string) (net.Listener, error) {
			addressPort := ":" + identifier
			lis, err := net.Listen("tcp", addressPort)
			if err != nil {
				log.Error().Msgf("Failed to listen on: %v", addressPort)
				return nil, err
			}
			return lis, nil
		},
	}
}

// Returns a go channel that blocks for specified amount of time.
func (raftServer *Server) GetTimeoutWaitChannel(timeoutMs int64) *time.Timer {
	return time.NewTimer(time.Millisecond * time.Duration(timeoutMs))
}

func (raftServer *Server) GetConfigElectionTimeoutMillis() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.raftConfig.electionTimeoutMillis
}

// Get amount of time remaining in millis before last heartbeat received is considered
// to have expired and thus we no longer have a leader.
func (raftServer *Server) GetRemainingHeartbeatTimeMs() int64 {
	timeoutMs := raftServer.GetConfigElectionTimeoutMillis()
	elapsedMs := raftServer.TimeSinceLastHeartBeatMillis()
	remainingMs := timeoutMs - elapsedMs
	if remainingMs < 0 {
		remainingMs = 0
	}
	return remainingMs
}

// Returns a go channel that blocks for a randomized election timeout time.
func (raftServer *Server) RandomizedElectionTimeout() *time.Timer {
	timeoutMs := PickElectionTimeOutMillis()
	return raftServer.GetTimeoutWaitChannel(timeoutMs)
}

// Picks a randomized time for the election timeout.
func PickElectionTimeOutMillis() int64 {
	baseTimeMs := int64(300)
	// Go random number is deterministic by default so we re-seed to get randomized behavior we want.
	randomOffsetMs := int64(rand.Intn(300))
	return baseTimeMs + randomOffsetMs
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
		go func(i int, node zproto.RaftClient) {
			defer waitGroup.Done()
			raftServer.SendAppendEntriesReplicationRpcForFollower(i, node)
		}(i, node)
	}

	waitGroup.Wait()

	// After replicating, increment the commit index if its possible.
	raftServer.IncrementCommitIndexIfPossible()
}

// If needed, sends append entries rpc to given "client" follower to make their logs match ours.
func (raftServer *Server) SendAppendEntriesReplicationRpcForFollower(serverIndex int, client zproto.RaftClient) {
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
	request := zproto.AppendEntriesRequest{}
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
func (raftServer *Server) SendHeartBeatsToFollowers() {
	otherNodes := raftServer.GetOtherNodes()

	var waitGroup sync.WaitGroup
	waitGroup.Add(len(otherNodes))

	for _, node := range otherNodes {
		// Send RPCs in parallel. Pass copy of node to avoid race conditions.
		go func(node zproto.RaftClient) {
			defer waitGroup.Done()
			raftServer.SendHeartBeatRpc(node)
		}(node)
	}
	waitGroup.Wait()
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
