package server

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	lua "github.com/taybart/go-erde"
	restlua "github.com/taybart/rest/lua"
)

// Stateful handlers (`stateful = true`, and pages) keep their lua state
// between requests, so globals, required modules, and closures survive. A
// request w/o a state id gets a new state, whose id is `rest.state` in lua.
// Requests that send it back in the StateHeader reuse that state (one request
// at a time).
//
// States cost memory (roughly 0.5-2MB each, depending on what's loaded), so
// they're limited: unused states are closed after the server's `state_ttl`,
// and when there are `max_states`, the least recently used one is closed to
// make room. A request for a closed state gets a 410 (gone).

const (
	StateHeader      = "X-Rest-State"
	DefaultMaxStates = 1000
	DefaultStateTTL  = 30 * time.Minute
)

type luaState struct {
	mu       sync.Mutex // held while a request uses the state
	l        *lua.LState
	lastUsed time.Time // guarded by stateStore.mu
	// set (by the request holding mu) to close the state on release
	discarded bool
	// set (holding mu) once closed, for requests that were waiting for it
	closed bool
}

type stateStore struct {
	mu     sync.Mutex
	states map[string]*luaState
	max    int
	ttl    time.Duration
}

func newStateStore(max int, ttl time.Duration) *stateStore {
	if max <= 0 {
		max = DefaultMaxStates
	}
	if ttl <= 0 {
		ttl = DefaultStateTTL
	}
	return &stateStore{states: map[string]*luaState{}, max: max, ttl: ttl}
}

func newStateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// errStateGone is returned for a state id that doesn't exist (anymore), ex.
// after a restart, or once it expired or was evicted.
type errStateGone struct{ id string }

func (e errStateGone) Error() string { return "state " + e.id + " is gone" }

// closeIdle closes and removes a state that isn't in use (the store is
// locked).
func (st *stateStore) closeIdle(id string, s *luaState) bool {
	if !s.mu.TryLock() {
		return false
	}
	s.l.Close()
	s.closed = true
	delete(st.states, id)
	s.mu.Unlock()
	return true
}

// acquire returns the request's lua state, locked until release. The bool is
// whether the state is new (and needs setting up).
func (st *stateStore) acquire(req *http.Request) (string, *luaState, bool, error) {
	st.mu.Lock()
	now := time.Now()
	for id, s := range st.states {
		if now.Sub(s.lastUsed) > st.ttl {
			st.closeIdle(id, s)
		}
	}

	if id := req.Header.Get(StateHeader); id != "" {
		s, ok := st.states[id]
		if !ok {
			st.mu.Unlock()
			return "", nil, false, errStateGone{id}
		}
		s.lastUsed = now
		st.mu.Unlock()
		// outside of the store lock, so requests to other states don't wait
		s.mu.Lock()
		if s.closed { // closed while waiting
			s.mu.Unlock()
			return "", nil, false, errStateGone{id}
		}
		return id, s, false, nil
	}

	// make room, least recently used first (states in use are skipped, so
	// under a lot of concurrent requests this can briefly go over)
	for len(st.states) >= st.max {
		oldestID, oldest := "", (*luaState)(nil)
		for id, s := range st.states {
			if oldest == nil || s.lastUsed.Before(oldest.lastUsed) {
				oldestID, oldest = id, s
			}
		}
		if !st.closeIdle(oldestID, oldest) {
			break
		}
	}

	id := newStateID()
	s := &luaState{l: restlua.NewState(), lastUsed: now}
	s.mu.Lock()
	st.states[id] = s
	st.mu.Unlock()
	return id, s, true, nil
}

// discard removes a state that's in use (by the caller); it's closed on
// release.
func (st *stateStore) discard(id string, s *luaState) {
	st.mu.Lock()
	delete(st.states, id)
	st.mu.Unlock()
	s.discarded = true
}

func (st *stateStore) release(s *luaState) {
	if s.discarded {
		s.l.Close()
		s.closed = true
	} else {
		st.mu.Lock()
		s.lastUsed = time.Now()
		st.mu.Unlock()
	}
	s.mu.Unlock()
}

func (st *stateStore) len() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.states)
}
