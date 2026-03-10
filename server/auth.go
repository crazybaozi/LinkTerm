package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

/** AgentRecord 已注册的 Agent 记录 */
type AgentRecord struct {
	Token        string    `json:"token"`
	Name         string    `json:"name"`
	AgentID      string    `json:"agent_id"`
	RegisteredAt time.Time `json:"registered_at"`
}

/** AuthManager 处理认证逻辑 */
type AuthManager struct {
	jwtSecret []byte
	dataDir   string
	agents    []AgentRecord
	failures  map[string]*failureRecord
	mu        sync.RWMutex
}

type failureRecord struct {
	Count    int
	LockedAt time.Time
}

func NewAuthManager(cfg *Config) *AuthManager {
	secret := cfg.Auth.JWTSecret
	if secret == "" {
		secret = generateRandomSecret()
		log.Println("[auth] jwt_secret not configured, using auto-generated random secret")
	}
	am := &AuthManager{
		jwtSecret: []byte(secret),
		dataDir:   cfg.Auth.DataDir,
		failures:  make(map[string]*failureRecord),
	}
	am.loadAgents()
	return am
}

func generateRandomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "fallback-" + hex.EncodeToString(sha256.New().Sum(nil))
	}
	return hex.EncodeToString(b)
}

/** loadAgents 从 agents.json 加载已注册的 Agent */
func (a *AuthManager) loadAgents() {
	path := filepath.Join(a.dataDir, "agents.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			a.agents = []AgentRecord{}
			return
		}
		log.Printf("[auth] failed to load agents.json: %v", err)
		a.agents = []AgentRecord{}
		return
	}
	if err := json.Unmarshal(data, &a.agents); err != nil {
		log.Printf("[auth] failed to parse agents.json: %v", err)
		a.agents = []AgentRecord{}
	}
	log.Printf("[auth] loaded %d agents from agents.json", len(a.agents))
}

/** saveAgents 将 Agent 列表写入 agents.json */
func (a *AuthManager) saveAgents() {
	if err := os.MkdirAll(a.dataDir, 0755); err != nil {
		log.Printf("[auth] failed to create data dir: %v", err)
		return
	}
	data, err := json.MarshalIndent(a.agents, "", "  ")
	if err != nil {
		log.Printf("[auth] failed to marshal agents: %v", err)
		return
	}
	path := filepath.Join(a.dataDir, "agents.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("[auth] failed to write agents.json: %v", err)
	}
}

/** agentIDFromToken 根据 token 生成确定性的 agent_id */
func agentIDFromToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return "agent-" + hex.EncodeToString(h[:8])
}

/** RegisterOrUpdate Agent 连接时调用：已知 token 则更新 name，未知则自动注册 */
func (a *AuthManager) RegisterOrUpdate(token, name string) *AgentRecord {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i := range a.agents {
		if a.agents[i].Token == token {
			if name != "" {
				a.agents[i].Name = name
			}
			a.saveAgents()
			return &a.agents[i]
		}
	}

	record := AgentRecord{
		Token:        token,
		Name:         name,
		AgentID:      agentIDFromToken(token),
		RegisteredAt: time.Now(),
	}
	a.agents = append(a.agents, record)
	a.saveAgents()
	log.Printf("[auth] new agent registered: %s (name=%s)", record.AgentID, name)
	return &record
}

/** FindByToken 根据 token 查找已注册的 Agent（浏览器登录用） */
func (a *AuthManager) FindByToken(token string, remoteIP string) (*AgentRecord, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if rec, ok := a.failures[remoteIP]; ok {
		if rec.Count >= 5 && time.Since(rec.LockedAt) < 15*time.Minute {
			return nil, false
		}
		if time.Since(rec.LockedAt) >= 15*time.Minute {
			delete(a.failures, remoteIP)
		}
	}

	for i := range a.agents {
		if a.agents[i].Token == token {
			delete(a.failures, remoteIP)
			return &a.agents[i], true
		}
	}

	rec, ok := a.failures[remoteIP]
	if !ok {
		rec = &failureRecord{}
		a.failures[remoteIP] = rec
	}
	rec.Count++
	rec.LockedAt = time.Now()
	return nil, false
}

/** IssueJWT 为浏览器签发 JWT，包含 agent_id 和 agent_name */
func (a *AuthManager) IssueJWT(agentID, agentName string) (string, error) {
	claims := jwt.MapClaims{
		"agent_id":   agentID,
		"agent_name": agentName,
		"exp":        time.Now().Add(24 * time.Hour).Unix(),
		"iat":        time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(a.jwtSecret)
}

/** ValidateJWT 校验浏览器 JWT，返回 agent_id 和 agent_name */
func (a *AuthManager) ValidateJWT(tokenStr string) (agentID string, agentName string, ok bool) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return a.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return "", "", false
	}
	claims, valid := token.Claims.(jwt.MapClaims)
	if !valid {
		return "", "", false
	}
	agentID, _ = claims["agent_id"].(string)
	agentName, _ = claims["agent_name"].(string)
	return agentID, agentName, true
}
