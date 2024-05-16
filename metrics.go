package raft

import "expvar"

var (
	termVar        = expvar.NewInt("currentTerm")
	commitIndexVar = expvar.NewInt("commitIndex")
	lastAppliedVar = expvar.NewInt("lastApplied")
	stateVar       = expvar.NewString("state")
	leaderVar      = expvar.NewString("leader")
	votedForVar    = expvar.NewString("votedFor")
)

func (r *Raft) updateMetrics() {
	termVar.Set(int64(r.currentTerm))
	commitIndexVar.Set(int64(r.commitIndex))
	lastAppliedVar.Set(int64(r.lastApplied))
	stateVar.Set(r.state.String())
	leaderVar.Set(r.leaderID)
	votedForVar.Set(r.votedFor)
}
