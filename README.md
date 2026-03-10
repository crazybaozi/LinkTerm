# LinkTerm

**手机浏览器远程操作 Mac 终端，打开即用，无需安装任何 App。**

```
手机/iPad (浏览器)  ──HTTPS/WSS──>  服务端 (云服务器)  <──WSS──  Mac Agent
```

外出时服务器告警？通勤时想跑个脚本？躺在床上想改个配置？打开手机浏览器，直接进入 Mac 终端。

---

## 特性

- **零安装** — 手机端纯浏览器，不需要装任何 App
- **PWA 支持** — 添加到主屏幕后全屏使用，体验接近原生 App
- **会话保持** — 手机锁屏再回来终端还在，断线自动重连 + 缓冲回放
- **智能选路** — 配置多个服务端节点，自动测速选择延迟最低的连接
- **防睡眠** — Mac 合盖接电源不断连
- **多用户** — 多台 Mac 各自注册，Token 即身份，无需登录注册
- **安全** — Token 认证 + JWT + 暴力破解防护 + 全链路 TLS
- **多会话** — 同时开多个终端，菜单一键切换
- **移动优化** — 虚拟辅助键盘（Tab/Ctrl/Esc/方向键）、字体大小调节、粘贴按钮
- **主题切换** — Tokyo Night / One Light / Dracula 三款主题
- **一键部署** — Docker Compose 启动服务端，Mac Agent 启动即用

## 截图

```
  ┌───────────────────────────────────┐
  │ ● 已连接                        ≡ │  ← 状态栏
  ├───────────────────────────────────┤
  │                                   │
  │  ~ $ echo "Hello from iPhone"     │  ← xterm.js 终端
  │  Hello from iPhone                │
  │  ~ $ _                            │
  │                                   │
  ├───────────────────────────────────┤
  │ Tab Ctrl Esc ↑ ↓ ← → | ~ A- A+  │  ← 虚拟键盘
  └───────────────────────────────────┘
```

## 架构

```
┌──────────────┐         ┌──────────────────┐         ┌──────────────┐
│  手机/iPad    │  HTTPS  │    LinkTerm      │   WSS   │   Mac 电脑    │
│  浏览器/PWA   │ ──────> │    Server        │ <────── │   Agent      │
│              │   WSS   │   (Docker)       │         │              │
└──────────────┘         └──────────────────┘         └──────────────┘
                                                            ↕
                                                          PTY (zsh)
```

| 组件 | 技术栈 | 说明 |
|------|--------|------|
| Server | Go + nhooyr.io/websocket | 转发终端数据，管理认证和会话 |
| Agent | Go + creack/pty | 运行在 Mac，创建 PTY，通过 WebSocket 连接服务端 |
| Web | xterm.js + 原生 JS | 终端 UI，PWA 支持，无框架依赖 |

---

## 快速开始

### 前置条件

- 一台云服务器（1C1G 即可）+ Docker
- Mac 电脑（macOS 12+）
- Go 1.21+（用于编译）

### 1. 部署服务端

```bash
git clone https://github.com/yourname/linkterm.git
cd linkterm

# 编辑配置（只需修改 jwt_secret）
vi deploy/config.yaml
```

**`deploy/config.yaml` 关键配置：**

```yaml
auth:
  jwt_secret: "改为随机字符串-至少32位"
  data_dir: "./data"    # Agent 注册信息存储目录
```

> 服务端不需要预配置任何 Agent Token，Agent 首次连接时自动注册。

```bash
# 启动
docker-compose up -d

# 查看日志
docker-compose logs -f
```

### 2. 安装 Mac Agent

```bash
# 编译
cd agent
GOOS=darwin GOARCH=arm64 go build -o linkterm-agent .
# Intel Mac: GOARCH=amd64

# 创建配置
mkdir -p ~/.linkterm
cat > ~/.linkterm/config.yaml << EOF
servers:
  - url: "wss://你的服务器域名"
    name: "主节点"

# token 留空，首次启动自动生成
token: ""
name: ""
prevent_sleep: true
EOF

# 运行
./linkterm-agent -config ~/.linkterm/config.yaml
```

