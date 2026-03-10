package main

import (
	"encoding/base64"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
	"github.com/linkterm/linkterm/proto"
)

/** Session 表示一个本地 PTY 会话 */
type Session struct {
	ID     string
	ptmx   *os.File
	cmd    *exec.Cmd
	tunnel *Tunnel
	buffer *RingBuffer
	done   chan struct{}
	once   sync.Once
}

/** NewSession 创建新的 PTY 会话 */
func NewSession(id string, cols, rows uint16, shell string, tunnel *Tunnel, bufferSize int) (*Session, error) {
	cmd := exec.Command(shell, "-l")

	home, _ := os.UserHomeDir()
	cmd.Dir = home
	cmd.Env = buildShellEnv(shell, home)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: cols,
		Rows: rows,
	})
	if err != nil {
		return nil, err
	}

	s := &Session{
		ID:     id,
		ptmx:   ptmx,
		cmd:    cmd,
		tunnel: tunnel,
		buffer: NewRingBuffer(bufferSize),
		done:   make(chan struct{}),
	}

	go s.readLoop()
	go s.waitExit()

	log.Printf("[session] %s started (shell=%s, cols=%d, rows=%d)", id, shell, cols, rows)
	return s, nil
}

/** readLoop 从 PTY stdout 读取数据并发送到 tunnel */
func (s *Session) readLoop() {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			data := buf[:n]
			s.buffer.Write(data)

			encoded := base64.StdEncoding.EncodeToString(data)
			msg := proto.NewMessage(proto.TypeSessionOutput, proto.SessionDataPayload{
				SessionID: s.ID,
				Data:      encoded,
			})
			s.tunnel.Send(msg)
		}
		if err != nil {
			return
		}
	}
}

/** waitExit 等待 shell 进程退出 */
func (s *Session) waitExit() {
	exitCode := 0
	if err := s.cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	s.once.Do(func() {
		close(s.done)
	})

	msg := proto.NewMessage(proto.TypeSessionClosed, proto.SessionClosedPayload{
		SessionID: s.ID,
		ExitCode:  exitCode,
	})
	s.tunnel.Send(msg)
	log.Printf("[session] %s exited (code=%d)", s.ID, exitCode)
}

/** HandleInput 写入 PTY stdin */
func (s *Session) HandleInput(dataB64 string) {
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return
	}
	s.ptmx.Write(data)
}

/** Resize 修改终端窗口大小 */
func (s *Session) Resize(cols, rows uint16) {
	ws := struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}{
		Row: rows,
		Col: cols,
	}
	syscall.Syscall(
		syscall.SYS_IOCTL,
		s.ptmx.Fd(),
		syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(&ws)),
	)
}

/** Close 关闭 PTY 会话 */
func (s *Session) Close() {
	s.once.Do(func() {
		close(s.done)
	})
	if s.cmd.Process != nil {
		s.cmd.Process.Signal(syscall.SIGHUP)
	}
	s.ptmx.Close()
	log.Printf("[session] %s closed", s.ID)
}

/** DrainBuffer 返回本地缓冲内容（用于重连后发送给服务端） */
func (s *Session) DrainBuffer() string {
	data := s.buffer.ReadAll()
	if len(data) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}

/**
 * buildShellEnv 构建干净的 shell 初始环境
 * 不继承 Agent（LaunchAgent）的残缺环境，让 login shell 通过 profile 文件自行构建完整 PATH
 */
func buildShellEnv(shell, home string) []string {
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("LOGNAME")
	}

	env := []string{
		"HOME=" + home,
		"USER=" + user,
		"LOGNAME=" + user,
		"SHELL=" + shell,
		"TERM=xterm-256color",
		"LANG=en_US.UTF-8",
	}

	if v := os.Getenv("SSH_AUTH_SOCK"); v != "" {
		env = append(env, "SSH_AUTH_SOCK="+v)
	}
	if v := os.Getenv("TMPDIR"); v != "" {
		env = append(env, "TMPDIR="+v)
	}
	if v := os.Getenv("XPC_FLAGS"); v != "" {
		env = append(env, "XPC_FLAGS="+v)
	}

	return env
}

/** SessionManager 管理本地所有 PTY 会话 */
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	shell    string
	bufSize  int
}

func NewSessionManager(shell string, bufferSize int) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		shell:    shell,
		bufSize:  bufferSize,
	}
}

func (m *SessionManager) Create(id string, cols, rows uint16, tunnel *Tunnel) (*Session, error) {
	s, err := NewSession(id, cols, rows, m.shell, tunnel, m.bufSize)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	return s, nil
}

func (m *SessionManager) Get(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

func (m *SessionManager) Remove(id string) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if s != nil {
		s.Close()
	}
}

func (m *SessionManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

/** ListInfo 返回所有存活会话的摘要信息 */
func (m *SessionManager) ListInfo() []proto.SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]proto.SessionInfo, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, proto.SessionInfo{
			SessionID: s.ID,
			Shell:     m.shell,
		})
	}
	return list
}

/** ResumeAll 重连后恢复所有会话 */
func (m *SessionManager) ResumeAll(tunnel *Tunnel) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		s.tunnel = tunnel
		msg := proto.NewMessage(proto.TypeSessionResume, proto.SessionResumePayload{
			SessionID:      s.ID,
			BufferedOutput: s.DrainBuffer(),
		})
		tunnel.Send(msg)
	}
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
