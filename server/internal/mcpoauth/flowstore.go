package mcpoauth

import (
	"sync"
	"time"
)

// flowTTL bounds how long a started authorization may stay pending before the
// user must restart it. Generous enough for a real login, short enough that a
// leaked state value is useless soon after.
const flowTTL = 10 * time.Minute

// Flow is one in-flight authorization, created by the start endpoint and
// consumed by the OAuth callback. It carries everything the callback needs to
// finish the exchange without re-discovering or trusting callback input.
type Flow struct {
	WorkspaceID string
	Resource    string
	RedirectURI string
	CreatedBy   string
	PKCE        PKCE
	Client      Client
	Discovery   Discovery
	createdAt   time.Time
}

// FlowStore holds pending authorizations keyed by opaque state. Entries are
// single-use (Take removes them) and expire after flowTTL. Safe for concurrent
// use. In-memory by design: a pending login is cheap to restart, so it needn't
// survive a control-plane restart.
type FlowStore struct {
	mu      sync.Mutex
	flows   map[string]Flow
	nowFunc func() time.Time // overridable in tests
}

// NewFlowStore returns an empty store using the wall clock.
func NewFlowStore() *FlowStore {
	return &FlowStore{flows: make(map[string]Flow), nowFunc: time.Now}
}

func (s *FlowStore) now() time.Time {
	if s.nowFunc != nil {
		return s.nowFunc()
	}
	return time.Now()
}

// Put stores a flow under the given state, stamping its creation time and
// opportunistically evicting expired entries.
func (s *FlowStore) Put(state string, f Flow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f.createdAt = s.now()
	s.flows[state] = f
	s.gcLocked()
}

// Take removes and returns the flow for state. ok is false when state is
// unknown or expired — either way the caller must reject the callback.
func (s *FlowStore) Take(state string) (Flow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.flows[state]
	if !ok {
		return Flow{}, false
	}
	delete(s.flows, state)
	if s.now().Sub(f.createdAt) > flowTTL {
		return Flow{}, false
	}
	return f, true
}

// gcLocked drops expired entries. Caller holds the lock.
func (s *FlowStore) gcLocked() {
	cutoff := s.now().Add(-flowTTL)
	for state, f := range s.flows {
		if f.createdAt.Before(cutoff) {
			delete(s.flows, state)
		}
	}
}
