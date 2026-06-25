package api

import "sync"

const maxMemResults = 32

type memResultCache struct {
	mu    sync.RWMutex
	items map[string]*BacktestResult
	order []string
}

func newMemResultCache() *memResultCache {
	return &memResultCache{items: make(map[string]*BacktestResult)}
}

func (c *memResultCache) get(id string) (*BacktestResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	r, ok := c.items[id]
	return r, ok
}

func (c *memResultCache) put(id string, res *BacktestResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.items[id]; !exists {
		c.order = append(c.order, id)
	}
	c.items[id] = res
	for len(c.order) > maxMemResults {
		evict := c.order[0]
		c.order = c.order[1:]
		delete(c.items, evict)
	}
}
