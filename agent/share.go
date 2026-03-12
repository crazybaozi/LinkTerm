package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

/** runShare 执行 linkterm share 命令：共享当前终端到手机端 */
func runShare(args []string) {
	sockPath := IPCSocketPath()
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m[LinkTerm]\033[0m Cannot connect to agent: %v\n", err)
		fmt.Fprintf(os.Stderr, "\033[31m[LinkTerm]\033[0m Make sure the agent is running\n")
		os.Exit(1)
	}
	defer conn.Close()

	fd := int(os.Stdin.Fd())
	cols, rows, err := term.GetSize(fd)
	if err != nil {
		cols, rows = 80, 24
	}

	ipcWriter := &syncWriter{encoder: json.NewEncoder(conn)}

	ipcWriter.Send(IPCMessage{
		Type: IPCShareOpen,
		Cols: uint16(cols),
		Rows: uint16(rows),
	})

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	if !scanner.Scan() {
		fmt.Fprintf(os.Stderr, "\033[31m[LinkTerm]\033[0m Failed to read response from agent\n")
		os.Exit(1)
	}

	var resp IPCMessage
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m[LinkTerm]\033[0m Invalid response: %v\n", err)
		os.Exit(1)
	}

	if resp.Type == IPCShareError {
		fmt.Fprintf(os.Stderr, "\033[31m[LinkTerm]\033[0m %s\n", resp.Error)
		os.Exit(1)
	}

	if resp.Type != IPCShareOpened {
		fmt.Fprintf(os.Stderr, "\033[31m[LinkTerm]\033[0m Unexpected response: %s\n", resp.Type)
		os.Exit(1)
	}

	sessionID := resp.SessionID

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}

	var cmd *exec.Cmd
	if len(args) > 0 {
		cmd = exec.Command(args[0], args[1:]...)
	} else {
		cmd = exec.Command(shell, "-l")
	}
	cmd.Env = os.Environ()

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m[LinkTerm]\033[0m Failed to start PTY: %v\n", err)
		ipcWriter.Send(IPCMessage{Type: IPCShareClose, SessionID: sessionID})
		os.Exit(1)
	}
	defer ptmx.Close()

	fmt.Printf("\033[32m[LinkTerm]\033[0m Terminal shared (session: %s)\n", sessionID)
	fmt.Printf("\033[32m[LinkTerm]\033[0m Phone can now connect to see this terminal\n\n")

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m[LinkTerm]\033[0m Failed to set raw mode: %v\n", err)
		ipcWriter.Send(IPCMessage{Type: IPCShareClose, SessionID: sessionID})
		cmd.Process.Signal(syscall.SIGHUP)
		os.Exit(1)
	}
	defer func() {
		term.Restore(fd, oldState)
		fmt.Printf("\r\n\033[32m[LinkTerm]\033[0m Share ended\r\n")
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			c, r, err := term.GetSize(fd)
			if err != nil {
				continue
			}
			pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(c), Rows: uint16(r)})
			ipcWriter.Send(IPCMessage{
				Type:      IPCShareResize,
				SessionID: sessionID,
				Cols:      uint16(c),
				Rows:      uint16(r),
			})
		}
	}()

	done := make(chan struct{})

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				data := buf[:n]
				os.Stdout.Write(data)

				encoded := base64.StdEncoding.EncodeToString(data)
				ipcWriter.Send(IPCMessage{
					Type:      IPCShareOutput,
					SessionID: sessionID,
					Data:      encoded,
				})
			}
			if err != nil {
				close(done)
				return
			}
		}
	}()

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				ptmx.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	go func() {
		for scanner.Scan() {
			var msg IPCMessage
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				continue
			}
			switch msg.Type {
			case IPCShareInput:
				data, err := base64.StdEncoding.DecodeString(msg.Data)
				if err == nil {
					ptmx.Write(data)
				}
			case IPCShareResize:
				pty.Setsize(ptmx, &pty.Winsize{Cols: msg.Cols, Rows: msg.Rows})
			case IPCShareClosed:
				ptmx.Close()
				cmd.Process.Signal(syscall.SIGHUP)
				return
			}
		}
	}()

	<-done
	cmd.Wait()
	ipcWriter.Send(IPCMessage{Type: IPCShareClose, SessionID: sessionID})
}

/** syncWriter 线程安全的 JSON 编码写入器 */
type syncWriter struct {
	encoder *json.Encoder
	mu      sync.Mutex
}

func (w *syncWriter) Send(msg IPCMessage) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.encoder.Encode(msg)
}
