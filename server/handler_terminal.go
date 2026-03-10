package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/linkterm/linkterm/proto"
	"nhooyr.io/websocket"
)

/** handleTerminalWS 处理浏览器端终端 WebSocket 连接 */
func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	sessionID := extractSessionID(r.URL.Path)
	if sessionID == "" {
		http.Error(w, "missing session_id", http.StatusBadRequest)
		return
	}

	token := r.URL.Query().Get("token")
	if _, _, ok := s.auth.ValidateJWT(token); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	session := s.sessions.Get(sessionID)
	if session == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		log.Printf("[terminal-ws] accept error: %v", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "bye")

	conn.SetReadLimit(1 << 16) // 64KB

	buffered := session.Buffer.ReadAll()
	if len(buffered) > 0 {
		sizeMsg := []byte(`{"type":"buffered","size":` + itoa(len(buffered)) + `}`)
		conn.Write(r.Context(), websocket.MessageText, sizeMsg)
		conn.Write(r.Context(), websocket.MessageBinary, buffered)
		log.Printf("[terminal-ws] session %s replayed %d bytes buffer", sessionID, len(buffered))
	}

	session.SetBrowserConn(conn)
	defer session.OnBrowserDisconnect()

	log.Printf("[terminal-ws] browser connected to session %s", sessionID)

	s.terminalReadLoop(r.Context(), conn, session)
}

/** terminalReadLoop 读取浏览器发来的数据并转发到 Agent */
func (s *Server) terminalReadLoop(ctx context.Context, conn *websocket.Conn, session *TerminalSession) {
	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			log.Printf("[terminal-ws] session %s browser read error: %v", session.ID, err)
			return
		}

		switch msgType {
		case websocket.MessageBinary:
			encoded := base64.StdEncoding.EncodeToString(data)
			msg := proto.NewMessage(proto.TypeSessionInput, proto.SessionDataPayload{
				SessionID: session.ID,
				Data:      encoded,
			})
			agent := s.hub.Get(session.AgentID)
			if agent != nil {
				msgData, _ := json.Marshal(msg)
				agent.mu.Lock()
				agent.Conn.Write(ctx, websocket.MessageText, msgData)
				agent.mu.Unlock()
			}

		case websocket.MessageText:
			var ctrl struct {
				Type string `json:"type"`
				Cols uint16 `json:"cols"`
				Rows uint16 `json:"rows"`
				Ts   int64  `json:"ts"`
			}
			if err := json.Unmarshal(data, &ctrl); err != nil {
				continue
			}

			switch ctrl.Type {
			case "resize":
				msg := proto.NewMessage(proto.TypeSessionResize, proto.SessionResizePayload{
					SessionID: session.ID,
					Cols:      ctrl.Cols,
					Rows:      ctrl.Rows,
				})
				agent := s.hub.Get(session.AgentID)
				if agent != nil {
					msgData, _ := json.Marshal(msg)
					agent.mu.Lock()
					agent.Conn.Write(ctx, websocket.MessageText, msgData)
					agent.mu.Unlock()
				}

			case "ping":
				pong := []byte(`{"type":"pong","ts":` + itoa64(ctrl.Ts) + `}`)
				conn.Write(ctx, websocket.MessageText, pong)
			}
		}
	}
}

func extractSessionID(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "ws" && parts[1] == "terminal" {
		return parts[2]
	}
	return ""
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func itoa64(n int64) string {
	return fmt.Sprintf("%d", n)
}
