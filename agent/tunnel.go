package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/linkterm/linkterm/proto"
	"nhooyr.io/websocket"
)

/** TunnelStatus 隧道连接状态 */
type TunnelStatus int

const (
	StatusDisconnected TunnelStatus = iota
	StatusConnecting
	StatusConnected
	StatusReconnecting
)

func (s TunnelStatus) String() string {
	switch s {
	case StatusDisconnected:
		return "disconnected"
	case StatusConnecting:
		return "connecting"
	case StatusConnected:
		return "connected"
	case StatusReconnecting:
		return "reconnecting"
	}
	return "unknown"
}

/** Tunnel 管理 Agent 到 Server 的 WebSocket 隧道 */
type Tunnel struct {
	serverURL      string
	serverName     string
	token          string
	agentID        string
	conn           *websocket.Conn
	connMu         sync.Mutex
	status         TunnelStatus
	sessions       *SessionManager
	config         *Config
	onStatus       func(TunnelStatus, string)
	onReconnect    func()
	ctx            context.Context
	cancel         context.CancelFunc
	intentional    bool
}

func NewTunnel(cfg *Config, sessions *SessionManager) *Tunnel {
	return &Tunnel{
		token:    cfg.Token,
		sessions: sessions,
		config:   cfg,
	}
}

/** SetStatusCallback 注册状态变化回调 */
func (t *Tunnel) SetStatusCallback(cb func(TunnelStatus, string)) {
	t.onStatus = cb
}

func (t *Tunnel) setStatus(s TunnelStatus, msg string) {
	t.status = s
	if t.onStatus != nil {
		t.onStatus(s, msg)
	}
}

/** Connect 连接到指定服务端 */
func (t *Tunnel) Connect(serverURL, serverName string) error {
	t.serverURL = serverURL
	t.serverName = serverName
	t.intentional = false
	t.setStatus(StatusConnecting, fmt.Sprintf("connecting to %s", serverName))

	if t.cancel != nil {
		t.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.ctx = ctx
	t.cancel = cancel

	wsURL := serverURL + "/ws/agent?token=" + t.token
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		Subprotocols: []string{"linkterm"},
	})
	if err != nil {
		cancel()
		return fmt.Errorf("dial %s: %w", serverURL, err)
	}

	conn.SetReadLimit(1 << 20) // 1MB

	t.connMu.Lock()
	t.conn = conn
	t.connMu.Unlock()

	if err := t.authenticate(ctx, conn); err != nil {
		conn.Close(websocket.StatusNormalClosure, "auth failed")
		cancel()
		return fmt.Errorf("auth: %w", err)
	}

	t.setStatus(StatusConnected, fmt.Sprintf("connected to %s", serverName))
	log.Printf("[tunnel] connected to %s (%s), agent_id=%s", serverName, serverURL, t.agentID)

	t.sessions.ResumeAll(t)

	go t.heartbeat(ctx)
	go t.readLoop(ctx)

	return nil
}

/** authenticate 发送认证消息并等待响应 */
func (t *Tunnel) authenticate(ctx context.Context, conn *websocket.Conn) error {
	authMsg := proto.NewMessage(proto.TypeAuth, proto.AuthPayload{
		Token:    t.token,
		Name:     t.config.Name,
		Sessions: t.sessions.ListInfo(),
	})
	data, _ := json.Marshal(authMsg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return err
	}

	readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, respData, err := conn.Read(readCtx)
	if err != nil {
		return fmt.Errorf("read auth_result: %w", err)
	}

	var msg proto.Message
	if err := json.Unmarshal(respData, &msg); err != nil {
		return err
	}

	var result proto.AuthResultPayload
	if err := json.Unmarshal(msg.Payload, &result); err != nil {
		return err
	}

	if !result.OK {
		return fmt.Errorf("auth rejected: %s", result.Error)
	}

	t.agentID = result.AgentID
	return nil
}

/** SetReconnectHandler 注册外部重连处理器（供 Selector 使用） */
func (t *Tunnel) SetReconnectHandler(handler func()) {
	t.onReconnect = handler
}

/** readLoop 持续读取服务端发来的消息 */
func (t *Tunnel) readLoop(ctx context.Context) {
	defer func() {
		t.setStatus(StatusDisconnected, "disconnected")
		if !t.intentional {
			if t.onReconnect != nil {
				go t.onReconnect()
			} else {
				go t.reconnectLoop()
			}
		}
	}()

	for {
		_, data, err := t.conn.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[tunnel] read error: %v", err)
			return
		}

		var msg proto.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("[tunnel] unmarshal error: %v", err)
			continue
		}

		t.handleMessage(msg)
	}
}

