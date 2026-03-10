package proto

import "encoding/json"

const (
	TypeAuth            = "auth"
	TypeAuthResult      = "auth_result"
	TypePing            = "ping"
	TypePong            = "pong"
	TypeSessionOpen     = "session_open"
	TypeSessionOpened   = "session_opened"
	TypeSessionOpenErr  = "session_open_error"
	TypeSessionInput    = "session_input"
	TypeSessionOutput   = "session_output"
	TypeSessionResize   = "session_resize"
	TypeSessionClose    = "session_close"
	TypeSessionClosed   = "session_closed"
	TypeSessionResume   = "session_resume"
	TypeSessionResumeOK = "session_resume_ok"
)

/** Message 是 Agent ↔ Server 之间所有通信的统一信封 */
type Message struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

/** AuthPayload Agent → Server 认证请求 */
type AuthPayload struct {
	Token    string        `json:"token"`
	Name     string        `json:"name,omitempty"`
	Sessions []SessionInfo `json:"sessions,omitempty"`
}

/** SessionInfo 存活会话的摘要信息 */
type SessionInfo struct {
	SessionID string `json:"session_id"`
	CreatedAt int64  `json:"created_at"`
	Shell     string `json:"shell"`
}

/** AuthResultPayload Server → Agent 认证响应 */
type AuthResultPayload struct {
	OK      bool   `json:"ok"`
	AgentID string `json:"agent_id,omitempty"`
	Error   string `json:"error,omitempty"`
}

/** PingPayload 心跳 */
type PingPayload struct {
	Ts int64 `json:"ts"`
}

/** SessionOpenPayload Server → Agent 请求创建 PTY */
type SessionOpenPayload struct {
	SessionID string `json:"session_id"`
	Cols      uint16 `json:"cols"`
	Rows      uint16 `json:"rows"`
}

/** SessionOpenedPayload Agent → Server PTY 创建成功 */
type SessionOpenedPayload struct {
	SessionID string `json:"session_id"`
}

/** SessionOpenErrPayload Agent → Server PTY 创建失败 */
type SessionOpenErrPayload struct {
	SessionID string `json:"session_id"`
	Error     string `json:"error"`
}

/** SessionDataPayload 终端输入/输出数据，data 为 base64 编码 */
type SessionDataPayload struct {
	SessionID string `json:"session_id"`
	Data      string `json:"data"`
}

/** SessionResizePayload 终端窗口大小变化 */
type SessionResizePayload struct {
	SessionID string `json:"session_id"`
	Cols      uint16 `json:"cols"`
	Rows      uint16 `json:"rows"`
}

/** SessionClosePayload 关闭/已关闭终端 */
type SessionClosePayload struct {
	SessionID string `json:"session_id"`
}

/** SessionClosedPayload Agent → Server 终端已关闭 */
type SessionClosedPayload struct {
	SessionID string `json:"session_id"`
	ExitCode  int    `json:"exit_code"`
}

/** SessionResumePayload Agent → Server 重连后恢复会话 */
type SessionResumePayload struct {
	SessionID      string `json:"session_id"`
	BufferedOutput string `json:"buffered_output,omitempty"`
}

/** SessionResumeOKPayload Server → Agent 确认会话恢复 */
type SessionResumeOKPayload struct {
	SessionID string `json:"session_id"`
}

/** MarshalPayload 将 payload 序列化为 json.RawMessage */
func MarshalPayload(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

/** NewMessage 创建带 payload 的消息 */
func NewMessage(msgType string, payload any) Message {
	return Message{
		Type:    msgType,
		Payload: MarshalPayload(payload),
	}
}
