# Phase 2 — 认证加固 · 长连接 · 会话保持

## 本阶段完成的功能

### 1. Agent ID 稳定化
- Agent ID 基于 Token 的 SHA256 哈希生成，不再使用随机后缀
- Agent 重连后保持同一 ID，服务端可正确关联其历史会话
- Hub 支持 ID 冲突时自动替换旧连接

### 2. 会话持久化（浏览器端）
- `terminal.js` 将 `sessionId` 存入 `localStorage`
- 页面加载时优先尝试恢复已有会话（直接 WebSocket 连接）
- 如果会话不存在（服务端返回 4404），自动查询可复用会话或创建新会话
- 菜单中新增「会话列表」，支持查看所有活跃会话并切换

### 3. 会话状态实时通知
- Agent 断开 → 服务端将其会话标记为 `orphan` 并通过 WebSocket 推送 `session_status: orphan` 给浏览器
- Agent 恢复 → 会话重新激活为 `active`/`detached`，推送 `session_status: active`
- 浏览器状态栏实时显示：「Mac 离线，等待重连...」/「已恢复连接」

### 4. Agent 断线检测
- 服务端心跳改用独立 `context.WithCancel`（不依赖 `r.Context()`，因 WebSocket hijack 后 HTTP context 不再有效）
- 心跳超时（45s 无 pong）→ cancel context → readLoop 退出 → defer 清理（Unregister + Orphan）
- 心跳 Write 失败也触发同样的清理流程

### 5. Agent 主动断开不触发重连
- `Tunnel` 增加 `intentional` 标志
- `Disconnect()` 设置 `intentional = true` → readLoop defer 不触发 `reconnectLoop`
- `Connect()` 重置 `intentional = false`

### 6. 浏览器重连策略
- 指数退避重连：500ms → 1s → 2s → 5s → ... → 30s（最多 20 次）
- `visibilitychange` 监听：页面从后台恢复时立即尝试重连
- 重连 overlay 提示用户当前状态

### 7. 两级缓冲联通
- Agent 本地 `RingBuffer`（128KB）缓存 PTY 输出
- 服务端 `RingBuffer`（64KB）缓存中转输出
- 浏览器连接时回放服务端缓冲，显示「恢复中，回放 X KB 数据...」
- Agent 重连时通过 `session_resume` 上传本地缓冲到服务端

## 修改的文件

| 文件 | 变更说明 |
|------|----------|
| `server/handler_agent.go` | 稳定 Agent ID (SHA256)；独立 context；heartbeat 关闭逻辑 |
| `server/session.go` | `OnAgentReconnect` 方法；`ListActive` 方法；orphan 时推送浏览器 |
| `server/handler_terminal.go` | session 不存在时返回 4404 WebSocket close code |
| `server/handler_web.go` | `handleListSessions` 支持列出所有活跃会话 |
| `server/web/js/terminal.js` | 会话持久化；查询已有会话；会话列表；重连策略 |
| `server/web/terminal.html` | 菜单增加会话列表区域 |
| `server/web/css/style.css` | 会话列表样式 |
| `agent/tunnel.go` | `intentional` 标志；Connect 前 cancel 旧 context |

## 验证结果

- ✅ Agent ID 稳定（多次重连 ID 一致）
- ✅ 创建会话 → WebSocket 连接 → 键盘输入 → PTY 执行 → 输出显示
- ✅ Agent 被 kill → 服务端在 2s 内检测到 → session 标记 orphan
- ✅ Agent 重启 → 重新注册同一 ID → 新会话可正常创建
- ✅ 缓冲回放（连接时接收到 134 bytes prompt 数据）
- ✅ Resize 命令正确转发
