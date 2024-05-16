package raft

import "io"

type FSM interface {
	Apply(log *Log) interface{}
	Snapshot() (FSMSnapshot, error)
	Restore(io.ReadCloser) error
}

type FSMSnapshot interface {
	Persist(sink SnapshotSink) error
	Release()
}

type SnapshotSink interface {
	io.Writer
	Close() error
	Cancel() error
}
