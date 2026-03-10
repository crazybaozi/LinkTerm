package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nhooyr.io/websocket"
)

func makeWSPair(t *testing.T) (serverConn *websocket.Conn, cleanup func()) {
	t.Helper()
	var sc *websocket.Conn
	ready := make(chan struct{})
	done := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		sc = c
		close(ready)
		<-done
		c.Close(websocket.StatusNormalClosure, "done")
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	cc, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatal(err)
	}

	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		srv.Close()
		t.Fatal("timed out waiting for server connection")
	}

	cleanup = func() {
		close(done)
		cc.Close(websocket.StatusNormalClosure, "")
		srv.Close()
	}
	return sc, cleanup
}

func TestSetBrowserConn_FirstConnection(t *testing.T) {
	session := &TerminalSession{
		ID:     "test-first",
		Status: StatusDetached,
		Buffer: NewRingBuffer(1024),
	}

	conn, cleanup := makeWSPair(t)
	defer cleanup()

	old := session.SetBrowserConn(conn)
	if old != nil {
		t.Error("first SetBrowserConn should return nil")
	}
	if session.Status != StatusActive {
		t.Errorf("expected status %s, got %s", StatusActive, session.Status)
	}
	if session.BrowserWS != conn {
		t.Error("BrowserWS should be the new conn")
	}
}

func TestSetBrowserConn_ReturnsOldConnection(t *testing.T) {
	session := &TerminalSession{
		ID:     "test-replace",
		Status: StatusDetached,
		Buffer: NewRingBuffer(1024),
	}

	conn1, cleanup1 := makeWSPair(t)
	defer cleanup1()
	conn2, cleanup2 := makeWSPair(t)
	defer cleanup2()

	session.SetBrowserConn(conn1)

	old := session.SetBrowserConn(conn2)
	if old != conn1 {
		t.Error("second SetBrowserConn should return conn1")
	}
	if session.BrowserWS != conn2 {
		t.Error("BrowserWS should be conn2")
	}
	if session.Status != StatusActive {
		t.Errorf("expected status %s, got %s", StatusActive, session.Status)
	}
}

func TestOnBrowserDisconnect_OnlyAffectsOwnConn(t *testing.T) {
	session := &TerminalSession{
		ID:     "test-race",
		Status: StatusDetached,
		Buffer: NewRingBuffer(1024),
	}

	conn1, cleanup1 := makeWSPair(t)
	defer cleanup1()
	conn2, cleanup2 := makeWSPair(t)
	defer cleanup2()

	session.SetBrowserConn(conn1)
	old := session.SetBrowserConn(conn2)
	if old != nil {
		old.Close(websocket.StatusNormalClosure, "replaced")
	}

	session.OnBrowserDisconnect(conn1)

	session.mu.Lock()
	ws := session.BrowserWS
	st := session.Status
	session.mu.Unlock()

	if ws != conn2 {
		t.Fatal("RACE BUG: old goroutine's OnBrowserDisconnect cleared new connection")
	}
	if st != StatusActive {
		t.Fatal("RACE BUG: status changed to non-active by old goroutine")
	}
}

func TestOnBrowserDisconnect_ClearsOwnConn(t *testing.T) {
	session := &TerminalSession{
		ID:     "test-normal-dc",
		Status: StatusDetached,
		Buffer: NewRingBuffer(1024),
	}

	conn, cleanup := makeWSPair(t)
	defer cleanup()

	session.SetBrowserConn(conn)
	session.OnBrowserDisconnect(conn)

	session.mu.Lock()
	ws := session.BrowserWS
	st := session.Status
	session.mu.Unlock()

	if ws != nil {
		t.Error("BrowserWS should be nil after disconnect")
	}
	if st != StatusDetached {
		t.Errorf("expected status %s, got %s", StatusDetached, st)
	}
}

