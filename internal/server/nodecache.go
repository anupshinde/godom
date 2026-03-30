package server

import (
	"sync"
	"time"

	"github.com/anupshinde/godom/internal/component"
	"github.com/anupshinde/godom/internal/vdom"
)

// cachedEntry holds a cached node lookup result.
type cachedEntry struct {
	Node         vdom.Node
	Comp         *component.Info
	LastAccessed time.Time
}

// nodeCache provides O(1) lookups for nodeID → node and nodeID → component.
// Falls back to tree traversal on cache miss, then caches the result.
// Stale entries (removed nodes/components) are evicted on access.
type nodeCache struct {
	mu      sync.RWMutex
	entries map[int]*cachedEntry
}

func newNodeCache() *nodeCache {
	return &nodeCache{entries: make(map[int]*cachedEntry)}
}

// get returns the cached entry for a nodeID, or nil if not cached or stale.
func (nc *nodeCache) get(nodeID int) *cachedEntry {
	nc.mu.RLock()
	e, ok := nc.entries[nodeID]
	nc.mu.RUnlock()
	if !ok {
		return nil
	}
	// Evict if node or component has been removed.
	if e.Node.IsRemoved() || e.Comp.Removed {
		nc.mu.Lock()
		delete(nc.entries, nodeID)
		nc.mu.Unlock()
		return nil
	}
	// Update access time (write lock needed).
	nc.mu.Lock()
	e.LastAccessed = time.Now()
	nc.mu.Unlock()
	return e
}

// put stores a node and its owning component in the cache.
func (nc *nodeCache) put(nodeID int, node vdom.Node, comp *component.Info) {
	nc.mu.Lock()
	nc.entries[nodeID] = &cachedEntry{
		Node:         node,
		Comp:         comp,
		LastAccessed: time.Now(),
	}
	nc.mu.Unlock()
}

// evictRemoved walks the cache and removes entries for nodes or components
// that have been marked as removed. Called after BuildUpdate when the tree
// structure may have changed.
func (nc *nodeCache) evictRemoved() {
	nc.mu.Lock()
	for id, e := range nc.entries {
		if e.Node.IsRemoved() || e.Comp.Removed {
			delete(nc.entries, id)
		}
	}
	nc.mu.Unlock()
}

// findNode looks up a node by ID. Checks the cache first, falls back to
// tree traversal on the given component, then caches the result.
func findNode(nodeID int, ci *component.Info, cache *nodeCache) vdom.Node {
	if e := cache.get(nodeID); e != nil {
		return e.Node
	}
	node := vdom.FindNodeByID(ci.Tree, nodeID)
	if node != nil {
		cache.put(nodeID, node, ci)
	}
	return node
}

// findComponent looks up which component owns a nodeID. Checks the cache
// first, falls back to searching all components' trees, then caches the result.
func findComponent(nodeID int, comps []*component.Info, cache *nodeCache) *component.Info {
	if e := cache.get(nodeID); e != nil {
		return e.Comp
	}
	for _, ci := range comps {
		ci.Mu.Lock()
		node := vdom.FindNodeByID(ci.Tree, nodeID)
		ci.Mu.Unlock()
		if node != nil {
			cache.put(nodeID, node, ci)
			return ci
		}
	}
	return nil
}
