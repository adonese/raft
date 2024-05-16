package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/adonese/raft"
)

// KVStoreFSM is a simple key-value store FSM.
type KVStoreFSM struct {
	mu    sync.Mutex
	store map[string]string
}

type SetCommand struct {
	Key   string
	Value string
}

func NewKVStoreFSM() *KVStoreFSM {
	return &KVStoreFSM{
		store: make(map[string]string),
	}
}

// Apply applies a log entry to the state machine.
func (fsm *KVStoreFSM) Apply(log *raft.Log) interface{} {
	var cmd SetCommand
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return fmt.Errorf("could not unmarshal log data: %v", err)
	}

	fsm.mu.Lock()
	defer fsm.mu.Unlock()
	fsm.store[cmd.Key] = cmd.Value
	return nil
}

// Snapshot creates a snapshot of the current state.
func (fsm *KVStoreFSM) Snapshot() (raft.FSMSnapshot, error) {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	snapshot := make(map[string]string)
	for k, v := range fsm.store {
		snapshot[k] = v
	}
	return &KVStoreSnapshot{store: snapshot}, nil
}

// Restore restores the state from a snapshot.
func (fsm *KVStoreFSM) Restore(snapshot io.ReadCloser) error {
	var store map[string]string
	if err := json.NewDecoder(snapshot).Decode(&store); err != nil {
		return fmt.Errorf("could not decode snapshot: %v", err)
	}

	fsm.mu.Lock()
	defer fsm.mu.Unlock()
	fsm.store = store
	return nil
}

type KVStoreSnapshot struct {
	store map[string]string
}

// Persist writes the snapshot to the sink.
func (s *KVStoreSnapshot) Persist(sink raft.SnapshotSink) error {
	data, err := json.Marshal(s.store)
	if err != nil {
		sink.Cancel()
		return fmt.Errorf("could not marshal snapshot data: %v", err)
	}

	if _, err := sink.Write(data); err != nil {
		sink.Cancel()
		return fmt.Errorf("could not write snapshot data: %v", err)
	}

	if err := sink.Close(); err != nil {
		return fmt.Errorf("could not close snapshot sink: %v", err)
	}
	return nil
}

// Release releases the snapshot.
func (s *KVStoreSnapshot) Release() {}
