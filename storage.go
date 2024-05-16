package raft

import (
	"encoding/gob"
	"io"
	"os"
)

type PersistentStore struct {
	logFile *os.File
}

func NewPersistentStore(path string) (*PersistentStore, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return nil, err
	}
	return &PersistentStore{logFile: file}, nil
}

func (ps *PersistentStore) AppendLogs(logs []*Log) error {
	encoder := gob.NewEncoder(ps.logFile)
	for _, log := range logs {
		if err := encoder.Encode(log); err != nil {
			return err
		}
	}
	return nil
}

func (ps *PersistentStore) LoadLogs() ([]*Log, error) {
	var logs []*Log
	decoder := gob.NewDecoder(ps.logFile)
	for {
		var log Log
		if err := decoder.Decode(&log); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		logs = append(logs, &log)
	}
	return logs, nil
}
