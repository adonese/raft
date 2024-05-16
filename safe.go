package raft

import "sync"

type SafeMap struct {
	sync.RWMutex
	internal map[string]interface{}
}

func NewSafeMap() *SafeMap {
	return &SafeMap{
		internal: make(map[string]interface{}),
	}
}

func (sm *SafeMap) Load(key string) (interface{}, bool) {
	sm.RLock()
	defer sm.RUnlock()
	value, ok := sm.internal[key]
	return value, ok
}

func (sm *SafeMap) Store(key string, value interface{}) {
	sm.Lock()
	defer sm.Unlock()
	sm.internal[key] = value
}

func (sm *SafeMap) Delete(key string) {
	sm.Lock()
	defer sm.Unlock()
	delete(sm.internal, key)
}
