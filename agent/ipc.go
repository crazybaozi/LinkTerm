package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/linkterm/linkterm/proto"
)

/** IPC 消息类型常量 */
const (
	IPCShareOpen   = "share_open"
	IPCShareOpened = "share_opened"
	IPCShareOutput = "share_output"
	IPCShareInput  = "share_input"
	IPCShareResize = "share_resize"
	IPCShareClose  = "share_close"
	IPCShareClosed = "share_closed"
	IPCShareError  = "share_error"
)

/** IPCMessage 是 share 客户端与 agent 之间的通信协议（换行分隔的 JSON） */
type IPCMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	Data      string `json:"data,omitempty"`
	Cols      uint16 `json:"cols,omitempty"`
	Rows      uint16 `json:"rows,omitempty"`
	Error     string `json:"error,omitempty"`
}

/** IPCClient 表示一个通过 Unix socket 连接的 share 客户端 */
type IPCClient struct {
	sessionID string
	createdAt int64
	shell     string
	conn      net.Conn
	encoder   *json.Encoder
	mu        sync.Mutex
}

/** Send 向 share 客户端发送消息（线程安全） */
func (c *IPCClient) Send(msg IPCMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.encoder.Encode(msg)
}

/** IPCServer 监听 Unix socket，接受 linkterm share 客户端连接 */
type IPCServer struct {
	sockPath string
	tunnel   *Tunnel
	clients  map[string]*IPCClient
	mu       sync.RWMutex
	listener net.Listener
}

func NewIPCServer(sockPath string) *IPCServer {
	return &IPCServer{
		sockPath: sockPath,
		clients:  make(map[string]*IPCClient),
	}
}

/** SetTunnel 设置关联的 Tunnel（延迟注入，因为 tunnel 创建可能晚于 IPC server） */
func (s *IPCServer) SetTunnel(t *Tunnel) {
	s.tunnel = t
}

/** Start 启动 Unix socket 监听 */
func (s *IPCServer) Start() error {
	os.Remove(s.sockPath)
	os.MkdirAll(filepath.Dir(s.sockPath), 0755)

	listener, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.sockPath, err)
	}
	s.listener = listener

	log.Printf("[ipc] listening on %s", s.sockPath)
	go s.acceptLoop()
	return nil
}

/** Stop 停止监听并关闭所有客户端连接 */
func (s *IPCServer) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(s.sockPath)

	s.mu.Lock()
	for _, c := range s.clients {
		c.conn.Close()
	}
	s.mu.Unlock()
}

func (s *IPCServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleClient(conn)
	}
}

/** handleClient 处理单个 share 客户端的完整生命周期 */
func (s *IPCServer) handleClient(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	encoder := json.NewEncoder(conn)

	if !scanner.Scan() {
		return
	}

	var openMsg IPCMessage
	if err := json.Unmarshal(scanner.Bytes(), &openMsg); err != nil || openMsg.Type != IPCShareOpen {
		encoder.Encode(IPCMessage{Type: IPCShareError, Error: "expected share_open"})
		return
	}

	if s.tunnel == nil || s.tunnel.Status() != StatusConnected {
		encoder.Encode(IPCMessage{Type: IPCShareError, Error: "agent not connected to server"})
		return
	}

	sessionID := fmt.Sprintf("share-%d", time.Now().UnixNano())

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}

	client := &IPCClient{
		sessionID: sessionID,
		createdAt: time.Now().UnixMilli(),
		shell:     shell,
		conn:      conn,
		encoder:   encoder,
	}

	s.mu.Lock()
	s.clients[sessionID] = client
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, sessionID)
		s.mu.Unlock()

		closedMsg := proto.NewMessage(proto.TypeSessionClosed, proto.SessionClosedPayload{
			SessionID: sessionID,
			ExitCode:  0,
		})
		s.tunnel.Send(closedMsg)
		log.Printf("[ipc] share session %s ended", sessionID)
	}()

	resumeMsg := proto.NewMessage(proto.TypeSessionResume, proto.SessionResumePayload{
		SessionID: sessionID,
	})
	s.tunnel.Send(resumeMsg)

	client.Send(IPCMessage{
		Type:      IPCShareOpened,
		SessionID: sessionID,
	})

	log.Printf("[ipc] share session %s started (cols=%d, rows=%d)", sessionID, openMsg.Cols, openMsg.Rows)

	for scanner.Scan() {
		var msg IPCMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		switch msg.Type {
		case IPCShareOutput:
			outMsg := proto.NewMessage(proto.TypeSessionOutput, proto.SessionDataPayload{
				SessionID: sessionID,
				Data:      msg.Data,
			})
			s.tunnel.Send(outMsg)

		case IPCShareResize:
			resizeMsg := proto.NewMessage(proto.TypeSessionResize, proto.SessionResizePayload{
				SessionID: sessionID,
				Cols:      msg.Cols,
				Rows:      msg.Rows,
			})
			s.tunnel.Send(resizeMsg)

		case IPCShareClose:
			return
		}
	}
}

/** SendToClient 根据 sessionID 向对应的 share 客户端发送消息 */
func (s *IPCServer) SendToClient(sessionID string, msg IPCMessage) {
	s.mu.RLock()
	client := s.clients[sessionID]
	s.mu.RUnlock()

	if client != nil {
		client.Send(msg)
	}
}

/** HasSession 检查是否存在指定 sessionID 的共享会话 */
func (s *IPCServer) HasSession(sessionID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.clients[sessionID]
	return ok
}

/** ResumeAll 重连后重新注册所有共享会话到服务端 */
func (s *IPCServer) ResumeAll(tunnel *Tunnel) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, client := range s.clients {
		msg := proto.NewMessage(proto.TypeSessionResume, proto.SessionResumePayload{
			SessionID: client.sessionID,
		})
		tunnel.Send(msg)
	}
}

/** ListInfo 返回所有共享会话的摘要信息 */
func (s *IPCServer) ListInfo() []proto.SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]proto.SessionInfo, 0, len(s.clients))
	for _, c := range s.clients {
		list = append(list, proto.SessionInfo{
			SessionID: c.sessionID,
			CreatedAt: c.createdAt,
			Shell:     c.shell,
		})
	}
	return list
}

/** Count 返回当前共享会话数量 */
func (s *IPCServer) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

/** IPCSocketPath 返回默认 IPC socket 路径 ~/.linkterm/agent.sock */
func IPCSocketPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".linkterm", "agent.sock")
}
