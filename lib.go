package raft

import (
	"encoding/gob"
	"fmt"
	"io"
	"math/rand"
	"net"
	"sort"
	"sync"
	"time"
)

// Raft represents the Raft consensus algorithm.
type Raft struct {
	mu             sync.Mutex
	state          State
	peers          map[string]*Peer
	logs           []*Log
	commitIndex    int
	lastApplied    int
	nextIndex      map[string]int
	matchIndex     map[string]int
	currentTerm    int
	votedFor       string
	leaderID       string
	fsm            FSM
	transport      *SecureTransport
	electionTimer  *time.Timer
	heartbeatTimer *time.Timer
	stopCh         chan struct{}
	config         *Configuration
	store          *PersistentStore
	db             *SafeMap
}

// State represents the state of a Raft node.
type State int

const (
	Follower State = iota
	Candidate
	Leader
)

func (s State) String() string {
	switch s {
	case Follower:
		return "Follower"
	case Candidate:
		return "Candidate"
	case Leader:
		return "Leader"
	default:
		return "Unknown"
	}
}

// Peer represents a peer in the Raft cluster.
type Peer struct {
	ID      string
	Address string
}

// NewRaft creates a new Raft instance.
func NewRaft(fsm FSM, transport *SecureTransport, store *PersistentStore, config *Configuration) *Raft {
	db := NewSafeMap()
	r := &Raft{
		state:          Follower,
		peers:          make(map[string]*Peer),
		nextIndex:      make(map[string]int),
		matchIndex:     make(map[string]int),
		fsm:            fsm,
		transport:      transport,
		electionTimer:  time.NewTimer(randomElectionTimeout()),
		heartbeatTimer: time.NewTimer(time.Second),
		stopCh:         make(chan struct{}),
		config:         config,
		store:          store,
		db:             db,
	}
	go r.run()
	return r
}

// run starts the main loop for the Raft node.
func (r *Raft) run() {
	for {
		select {
		case <-r.electionTimer.C:
			r.startElection()
		case <-r.heartbeatTimer.C:
			r.sendHeartbeats()
		case <-r.stopCh:
			return
		}
	}
}

// startElection starts a new election.
func (r *Raft) startElection() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.state = Candidate
	r.currentTerm++
	r.votedFor = r.transport.LocalAddr().String()
	r.resetElectionTimer()

	votes := 1
	voteCh := make(chan bool, len(r.peers))

	r.logInfo("entering candidate state: node=%q term=%d", r.transport.LocalAddr(), r.currentTerm)

	for _, peer := range r.peers {
		go func(peer *Peer) {
			r.logDebug("asking for vote: term=%d from=%s address=%s", r.currentTerm, peer.ID, peer.Address)
			voteGranted := r.requestVote(peer)
			voteCh <- voteGranted
		}(peer)
	}

	for i := 0; i < len(r.peers); i++ {
		if <-voteCh {
			votes++
			if votes > len(r.peers)/2 {
				r.becomeLeader()
				return
			}
		}
	}
}

// requestVote sends a RequestVote RPC to a peer.
func (r *Raft) requestVote(peer *Peer) bool {
	conn, err := r.transport.Dial(peer.Address, time.Second*2)
	if err != nil {
		r.logError("failed to make requestVote RPC: target=%q error=%v", peer, err)
		return false
	}
	defer conn.Close()

	req := &RequestVoteRequest{
		Term:        r.currentTerm,
		CandidateID: r.transport.LocalAddr().String(),
	}
	resp := &RequestVoteResponse{}

	if err := sendRPC(conn, req, resp); err != nil {
		r.logError("failed to send requestVote RPC: target=%q error=%v", peer, err)
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if resp.Term > r.currentTerm {
		r.currentTerm = resp.Term
		r.state = Follower
		r.votedFor = ""
		r.resetElectionTimer()
		r.logWarn("lost leadership because received a requestVote with a newer term")
		return false
	}

	return resp.VoteGranted
}

// becomeLeader transitions the node to the leader state.
func (r *Raft) becomeLeader() {
	r.state = Leader
	r.leaderID = r.transport.LocalAddr().String()

	for peerID := range r.peers {
		r.nextIndex[peerID] = len(r.logs)
		r.matchIndex[peerID] = 0
	}

	r.logInfo("entering leader state: node=%q term=%d", r.transport.LocalAddr(), r.currentTerm)
	r.sendHeartbeats()
}

// sendHeartbeats sends heartbeats to all peers.
func (r *Raft) sendHeartbeats() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state != Leader {
		return
	}

	r.logDebug("sending heartbeats: term=%d", r.currentTerm)
	for _, peer := range r.peers {
		go r.appendEntries(peer)
	}

	r.resetHeartbeatTimer()
}