Agent 首次启动时会**自动生成 Token** 并保存到配置文件，同时向服务端注册。启动后会显示：

```
  ┌───────────────────────────────────────┐
  │         LinkTerm Agent Running         │
  ├───────────────────────────────────────┤
  │  Server : 主节点                       │
  │  URL    : https://你的服务器域名        │
  │  Token  : lt_a1b2c3d4e5f6...          │
  ├───────────────────────────────────────┤
  │  手机浏览器打开上面的 URL              │
  │  输入 Token 即可访问                   │
  │  Press Ctrl+C to stop                 │
  └───────────────────────────────────────┘
```

### 3. 手机访问

1. 手机浏览器打开 `https://你的服务器域名`
2. 输入 Agent Token（在 Mac Agent 启动时显示，或在 `~/.linkterm/config.yaml` 中查看）
3. 进入终端，开始操作

**推荐添加到主屏幕：**
- iOS: Safari → 分享 → 添加到主屏幕
- Android: Chrome → 菜单 → 安装应用

---

## 认证机制

LinkTerm 采用 **Token 即身份** 的极简认证模型，无需登录注册：

```
Agent 认证：Agent 启动 → 自动生成 Token → 连接服务端 → 自动注册
浏览器认证：输入 Agent Token → 服务端校验 → 签发 JWT → 访问终端
```

| 概念 | 说明 |
|------|------|
| Agent Token | 每台 Mac 的唯一标识，首次启动自动生成，存储在 `~/.linkterm/config.yaml` |
| JWT | 浏览器登录后签发，24 小时有效，包含 agent_id |
| agents.json | 服务端自动维护的已注册 Agent 列表，存储在 `data_dir` 下 |

**多台 Mac**：每台 Mac 有独立的 Token，通过不同 Token 访问不同的 Mac。

**共享访问**：将 Token 分享给他人即可共享终端访问权限，多人可同时操作同一终端。

**Token 重新生成**：Agent 可随时重新生成 Token，旧 Token 立即失效。

---

## 配置说明

### 服务端配置（`deploy/config.yaml`）

```yaml
listen: ":8080"
region: "default"

tls:
  # cert: /etc/linkterm/certs/fullchain.pem
  # key: /etc/linkterm/certs/privkey.pem

auth:
  jwt_secret: "改为随机字符串"
  data_dir: "./data"       # agents.json 存放目录

session:
  max_per_agent: 10        # 每个 Agent 最大会话数
  buffer_size: 65536       # 服务端缓冲区大小 (64KB)

heartbeat:
  interval: 15s
  timeout: 45s
```

### Agent 配置（`~/.linkterm/config.yaml`）

```yaml
servers:                        # 支持多节点，自动测速选最优
  - url: "wss://node1.example.com"
    name: "节点1"
  - url: "wss://node2.example.com"
    name: "节点2"

token: ""                       # 留空则自动生成
name: ""                        # 留空则取系统 hostname
# shell: /bin/zsh              # 默认自动检测
prevent_sleep: true             # 防止 Mac 空闲睡眠
reconnect_max_interval: 30      # 最大重连间隔(秒)
local_buffer_size: 131072       # 本地缓冲 128KB
max_sessions: 10
```

---

## TLS 配置

**方式一：Nginx 反向代理（推荐）**

```nginx
server {
    listen 443 ssl;
    server_name term.example.com;

    ssl_certificate /etc/letsencrypt/live/term.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/term.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
    }
}
```

**方式二：直接配置 TLS**

将证书放在 `deploy/certs/`，取消 `deploy/config.yaml` 中 TLS 注释即可。

---

## 开机自启（macOS LaunchAgent）

```bash
bash deploy/install-agent.sh
```

脚本会将 Agent 注册为 macOS 登录项，开机自动运行。

- 启动：`launchctl load ~/Library/LaunchAgents/com.linkterm.agent.plist`
- 停止：`launchctl unload ~/Library/LaunchAgents/com.linkterm.agent.plist`
- 日志：`~/.linkterm/agent.log`

---

## 智能选路

当配置多个服务端节点时，Agent 会：

