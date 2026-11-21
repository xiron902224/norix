// Package desiredwatch wakes agents waiting on WatchDesired when desired state changes (#44).
package desiredwatch

import "sync"

// Hub is an in-process pub/sub keyed by node ID (empty key = broadcast to all).
type Hub struct {
	mu      sync.Mutex
	waiters map[string]map[chan struct{}]struct{}
}

// Default is shared across AgentService and mutators (workloads, compose, …).
var Default = New()

// New returns an empty Hub.
func New() *Hub {
	return &Hub{waiters: make(map[string]map[chan struct{}]struct{})}
}

// Subscribe registers a waiter for nodeID. Call the returned cancel to unregister.
// The channel is closed (or receives once) when Notify/NotifyAll fires; callers should
// treat any receive as "maybe changed" and re-check the fingerprint.
func (h *Hub) Subscribe(nodeID string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	if h.waiters[nodeID] == nil {
		h.waiters[nodeID] = make(map[chan struct{}]struct{})
	}
	h.waiters[nodeID][ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			if set := h.waiters[nodeID]; set != nil {
				delete(set, ch)
				if len(set) == 0 {
					delete(h.waiters, nodeID)
				}
			}
			h.mu.Unlock()
		})
	}
	return ch, cancel
}

// Notify wakes waiters for a specific node and broadcast ("") waiters.
func (h *Hub) Notify(nodeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.signalLocked(nodeID)
	if nodeID != "" {
		h.signalLocked("")
	}
}

// NotifyAll wakes every registered waiter (use when the affected node is unknown).
func (h *Hub) NotifyAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for nodeID := range h.waiters {
		h.signalLocked(nodeID)
	}
}

func (h *Hub) signalLocked(nodeID string) {
	for ch := range h.waiters[nodeID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