/** handleMessage 分发服务端消息 */
func (t *Tunnel) handleMessage(msg proto.Message) {
	switch msg.Type {
	case proto.TypePing:
		pong := proto.NewMessage(proto.TypePong, proto.PingPayload{Ts: time.Now().UnixMilli()})
		t.Send(pong)

	case proto.TypeSessionOpen:
		var p proto.SessionOpenPayload
		json.Unmarshal(msg.Payload, &p)
		t.handleSessionOpen(p)

	case proto.TypeSessionInput:
		var p proto.SessionDataPayload
		json.Unmarshal(msg.Payload, &p)
		if s := t.sessions.Get(p.SessionID); s != nil {
			s.HandleInput(p.Data)
		}

	case proto.TypeSessionResize:
		var p proto.SessionResizePayload
		json.Unmarshal(msg.Payload, &p)
		if s := t.sessions.Get(p.SessionID); s != nil {
			s.Resize(p.Cols, p.Rows)
		}

	case proto.TypeSessionClose:
		var p proto.SessionClosePayload
		json.Unmarshal(msg.Payload, &p)
		t.sessions.Remove(p.SessionID)

	case proto.TypeSessionResumeOK:
		var p proto.SessionResumeOKPayload
		json.Unmarshal(msg.Payload, &p)
		log.Printf("[tunnel] session %s resume confirmed", p.SessionID)
	}
}

func (t *Tunnel) handleSessionOpen(p proto.SessionOpenPayload) {
	if t.sessions.Count() >= t.config.MaxSessions {
		errMsg := proto.NewMessage(proto.TypeSessionOpenErr, proto.SessionOpenErrPayload{
			SessionID: p.SessionID,
			Error:     "max sessions reached",
		})
		t.Send(errMsg)
		return
	}

	_, err := t.sessions.Create(p.SessionID, p.Cols, p.Rows, t)
	if err != nil {
		errMsg := proto.NewMessage(proto.TypeSessionOpenErr, proto.SessionOpenErrPayload{
			SessionID: p.SessionID,
			Error:     err.Error(),
		})
		t.Send(errMsg)
		log.Printf("[tunnel] failed to open session %s: %v", p.SessionID, err)
		return
	}

	openedMsg := proto.NewMessage(proto.TypeSessionOpened, proto.SessionOpenedPayload{
		SessionID: p.SessionID,
	})
	t.Send(openedMsg)
}

/** Send 发送消息到服务端 */
func (t *Tunnel) Send(msg proto.Message) {
	t.connMu.Lock()
	conn := t.conn
	t.connMu.Unlock()

	if conn == nil {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	conn.Write(context.Background(), websocket.MessageText, data)
}

/** heartbeat 定时发送心跳 */
func (t *Tunnel) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			msg := proto.NewMessage(proto.TypePong, proto.PingPayload{Ts: time.Now().UnixMilli()})
			t.Send(msg)
		}
	}
}

/** reconnectLoop 断线后自动重连 */
func (t *Tunnel) reconnectLoop() {
	t.setStatus(StatusReconnecting, "reconnecting...")

	backoffs := []time.Duration{
		500 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		5 * time.Second,
		10 * time.Second,
		30 * time.Second,
	}
	maxInterval := time.Duration(t.config.ReconnectMaxInterval) * time.Second
	if maxInterval == 0 {
		maxInterval = 30 * time.Second
	}

	for attempt := 0; ; attempt++ {
		delay := backoffs[min(attempt, len(backoffs)-1)]
		if delay > maxInterval {
			delay = maxInterval
		}

		log.Printf("[tunnel] reconnecting in %v (attempt %d)", delay, attempt+1)
		time.Sleep(delay)

		err := t.Connect(t.serverURL, t.serverName)
		if err == nil {
			return
		}
		log.Printf("[tunnel] reconnect failed: %v", err)
	}
}

/** Disconnect 主动断开连接 */
func (t *Tunnel) Disconnect() {
	t.intentional = true
	if t.cancel != nil {
		t.cancel()
	}
	t.connMu.Lock()
	conn := t.conn
	t.conn = nil
	t.connMu.Unlock()

	if conn != nil {
		conn.Close(websocket.StatusNormalClosure, "user disconnect")
	}
	t.setStatus(StatusDisconnected, "disconnected")
}

/** RegenerateToken 重新生成 token：生成新 token → 保存 → 断开（触发自动重连） */
func (t *Tunnel) RegenerateToken(configPath string) string {
	newToken := RegenerateToken(configPath, t.config)
	t.token = newToken
	t.Disconnect()
	return newToken
}

/** Status 返回当前状态 */
func (t *Tunnel) Status() TunnelStatus {
	return t.status
}

/** ServerName 返回当前连接的服务端名称 */
func (t *Tunnel) ServerName() string {
	return t.serverName
}