1. **启动时**并发测速所有节点（`/health/ping`，每节点 3 次取中位数）
2. **自动连接**延迟最低的节点
3. **每 5 分钟**后台静默测速
4. **连续 3 个周期**发现更优节点（延迟低于当前 50%）时自动切换
5. 切换采用**先连后断**策略，PTY 进程不受影响

```
[selector] measuring latency to all nodes...
[selector]   上海: 12ms
[selector]   北京: 35ms
[selector]   广州: 28ms
[selector] selected: 上海 (12ms)
```

---

## 项目结构

```
LinkTerm/
├── proto/                      # 共享协议定义
│   └── message.go
├── server/                     # 服务端
│   ├── Dockerfile
│   ├── main.go                 # 入口
│   ├── config.go               # 配置
│   ├── auth.go                 # 认证 (agents.json + JWT + 暴力破解防护)
│   ├── hub.go                  # Agent 连接管理
│   ├── session.go              # 会话管理 + RingBuffer
│   ├── handler_agent.go        # Agent WebSocket 处理（含自动注册）
│   ├── handler_terminal.go     # 浏览器 WebSocket 处理
│   ├── handler_web.go          # HTTP API + 静态资源
│   └── web/                    # 前端 (嵌入到二进制)
│       ├── index.html          # 登录页（输入 Agent Token）
│       ├── terminal.html       # 终端页
│       ├── css/style.css
│       ├── js/auth.js
│       ├── js/terminal.js      # xterm.js 集成
│       ├── icons/icon.svg      # PWA 图标
│       ├── manifest.json       # PWA manifest
│       └── sw.js               # Service Worker
├── agent/                      # Mac Agent
│   ├── main.go                 # 入口
│   ├── config.go               # 配置（Token 自动生成 + 回写）
│   ├── tunnel.go               # WebSocket 隧道（含 Token 重新生成）
│   ├── selector.go             # 智能选路
│   ├── session.go              # PTY 会话 + RingBuffer
│   ├── sleep.go                # 防睡眠 (caffeinate)
│   └── qrcode.go               # 启动信息展示
├── deploy/                     # 部署文件
│   ├── config.yaml             # 服务端配置模板
│   ├── agent-config.yaml       # Agent 配置模板
│   └── install-agent.sh        # macOS 安装脚本
├── docker-compose.yml
└── docs/
    ├── PRD.md                  # 产品需求文档
    ├── 多用户模式改造说明.md    # 多用户模式说明
    └── 部署指南.md              # 详细部署文档
```

---

## 常见问题

### 连接后提示 "Mac 离线"
- 检查 Mac Agent 是否在运行
- 检查 Agent 的 server URL 是否正确
- 服务端日志中确认 Agent 已注册成功

### 如何查看我的 Agent Token
- 查看 Agent 配置文件：`cat ~/.linkterm/config.yaml` 中的 `token` 字段
- Agent 启动时也会在终端打印 Token

### Token 被泄露了怎么办
- 在 Agent 端重新生成 Token（调用 RegenerateToken 或删除配置中的 token 字段后重启 Agent）
- 旧 Token 立即失效，持有旧 Token 的浏览器用户需使用新 Token 重新登录

### Agent 无法连接服务端
- 检查服务端是否启动：`curl https://your-server.com/health/ping`
- 检查防火墙是否开放端口
- TLS 模式下确认证书有效

### 手机锁屏回来终端卡住
- 正常现象，页面恢复后会自动重连（1-3 秒）
- 超过 30 秒可手动刷新

### Mac 合盖后断连
- 确认 `prevent_sleep: true` 已配置
- **必须接电源**，纯电池合盖 macOS 会强制睡眠
- 建议在「系统设置 → 电池 → 选项」中开启「连接到电源适配器时防止自动休眠」

### 如何同时操作多个终端
- 点击右上角 `≡` → 新建终端
- 在会话列表中切换

---

## 技术栈

| 组件 | 依赖 |
|------|------|
| 后端 | Go 1.21, nhooyr.io/websocket, golang-jwt/jwt, gopkg.in/yaml.v3 |
| 终端 | github.com/creack/pty |
| 前端 | xterm.js 5.5, 原生 JavaScript（无框架） |
| 部署 | Docker, Docker Compose |

---

## License

MIT
