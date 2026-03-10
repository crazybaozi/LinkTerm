package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/linkterm/linkterm/proto"
	"nhooyr.io/websocket"
)

//go:embed web
var webFS embed.FS

/** Server 聚合所有服务端组件 */
type Server struct {
	config   *Config
	hub      *Hub
	sessions *SessionManager
	auth     *AuthManager
	mux      *http.ServeMux
}

func NewServer(cfg *Config) *Server {
	s := &Server{
		config:   cfg,
		hub:      NewHub(cfg),
		sessions: NewSessionManager(cfg.Session.BufferSize),
		auth:     NewAuthManager(cfg),
		mux:      http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/health/ping", s.handleHealthPing)
	s.mux.HandleFunc("/api/auth", s.handleAPIAuth)
	s.mux.HandleFunc("/api/agents", s.handleAPIAgents)
	s.mux.HandleFunc("/api/sessions", s.handleAPISessions)
	s.mux.HandleFunc("/ws/agent", s.handleAgentWS)
	s.mux.HandleFunc("/ws/terminal/", s.handleTerminalWS)

	webRoot, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	fileServer := http.FileServer(http.FS(webRoot))
	s.mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sw.js" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Service-Worker-Allowed", "/")
		}
		fileServer.ServeHTTP(w, r)
	}))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

/** handleHealthPing 测速接口 */
func (s *Server) handleHealthPing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true,"ts":%d,"region":"%s"}`, time.Now().UnixMilli(), s.config.Region)
}

/** handleAPIAuth 浏览器登录（使用 Agent Token） */
func (s *Server) handleAPIAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	remoteIP := strings.Split(r.RemoteAddr, ":")[0]
	record, ok := s.auth.FindByToken(req.Token, remoteIP)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid token or locked"})
		return
	}

	jwtToken, err := s.auth.IssueJWT(record.AgentID, record.Name)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":      jwtToken,
		"agent_id":   record.AgentID,
		"agent_name": record.Name,
	})
}

/** handleAPIAgents 获取当前用户有权限的在线 Agent */
func (s *Server) handleAPIAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := extractBearerToken(r)
	agentID, _, ok := s.auth.ValidateJWT(token)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	allAgents := s.hub.ListAgents()
	var filtered []AgentInfo
	for _, a := range allAgents {
		if a.ID == agentID {
			filtered = append(filtered, a)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filtered)
}

/** handleAPISessions 会话管理 */
func (s *Server) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	agentID, _, ok := s.auth.ValidateJWT(token)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleListSessions(w, r, agentID)
	case http.MethodPost:
		s.handleCreateSession(w, r, agentID)
	case http.MethodDelete:
		s.handleDeleteSession(w, r, agentID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request, jwtAgentID string) {
	type sessionResp struct {
		ID        string `json:"id"`
		AgentID   string `json:"agent_id"`
		Status    string `json:"status"`
		CreatedAt int64  `json:"created_at"`
	}

	resp := []sessionResp{}
	sessions := s.sessions.ListByAgent(jwtAgentID)
	for _, sess := range sessions {
		resp = append(resp, sessionResp{
			ID:        sess.ID,
			AgentID:   sess.AgentID,
			Status:    string(sess.Status),
			CreatedAt: sess.CreatedAt.UnixMilli(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request, jwtAgentID string) {
	var req struct {
		Cols uint16 `json:"cols"`
		Rows uint16 `json:"rows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	agent := s.hub.Get(jwtAgentID)
	if agent == nil {
		http.Error(w, "agent not found or offline", http.StatusNotFound)
		return
	}

	if s.sessions.CountByAgent(jwtAgentID) >= s.config.Session.MaxPerAgent {
		http.Error(w, "max sessions reached", http.StatusTooManyRequests)
		return
	}

	sessionID := fmt.Sprintf("sess-%d", time.Now().UnixNano())
	s.sessions.Create(sessionID, jwtAgentID)

	cols := req.Cols
	rows := req.Rows
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}

	openMsg := proto.NewMessage(proto.TypeSessionOpen, proto.SessionOpenPayload{
		SessionID: sessionID,
		Cols:      cols,
		Rows:      rows,
	})
	msgData, _ := json.Marshal(openMsg)
	agent.mu.Lock()
	err := agent.Conn.Write(r.Context(), websocket.MessageText, msgData)
	agent.mu.Unlock()
	if err != nil {
		s.sessions.Remove(sessionID)
		http.Error(w, "failed to reach agent", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"session_id": sessionID})
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request, jwtAgentID string) {
	sessionID := r.URL.Query().Get("id")
	if sessionID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	session := s.sessions.Get(sessionID)
	if session == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	if session.AgentID != jwtAgentID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	agent := s.hub.Get(session.AgentID)
	if agent != nil {
		closeMsg := proto.NewMessage(proto.TypeSessionClose, proto.SessionClosePayload{
			SessionID: sessionID,
		})
		msgData, _ := json.Marshal(closeMsg)
		agent.mu.Lock()
		agent.Conn.Write(r.Context(), websocket.MessageText, msgData)
		agent.mu.Unlock()
	}

	s.sessions.Remove(sessionID)
	w.WriteHeader(http.StatusNoContent)
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return r.URL.Query().Get("token")
}
