package raft

type Log struct {
	Type LogType
	Data []byte
	Term int
}

type LogType uint8

const (
	LogCommand LogType = iota
	LogNoop
)

type RequestVoteRequest struct {
	Term        int
	CandidateID string
}

type RequestVoteResponse struct {
	Term        int
	VoteGranted bool
}

type AppendEntriesRequest struct {
	Term         int
	LeaderID     string
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []*Log
	LeaderCommit int
}

type AppendEntriesResponse struct {
	Term    int
	Success bool
}