// appendEntries sends an AppendEntries RPC to a peer.
func (r *Raft) appendEntries(peer *Peer) {
	conn, err := r.transport.Dial(peer.Address, time.Second*2)
	if err != nil {
		r.logError("failed to appendEntries to: peer=%q error=%v", peer, err)
		return
	}
	defer conn.Close()

	req := &AppendEntriesRequest{
		Term:         r.currentTerm,
		LeaderID:     r.transport.LocalAddr().String(),
		PrevLogIndex: r.nextIndex[peer.ID] - 1,
		PrevLogTerm:  r.logs[r.nextIndex[peer.ID]-1].Term,
		Entries:      r.logs[r.nextIndex[peer.ID]:],
		LeaderCommit: r.commitIndex,
	}
	resp := &AppendEntriesResponse{}

	if err := sendRPC(conn, req, resp); err != nil {
		r.logError("failed to send appendEntries RPC: target=%q error=%v", peer, err)
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if resp.Term > r.currentTerm {
		r.currentTerm = resp.Term
		r.state = Follower
		r.votedFor = ""
		r.resetElectionTimer()
		r.logWarn("lost leadership because received an appendEntries with a newer term")
		return
	}

	if resp.Success {
		r.matchIndex[peer.ID] = req.PrevLogIndex + len(req.Entries)
		r.nextIndex[peer.ID] = r.matchIndex[peer.ID] + 1
		r.updateCommitIndex()
	} else {
		r.nextIndex[peer.ID]--
		r.appendEntries(peer)
	}
}

// updateCommitIndex updates the commit index.
func (r *Raft) updateCommitIndex() {
	matchIndexes := make([]int, 0, len(r.matchIndex))
	for _, idx := range r.matchIndex {
		matchIndexes = append(matchIndexes, idx)
	}
	sort.Ints(matchIndexes)

	n := len(matchIndexes)
	newCommitIndex := matchIndexes[n/2]

	if newCommitIndex > r.commitIndex && r.logs[newCommitIndex].Term == r.currentTerm {
		r.commitIndex = newCommitIndex
		r.applyLogs()
	}
}

// resetElectionTimer resets the election timer with a random timeout.
func (r *Raft) resetElectionTimer() {
	r.electionTimer.Stop()
	r.electionTimer.Reset(randomElectionTimeout())
}

// resetHeartbeatTimer resets the heartbeat timer.
func (r *Raft) resetHeartbeatTimer() {
	r.heartbeatTimer.Stop()
	r.heartbeatTimer.Reset(time.Second)
}

// randomElectionTimeout generates a random election timeout.
func randomElectionTimeout() time.Duration {
	return time.Duration(150+rand.Intn(150)) * time.Millisecond
}

// sendWithRetry sends RPCs with retries.
func (r *Raft) sendWithRetry(addr string, req, resp interface{}, retries int, timeout time.Duration) error {
	var err error
	for i := 0; i < retries; i++ {
		conn, err := r.transport.Dial(addr, timeout)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		defer conn.Close()
		if err = sendRPC(conn, req, resp); err != nil {
			time.Sleep(time.Second)
			continue
		}
		return nil
	}
	return err
}

// Apply adds a new log entry to the Raft log.
func (r *Raft) Apply(data []byte, timeout time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state != Leader {
		return fmt.Errorf("not the leader")
	}

	log := &Log{Type: LogCommand, Data: data, Term: r.currentTerm}
	r.logs = append(r.logs, log)
	r.store.AppendLogs(r.logs) // Persist logs
	r.fsm.Apply(log)

	r.commitIndex++
	r.applyLogs()

	r.logDebug("applied log entry: index=%d term=%d", r.commitIndex, r.currentTerm)

	return nil
}

// applyLogs applies committed logs to the state machine.
func (r *Raft) applyLogs() {
	for r.commitIndex > r.lastApplied {
		r.lastApplied++
		log := r.logs[r.lastApplied]
		r.fsm.Apply(log)
	}
}

// Snapshot creates a snapshot of the current state.
func (r *Raft) Snapshot() (FSMSnapshot, error) {
	return r.fsm.Snapshot()
}

// Restore restores the state from a snapshot.
func (r *Raft) Restore(snapshot io.ReadCloser) error {
	return r.fsm.Restore(snapshot)
}

// Leader returns the address of the current leader.
func (r *Raft) Leader() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state == Leader {
		return r.transport.LocalAddr().String()
	}
	return r.leaderID
}

// AddVoter adds a new voter to the cluster.
func (r *Raft) AddVoter(id string, address string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.peers[id] = &Peer{ID: id, Address: address}
	r.config.AddServer(id, address)
	return nil
}

// Stop stops the Raft instance.
func (r *Raft) Stop() {
	close(r.stopCh)
	r.electionTimer.Stop()
	r.heartbeatTimer.Stop()
}

// Utility functions for logging
func (r *Raft) logInfo(format string, args ...interface{}) {
	fmt.Printf("[INFO]  raft: "+format+"\n", args...)
}

func (r *Raft) logWarn(format string, args ...interface{}) {
	fmt.Printf("[WARN]  raft: "+format+"\n", args...)
}

func (r *Raft) logError(format string, args ...interface{}) {
	fmt.Printf("[ERROR] raft: "+format+"\n", args...)
}

func (r *Raft) logDebug(format string, args ...interface{}) {
	fmt.Printf("[DEBUG] raft: "+format+"\n", args...)
}

// sendRPC sends an RPC request and receives an RPC response.
func sendRPC(conn net.Conn, req interface{}, resp interface{}) error {
	encoder := gob.NewEncoder(conn)
	decoder := gob.NewDecoder(conn)

	if err := encoder.Encode(req); err != nil {
		return err
	}
	return decoder.Decode(resp)
}
