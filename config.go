package raft

import "sync"

type Configuration struct {
	sync.Mutex
	Servers map[string]string
}

func NewConfiguration() *Configuration {
	return &Configuration{
		Servers: make(map[string]string),
	}
}

func (c *Configuration) AddServer(id, address string) {
	c.Lock()
	defer c.Unlock()
	c.Servers[id] = address
}

func (c *Configuration) RemoveServer(id string) {
	c.Lock()
	defer c.Unlock()
	delete(c.Servers, id)
}

func (c *Configuration) GetServers() map[string]string {
	c.Lock()
	defer c.Unlock()
	return c.Servers
}
