package server

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func stateRequest(id string) *http.Request {
	req, _ := http.NewRequest("GET", "/", nil)
	if id != "" {
		req.Header.Set(StateHeader, id)
	}
	return req
}

// The least recently used state is evicted to make room, and unused states
// expire.
func TestStateStoreLimits(t *testing.T) {
	st := newStateStore(2, time.Hour)
	ids := []string{}
	for range 3 {
		id, s, _, err := st.acquire(stateRequest(""))
		if err != nil {
			t.Fatal(err)
		}
		st.release(s)
		ids = append(ids, id)
		time.Sleep(time.Millisecond) // distinct lastUsed
	}
	if n := st.len(); n != 2 {
		t.Errorf("expected 2 states, got %d", n)
	}
	var gone errStateGone
	if _, _, _, err := st.acquire(stateRequest(ids[0])); !errors.As(err, &gone) {
		t.Errorf("oldest state: expected gone, got %v", err)
	}

	// using a state makes it the most recent
	_, s, _, err := st.acquire(stateRequest(ids[1]))
	if err != nil {
		t.Fatal(err)
	}
	st.release(s)
	_, s, _, _ = st.acquire(stateRequest(""))
	st.release(s)
	if _, _, _, err := st.acquire(stateRequest(ids[2])); !errors.As(err, &gone) {
		t.Errorf("least recently used: expected gone, got %v", err)
	}
	if _, s, _, err := st.acquire(stateRequest(ids[1])); err != nil {
		t.Errorf("recently used: %v", err)
	} else {
		st.release(s)
	}

	// states in use aren't evicted (the store goes over instead)
	_, busy, _, _ := st.acquire(stateRequest(ids[1]))
	_, s, _, _ = st.acquire(stateRequest(""))
	st.release(s)
	_, s, _, _ = st.acquire(stateRequest(""))
	st.release(s)
	if busy.closed {
		t.Error("evicted a state in use")
	}
	st.release(busy)

	// expiry
	st = newStateStore(10, 10*time.Millisecond)
	id, s, _, _ := st.acquire(stateRequest(""))
	st.release(s)
	time.Sleep(20 * time.Millisecond)
	if _, _, _, err := st.acquire(stateRequest(id)); !errors.As(err, &gone) {
		t.Errorf("expired: expected gone, got %v", err)
	}
}

// Discarded states are closed on release.
func TestStateStoreDiscard(t *testing.T) {
	st := newStateStore(0, 0)
	id, s, _, _ := st.acquire(stateRequest(""))
	st.discard(id, s)
	st.release(s)
	if !s.closed || st.len() != 0 {
		t.Errorf("closed %v, states %d", s.closed, st.len())
	}
}

// A page's state is only kept if it has actions.
func TestPageStateKeptOnlyIfInteractive(t *testing.T) {
	s := &Server{Router: http.NewServeMux(), Config: Config{Quiet: true}}
	s.states = newStateStore(0, 0)
	for _, c := range []struct {
		page string
		kept int
	}{
		{`return <p>static</p>`, 0},
		{`return { status = 302, headers = { Location = '/' } }`, 0},
		{`error('boom')`, 0},
		{`return <button onclick={() -> {}}>live</button>`, 1},
	} {
		before := s.states.len()
		s.RunLuaPipeline(&Handler{Method: "GET", Path: "/", Page: c.page, Lang: "erde"}, stateRequest(""), nil)
		if kept := s.states.len() - before; kept != c.kept {
			t.Errorf("%s: kept %d states, want %d", c.page, kept, c.kept)
		}
	}
}
