package server

import (
	"sync"

	"github.com/anupshinde/godom/internal/component"
	"github.com/anupshinde/godom/internal/vdom"
)

// lookupEntry holds a node lookup result.
type lookupEntry struct {
	Node vdom.Node
	Comp *component.Info
}

// nodeLookup provides O(1) lookups for nodeID → node and nodeID → component.
// Lazily populated on first access via tree traversal, then serves subsequent
// lookups directly. Stale entries (removed nodes/components) are evicted on
// access and swept after BuildUpdate.
type nodeLookup struct {
	mu      sync.RWMutex
	entries map[int]*lookupEntry
}

func newNodeLookup() *nodeLookup {
	return &nodeLookup{entries: make(map[int]*lookupEntry)}
}

// get returns the cached entry for a nodeID, or nil if not cached or stale.
func (nc *nodeLookup) get(nodeID int) *lookupEntry {
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
	return e
}

// put stores a node and its owning component in the cache.
func (nc *nodeLookup) put(nodeID int, node vdom.Node, comp *component.Info) {
	nc.mu.Lock()
	nc.entries[nodeID] = &lookupEntry{
		Node: node,
		Comp: comp,
	}
	nc.mu.Unlock()
}

// evictRemoved walks the lookup and removes entries for nodes or components
// that have been marked as removed. Called after BuildUpdate when the tree
// structure may have changed.
func (nc *nodeLookup) evictRemoved() {
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
func findNode(nodeID int, ci *component.Info, cache *nodeLookup) vdom.Node {
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
func findComponent(nodeID int, comps []*component.Info, cache *nodeLookup) *component.Info {
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
