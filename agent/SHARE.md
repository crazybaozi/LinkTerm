# linkterm share - 终端共享功能

## 功能说明

`linkterm share` 允许用户在 Mac 的 Terminal.app / iTerm2 中执行一条命令，将当前终端共享到手机端。手机连接后可以实时看到终端输出，并且也能输入命令。

## 使用方式

### 基本用法

在任意终端窗口中执行：

```bash
linkterm-agent share
```

此时会启动一个新的 login shell，所有输入输出会同时显示在：
- 当前 Mac 终端窗口
- 手机端浏览器

### 共享指定命令

```bash
linkterm-agent share claude
linkterm-agent share -- claude --dangerously-skip-permissions
```

使用 `--` 分隔可以传递带参数的命令。

### 结束共享

- 输入 `exit` 或按 `Ctrl+D` 退出 shell
- 如果是指定命令模式，命令结束后自动退出共享

## 架构设计

```
Terminal.app ─── stdin/stdout ───┐
                                 │
              linkterm share     ├──→ PTY (shell) ──→ Agent ──→ Server ──→ 手机
                                 │
Terminal.app 正常显示 ←──────────┘
```

### 通信链路

```
linkterm share  ←── Unix Socket ──→  Agent  ←── WebSocket ──→  Server  ←── WebSocket ──→  手机浏览器
  (PTY owner)       (~/.linkterm/        (relay)                 (relay)                    (xterm.js)
                     agent.sock)
```

### IPC 协议

`linkterm share` 与 Agent 之间通过 Unix Domain Socket (`~/.linkterm/agent.sock`) 通信，使用换行分隔的 JSON 消息：

| 消息类型 | 方向 | 说明 |
|---------|------|------|
| `share_open` | share → agent | 请求共享，携带终端尺寸 |
| `share_opened` | agent → share | 确认共享，返回 session_id |
| `share_output` | share → agent | PTY 输出（base64），转发到服务端 |
| `share_input` | agent → share | 手机端输入（base64），写入 PTY |
| `share_resize` | 双向 | 终端窗口大小变化 |
| `share_close` | share → agent | 结束共享 |
| `share_closed` | agent → share | 服务端关闭会话 |
| `share_error` | agent → share | 错误信息 |

### 会话注册

共享会话通过 `session_resume` 消息注册到服务端（复用现有协议）。服务端视其为普通终端会话，手机端可以直接连接。

## 涉及文件

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `agent/ipc.go` | 新增 | IPC Server + 协议定义 |
| `agent/share.go` | 新增 | share 命令实现（PTY、raw mode、I/O relay） |
| `agent/main.go` | 修改 | 添加 `share` 子命令路由，headless 模式启动 IPC |
| `agent/tunnel.go` | 修改 | 添加 IPC 字段，路由 session_input/resize/close 到共享会话 |
| `agent/tray.go` | 修改 | 启动 IPC Server，终端计数包含共享会话 |

## 前置条件

- Agent 必须正在运行（菜单栏模式或 headless 模式）
- Agent 必须已连接到服务端

## 限制

- 需要在启动长任务**之前**执行 `linkterm share`，无法回溯接入已运行的进程（操作系统限制）
- 建议使用 alias 简化：`alias claude='linkterm-agent share -- claude'`
