package main

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/linkterm/linkterm/proto"
	"nhooyr.io/websocket"
)

/** AgentConn 表示一个已连接的 Agent */
type AgentConn struct {
	ID       string
	Token    string
	Name     string
	Conn     *websocket.Conn
	Sessions map[string]bool
	LastPing time.Time
	mu       sync.Mutex
}

/** Send 向 Agent 发送消息 */
func (a *AgentConn) Send(msg proto.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Conn.Write(context.Background(), websocket.MessageText, data)
}

/** Hub 管理所有 Agent 连接 */
type Hub struct {
	agents map[string]*AgentConn
	mu     sync.RWMutex
	config *Config
}

func NewHub(cfg *Config) *Hub {
	return &Hub{
		agents: make(map[string]*AgentConn),
		config: cfg,
	}
}

/** Register 注册一个新的 Agent 连接 */
func (h *Hub) Register(agent *AgentConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if old, ok := h.agents[agent.ID]; ok {
		old.Conn.Close(websocket.StatusGoingAway, "replaced by new connection")
		log.Printf("[hub] agent %s replaced old connection", agent.ID)
	}
	h.agents[agent.ID] = agent
	log.Printf("[hub] agent %s registered (name=%s)", agent.ID, agent.Name)
}

/** Unregister 注销 Agent */
func (h *Hub) Unregister(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.agents, agentID)
	log.Printf("[hub] agent %s unregistered", agentID)
}

/** Get 获取 Agent 连接 */
func (h *Hub) Get(agentID string) *AgentConn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.agents[agentID]
}

/** GetByToken 通过 token 查找 Agent */
func (h *Hub) GetByToken(token string) *AgentConn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, a := range h.agents {
		if a.Token == token {
			return a
		}
	}
	return nil
}

/** ListAgents 返回所有在线 Agent 的 ID 和名称 */
func (h *Hub) ListAgents() []AgentInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	list := make([]AgentInfo, 0, len(h.agents))
	for _, a := range h.agents {
		list = append(list, AgentInfo{
			ID:           a.ID,
			Name:         a.Name,
			SessionCount: len(a.Sessions),
		})
	}
	return list
}

type AgentInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	SessionCount int    `json:"session_count"`
}
