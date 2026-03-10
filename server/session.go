package main

import (
	"context"
	"log"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

/** SessionStatus 终端会话状态 */
type SessionStatus string

const (
	StatusActive   SessionStatus = "active"
	StatusDetached SessionStatus = "detached"
	StatusOrphan   SessionStatus = "orphan"
	StatusClosed   SessionStatus = "closed"
)

/** TerminalSession 表示一个终端会话 */
type TerminalSession struct {
	ID         string
	AgentID    string
	Status     SessionStatus
	BrowserWS  *websocket.Conn
	Buffer     *RingBuffer
	CreatedAt  time.Time
	DetachedAt time.Time
	mu         sync.Mutex
}

/** SetBrowserConn 设置浏览器端 WebSocket 连接，返回被替换的旧连接 */
func (s *TerminalSession) SetBrowserConn(conn *websocket.Conn) *websocket.Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.BrowserWS
	s.BrowserWS = conn
	if conn != nil {
		s.Status = StatusActive
		s.DetachedAt = time.Time{}
	}
	return old
}

/** OnBrowserDisconnect 浏览器断开时调用，仅当 conn 仍是当前连接时才清除 */
func (s *TerminalSession) OnBrowserDisconnect(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.BrowserWS != conn {
		return
	}
	s.BrowserWS = nil
	if s.Status == StatusActive {
		s.Status = StatusDetached
		s.DetachedAt = time.Now()
		log.Printf("[session] %s detached (browser disconnected)", s.ID)
	}
}

/** OnAgentDisconnect Agent 断开时调用 */
func (s *TerminalSession) OnAgentDisconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Status != StatusClosed {
		s.Status = StatusOrphan
		log.Printf("[session] %s orphaned (agent disconnected)", s.ID)
		if s.BrowserWS != nil {
			s.BrowserWS.Write(context.Background(), websocket.MessageText, []byte(`{"type":"session_status","status":"orphan"}`))
		}
	}
}

/** OnAgentReconnect Agent 重连后恢复会话 */
func (s *TerminalSession) OnAgentReconnect(agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AgentID = agentID
	if s.BrowserWS != nil {
		s.Status = StatusActive
		s.BrowserWS.Write(context.Background(), websocket.MessageText, []byte(`{"type":"session_status","status":"active"}`))
	} else {
		s.Status = StatusDetached
	}
	log.Printf("[session] %s reactivated by agent %s", s.ID, agentID)
}

/** WriteToBrowser 将 PTY 输出写入浏览器和缓冲 */
func (s *TerminalSession) WriteToBrowser(data []byte) {
	s.Buffer.Write(data)

	s.mu.Lock()
	conn := s.BrowserWS
	s.mu.Unlock()

	if conn != nil {
		conn.Write(context.Background(), websocket.MessageBinary, data)
	}
}

/** SessionManager 管理所有终端会话 */
type SessionManager struct {
	sessions   map[string]*TerminalSession
	mu         sync.RWMutex
	bufferSize int
}

func NewSessionManager(bufferSize int) *SessionManager {
	return &SessionManager{
		sessions:   make(map[string]*TerminalSession),
		bufferSize: bufferSize,
	}
}

/** Create 创建新会话 */
func (m *SessionManager) Create(sessionID, agentID string) *TerminalSession {
	s := &TerminalSession{
		ID:        sessionID,
		AgentID:   agentID,
		Status:    StatusDetached,
		Buffer:    NewRingBuffer(m.bufferSize),
		CreatedAt: time.Now(),
	}
	m.mu.Lock()
	m.sessions[sessionID] = s
	m.mu.Unlock()
	log.Printf("[session] %s created for agent %s", sessionID, agentID)
	return s
}

/** Get 获取会话 */
func (m *SessionManager) Get(sessionID string) *TerminalSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[sessionID]
}

/** Remove 移除会话 */
func (m *SessionManager) Remove(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[sessionID]; ok {
		s.Status = StatusClosed
		delete(m.sessions, sessionID)
		log.Printf("[session] %s removed", sessionID)
	}
}

/** ListByAgent 列出 Agent 的所有会话 */
func (m *SessionManager) ListByAgent(agentID string) []*TerminalSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*TerminalSession
	for _, s := range m.sessions {
		if s.AgentID == agentID {
			list = append(list, s)
		}
	}
	return list
}

/** OrphanByAgent 将 Agent 的所有会话标记为 orphan */
func (m *SessionManager) OrphanByAgent(agentID string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		if s.AgentID == agentID {
			s.OnAgentDisconnect()
		}
	}
}

/** ListActive 列出所有非关闭的会话 */
func (m *SessionManager) ListActive() []*TerminalSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*TerminalSession
	for _, s := range m.sessions {
		if s.Status != StatusClosed {
			list = append(list, s)
		}
	}
	return list
}

/** CountByAgent 统计 Agent 的活跃会话数 */
func (m *SessionManager) CountByAgent(agentID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, s := range m.sessions {
		if s.AgentID == agentID && s.Status != StatusClosed {
			count++
		}
	}
	return count
}

/** RingBuffer 环形缓冲区 */
type RingBuffer struct {
	data []byte
	size int
	pos  int
	full bool
	mu   sync.Mutex
}

func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		data: make([]byte, size),
		size: size,
	}
}

func (r *RingBuffer) Write(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, b := range p {
		r.data[r.pos] = b
		r.pos = (r.pos + 1) % r.size
		if r.pos == 0 {
			r.full = true
		}
	}
}

/** ReadAll 读取缓冲区中所有数据 */
func (r *RingBuffer) ReadAll() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		result := make([]byte, r.pos)
		copy(result, r.data[:r.pos])
		return result
	}
	result := make([]byte, r.size)
	copy(result, r.data[r.pos:])
	copy(result[r.size-r.pos:], r.data[:r.pos])
	return result
}

/** Len 返回缓冲区中已写入的数据量 */
func (r *RingBuffer) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.full {
		return r.size
	}
	return r.pos
}
