# LinkTerm

**手机浏览器远程操作 Mac 终端，打开即用，无需安装任何 App。**

```
手机/iPad (浏览器)  ──HTTPS/WSS──>  服务端 (云服务器)  <──WSS──  Mac Agent
```

---

## 特性

- **零安装** — 手机端纯浏览器，不装任何 App
- **PWA 支持** — 添加到主屏幕，全屏使用，体验接近原生
- **会话保持** — 锁屏再回来终端还在，断线自动重连 + 缓冲回放
- **菜单栏常驻** — Mac 菜单栏显示连接状态，一键复制 Token
- **智能选路** — 多节点自动测速，选延迟最低的连接
- **防睡眠** — Mac 合盖接电源不断连
- **安全** — Token 认证 + JWT + 暴力破解防护 + 全链路 TLS
- **一键安装** — 一行命令完成 Agent 安装、配置、启动

---

## 快速开始

### 1. 部署服务端

```bash
git clone https://github.com/crazybaozi/LinkTerm.git
cd LinkTerm

# 编辑配置，修改 jwt_secret 为随机字符串
vi deploy/config.yaml

# 启动
docker compose up -d
```

### 2. 安装 Mac Agent

```bash
bash scripts/install.sh
```

脚本自动完成：检测架构 → 安装 → 配置服务端地址 → 生成 Token → 开机自启 → 启动。

也可指定地址跳过交互：`bash scripts/install.sh --server wss://你的域名`

### 3. 手机访问

1. 手机浏览器打开 `https://你的服务器域名`
2. 输入 Token（Mac 菜单栏点击图标即可复制）
3. 进入终端，开始操作

> 推荐添加到主屏幕：iOS Safari 分享 → 添加到主屏幕 / Android Chrome → 安装应用

---

## Mac 菜单栏图标

Agent 启动后会在 Mac 菜单栏显示一个 **L⚡** 图标，闪电颜色表示连接状态：

| 闪电颜色 | 含义 |
|:---|:---|
| 🟢 绿色 | 已连接 |
| 🔴 红色 | 未连接 |
| 🟡 黄色 | 连接中 / 重连中 |

点击图标展开操作面板：

```
┌──────────────────────────────────┐
│  🟢 已连接                        │
│  服务器: 主节点                    │
│  活跃终端: 2                      │
│  ──────────────────────────────  │
│  访问本机 Token                   │  ← 点击复制
│  服务器地址: wss://term.xx.com    │  ← 点击复制
│  ──────────────────────────────  │
│  重新连接                         │
│  ──────────────────────────────  │
│  退出 LinkTerm                    │
└──────────────────────────────────┘
```

如需无图标模式运行：`linkterm-agent --no-tray`

---

## Agent 管理

```bash
# 查看状态
bash scripts/install.sh --status

# 停止
launchctl bootout gui/$(id -u)/com.linkterm.agent

# 启动
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.linkterm.agent.plist

# 查看日志
tail -f ~/.linkterm/agent.log

# 卸载
bash scripts/install.sh --uninstall
```

---

## 配置说明

### 服务端（`deploy/config.yaml`）

```yaml
listen: ":8080"
auth:
  jwt_secret: "改为随机字符串"
  data_dir: "./data"
session:
  max_per_agent: 10
  buffer_size: 65536
heartbeat:
  interval: 15s
  timeout: 45s
```

### Agent（`~/.linkterm/config.yaml`）

```yaml
servers:
  - url: "wss://your-server.com"
    name: "主节点"
token: ""                       # 留空自动生成
prevent_sleep: true             # 合盖不断连
max_sessions: 10
```

---

## 常见问题

| 问题 | 解决 |
|:---|:---|
| Mac 离线 | 检查 Agent 是否运行、server URL 是否正确 |
| 查看 Token | 菜单栏点击图标 →「访问本机 Token」，或 `cat ~/.linkterm/config.yaml` |
| Token 泄露 | 删除 config.yaml 中 token 字段后重启 Agent，旧 Token 立即失效 |
| 无法连接服务端 | `curl https://your-server.com/health/ping` 验证服务端，检查防火墙 |
| 手机锁屏回来卡住 | 正常现象，1-3 秒自动重连，超过 30 秒手动刷新 |
| 合盖断连 | 确认 `prevent_sleep: true`，**必须接电源** |
| 菜单栏无图标 | 确认未加 `--no-tray` 参数，编译需 `CGO_ENABLED=1` |

---

## 项目结构

```
LinkTerm/
├── proto/                      # 共享协议定义
├── server/                     # 服务端
│   ├── web/                    # 前端 (嵌入到二进制)
│   └── ...
├── agent/                      # Mac Agent
│   ├── main.go                 # 入口（菜单栏模式 / --no-tray 模式）
│   ├── tray.go                 # 菜单栏图标和操作面板
│   ├── tunnel.go               # WebSocket 隧道
│   ├── selector.go             # 智能选路
│   ├── session.go              # PTY 会话 + RingBuffer
│   └── sleep.go                # 防睡眠
├── scripts/install.sh          # 一键安装脚本
├── deploy/                     # 服务端部署配置
├── docker-compose.yml
└── docs/                       # 文档
```

---

## 技术栈

| 组件 | 依赖 |
|:---|:---|
| 后端 | Go 1.21, nhooyr.io/websocket, golang-jwt/jwt |
| 终端 | github.com/creack/pty |
| 菜单栏 | fyne.io/systray |
| 前端 | xterm.js 5.5, 原生 JavaScript |
| 部署 | Docker, Docker Compose |

## License

MIT