func TestOnBrowserDisconnect_ConcurrentGoroutines(t *testing.T) {
	session := &TerminalSession{
		ID:     "test-concurrent",
		Status: StatusDetached,
		Buffer: NewRingBuffer(1024),
	}

	conn1, cleanup1 := makeWSPair(t)
	defer cleanup1()
	conn2, cleanup2 := makeWSPair(t)
	defer cleanup2()

	session.SetBrowserConn(conn1)
	old := session.SetBrowserConn(conn2)
	if old != nil {
		old.Close(websocket.StatusNormalClosure, "replaced")
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		session.OnBrowserDisconnect(conn1)
	}()

	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		session.mu.Lock()
		ws := session.BrowserWS
		st := session.Status
		session.mu.Unlock()
		if ws != conn2 {
			t.Error("RACE BUG: BrowserWS is not conn2 during concurrent disconnect")
		}
		if st != StatusActive {
			t.Error("RACE BUG: status is not active during concurrent disconnect")
		}
	}()

	wg.Wait()

	session.mu.Lock()
	finalWS := session.BrowserWS
	finalSt := session.Status
	session.mu.Unlock()

	if finalWS != conn2 {
		t.Fatal("RACE BUG: after concurrent ops, BrowserWS should be conn2")
	}
	if finalSt != StatusActive {
		t.Fatal("RACE BUG: after concurrent ops, status should be active")
	}
}

func TestWriteToBrowser_AfterReplacement(t *testing.T) {
	session := &TerminalSession{
		ID:     "test-write",
		Status: StatusDetached,
		Buffer: NewRingBuffer(1024),
	}

	conn1, cleanup1 := makeWSPair(t)
	defer cleanup1()
	conn2, cleanup2 := makeWSPair(t)
	defer cleanup2()

	session.SetBrowserConn(conn1)
	old := session.SetBrowserConn(conn2)
	if old != nil {
		old.Close(websocket.StatusNormalClosure, "replaced")
	}

	session.OnBrowserDisconnect(conn1)

	session.mu.Lock()
	ws := session.BrowserWS
	session.mu.Unlock()

	if ws != conn2 {
		t.Fatal("after replacement and old disconnect, BrowserWS should still be conn2")
	}

	session.WriteToBrowser([]byte("hello"))

	if session.Buffer.Len() == 0 {
		t.Error("buffer should have data after WriteToBrowser")
	}
}

func TestRingBuffer_WriteRead(t *testing.T) {
	buf := NewRingBuffer(8)

	buf.Write([]byte("hello"))
	got := buf.ReadAll()
	if string(got) != "hello" {
		t.Errorf("expected 'hello', got '%s'", string(got))
	}

	buf.Write([]byte("12345678"))
	got = buf.ReadAll()
	if len(got) != 8 {
		t.Errorf("expected 8 bytes, got %d", len(got))
	}
	if string(got) != "12345678" {
		t.Errorf("expected '12345678', got '%s'", string(got))
	}
}

func TestRingBuffer_Overflow(t *testing.T) {
	buf := NewRingBuffer(4)
	buf.Write([]byte("abcdef"))
	got := buf.ReadAll()
	if string(got) != "cdef" {
		t.Errorf("expected 'cdef' after overflow, got '%s'", string(got))
	}
}

func TestSessionManager_CreateAndGet(t *testing.T) {
	mgr := NewSessionManager(1024)
	sess := mgr.Create("s1", "agent1")
	if sess.ID != "s1" {
		t.Errorf("expected ID 's1', got '%s'", sess.ID)
	}
	if sess.AgentID != "agent1" {
		t.Errorf("expected AgentID 'agent1', got '%s'", sess.AgentID)
	}

	got := mgr.Get("s1")
	if got != sess {
		t.Error("Get should return the created session")
	}

	if mgr.Get("nonexistent") != nil {
		t.Error("Get should return nil for nonexistent session")
	}
}

func TestSessionManager_CountAndList(t *testing.T) {
	mgr := NewSessionManager(1024)
	mgr.Create("s1", "agent1")
	mgr.Create("s2", "agent1")
	mgr.Create("s3", "agent2")

	if mgr.CountByAgent("agent1") != 2 {
		t.Errorf("expected 2, got %d", mgr.CountByAgent("agent1"))
	}
	if mgr.CountByAgent("agent2") != 1 {
		t.Errorf("expected 1, got %d", mgr.CountByAgent("agent2"))
	}

	list := mgr.ListByAgent("agent1")
	if len(list) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(list))
	}

	active := mgr.ListActive()
	if len(active) != 3 {
		t.Errorf("expected 3 active sessions, got %d", len(active))
	}
}

func TestSessionManager_Remove(t *testing.T) {
	mgr := NewSessionManager(1024)
	mgr.Create("s1", "agent1")
	mgr.Remove("s1")

	if mgr.Get("s1") != nil {
		t.Error("session should be nil after removal")
	}
	if mgr.CountByAgent("agent1") != 0 {
		t.Error("count should be 0 after removal")
	}
}
