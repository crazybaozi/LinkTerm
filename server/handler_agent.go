package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/linkterm/linkterm/proto"
	"nhooyr.io/websocket"
)

/** handleAgentWS 处理 Agent WebSocket 连接 */
func (s *Server) handleAgentWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		log.Printf("[agent-ws] accept error: %v", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "bye")

	conn.SetReadLimit(1 << 20) // 1MB

	authCtx := context.Background()

	agent, err := s.authenticateAgent(authCtx, conn)
	if err != nil {
		log.Printf("[agent-ws] auth failed: %v", err)
		conn.Close(websocket.StatusPolicyViolation, "auth failed")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.hub.Register(agent)
	defer func() {
		s.hub.Unregister(agent.ID)
		s.sessions.OrphanByAgent(agent.ID)
	}()

	go s.agentHeartbeat(ctx, cancel, agent)

	s.agentReadLoop(ctx, agent)
}

/** authenticateAgent 等待 Agent 发送认证消息，自动注册未知 token */
func (s *Server) authenticateAgent(ctx context.Context, conn *websocket.Conn) (*AgentConn, error) {
	authCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, data, err := conn.Read(authCtx)
	if err != nil {
		return nil, fmt.Errorf("read auth message: %w", err)
	}

	var msg proto.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	if msg.Type != proto.TypeAuth {
		return nil, fmt.Errorf("expected auth, got %s", msg.Type)
	}

	var payload proto.AuthPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal auth payload: %w", err)
	}

	if payload.Token == "" {
		resp := proto.NewMessage(proto.TypeAuthResult, proto.AuthResultPayload{
			OK:    false,
			Error: "empty token",
		})
		respData, _ := json.Marshal(resp)
		conn.Write(ctx, websocket.MessageText, respData)
		return nil, fmt.Errorf("empty token")
	}

	record := s.auth.RegisterOrUpdate(payload.Token, payload.Name)

	agent := &AgentConn{
		ID:       record.AgentID,
		Token:    payload.Token,
		Name:     record.Name,
		Conn:     conn,
		Sessions: make(map[string]bool),
		LastPing: time.Now(),
	}

	resp := proto.NewMessage(proto.TypeAuthResult, proto.AuthResultPayload{
		OK:      true,
		AgentID: record.AgentID,
	})
	respData, _ := json.Marshal(resp)
	conn.Write(ctx, websocket.MessageText, respData)

	for _, si := range payload.Sessions {
		agent.Sessions[si.SessionID] = true
	}

	log.Printf("[agent-ws] agent %s authenticated (name=%s, existing_sessions=%d)", record.AgentID, record.Name, len(payload.Sessions))
	return agent, nil
}

/** agentReadLoop 读取 Agent 发来的消息并分发 */
func (s *Server) agentReadLoop(ctx context.Context, agent *AgentConn) {
	for {
		_, data, err := agent.Conn.Read(ctx)
		if err != nil {
			log.Printf("[agent-ws] agent %s read error: %v", agent.ID, err)
			return
		}

		var msg proto.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("[agent-ws] agent %s unmarshal error: %v", agent.ID, err)
			continue
		}

		switch msg.Type {
		case proto.TypePong:
			agent.LastPing = time.Now()

		case proto.TypeSessionOpened:
			var p proto.SessionOpenedPayload
			json.Unmarshal(msg.Payload, &p)
			agent.Sessions[p.SessionID] = true
			log.Printf("[agent-ws] agent %s session %s opened", agent.ID, p.SessionID)

		case proto.TypeSessionOpenErr:
			var p proto.SessionOpenErrPayload
			json.Unmarshal(msg.Payload, &p)
			log.Printf("[agent-ws] agent %s session %s open error: %s", agent.ID, p.SessionID, p.Error)
			s.sessions.Remove(p.SessionID)

		case proto.TypeSessionOutput:
			var p proto.SessionDataPayload
			json.Unmarshal(msg.Payload, &p)
			session := s.sessions.Get(p.SessionID)
			if session != nil {
				decoded, err := base64.StdEncoding.DecodeString(p.Data)
				if err == nil {
					session.WriteToBrowser(decoded)
				}
			}

		case proto.TypeSessionClosed:
			var p proto.SessionClosedPayload
			json.Unmarshal(msg.Payload, &p)
			log.Printf("[agent-ws] agent %s session %s closed (exit_code=%d)", agent.ID, p.SessionID, p.ExitCode)
			session := s.sessions.Get(p.SessionID)
			if session != nil {
				session.mu.Lock()
				session.Status = StatusClosed
				if session.BrowserWS != nil {
					closedMsg := fmt.Sprintf(`{"type":"closed","exit_code":%d}`, p.ExitCode)
					session.BrowserWS.Write(context.Background(), websocket.MessageText, []byte(closedMsg))
				}
				session.mu.Unlock()
			}
			delete(agent.Sessions, p.SessionID)

		case proto.TypeSessionResume:
			var p proto.SessionResumePayload
			json.Unmarshal(msg.Payload, &p)
			session := s.sessions.Get(p.SessionID)
			if session == nil {
				session = s.sessions.Create(p.SessionID, agent.ID)
			}

			if p.BufferedOutput != "" {
				decoded, err := base64.StdEncoding.DecodeString(p.BufferedOutput)
				if err == nil {
					session.Buffer.Write(decoded)
				}
			}

			session.OnAgentReconnect(agent.ID)
			agent.Sessions[p.SessionID] = true

			respMsg := proto.NewMessage(proto.TypeSessionResumeOK, proto.SessionResumeOKPayload{
				SessionID: p.SessionID,
			})
			respData, _ := json.Marshal(respMsg)
			agent.Conn.Write(context.Background(), websocket.MessageText, respData)
			log.Printf("[agent-ws] agent %s session %s resumed", agent.ID, p.SessionID)
		}
	}
}

/** agentHeartbeat 定期向 Agent 发送心跳，超时时 cancel context 以释放 readLoop */
func (s *Server) agentHeartbeat(ctx context.Context, cancel context.CancelFunc, agent *AgentConn) {
	ticker := time.NewTicker(s.config.Heartbeat.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Since(agent.LastPing) >= s.config.Heartbeat.Timeout {
				log.Printf("[agent-ws] agent %s heartbeat timeout (last_pong=%v ago)", agent.ID, time.Since(agent.LastPing).Round(time.Second))
				cancel()
				return
			}

			msg := proto.NewMessage(proto.TypePing, proto.PingPayload{Ts: time.Now().UnixMilli()})
			data, _ := json.Marshal(msg)
			agent.mu.Lock()
			writeCtx, wcancel := context.WithTimeout(ctx, 10*time.Second)
			err := agent.Conn.Write(writeCtx, websocket.MessageText, data)
			wcancel()
			agent.mu.Unlock()
			if err != nil {
				log.Printf("[agent-ws] agent %s heartbeat write failed: %v", agent.ID, err)
				cancel()
				return
			}
		}
	}
}
