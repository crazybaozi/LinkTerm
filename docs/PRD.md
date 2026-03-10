# LinkTerm 产品需求文档（PRD）

## 1. 产品概述

### 1.1 一句话描述

LinkTerm 让你随时从手机浏览器远程操作 Mac 终端，打开即用，无需安装任何 App。

### 1.2 目标用户

- 需要外出时紧急处理服务器问题的开发者
- 想利用碎片时间跑脚本、查看日志的运维人员
- 习惯命令行工作流，不想随身携带笔记本的极客

### 1.3 核心价值

| 场景 | 痛点 | LinkTerm 解决 |
|:---|:---|:---|
| 外出时服务器告警 | 电脑不在身边，手机无法 SSH 到内网机器 | 手机打开网页 → 直连家里 Mac → 跳到目标服务器 |
| 通勤时想跑个脚本 | 笔记本太重，手机没有完整终端环境 | 手机点开 LinkTerm 图标，直接进入 Mac 开发环境 |
| 躺在床上改配置 | 不想起身去书房开电脑 | iPad 打开浏览器直接操作 |

### 1.4 竞品对比

| 产品 | 方式 | LinkTerm 差异 |
|:---|:---|:---|
| **Tailscale** | P2P 组网 | 需要两端装客户端，配置复杂 |
| **Cloudflare Tunnel** | 反向代理 | 需域名，TCP 支持弱，无固定端口 |
| **ngrok** | 公共隧道 | 免费版随机域名/端口，不稳定 |
| **自建 FRP** | 手动配置 | 门槛高，需理解 FRP 配置 |
| **SSH App（Termius 等）** | 直连 SSH | 需要公网 IP 或 VPN，手机端需安装 App |

**LinkTerm 定位**：手机零安装，Mac 一键配置，打开浏览器就是终端。

---

## 2. 系统架构

### 2.1 架构图

```
                                       ┌──────────────────┐
                                       │  服务端 A (北京)   │
                                  ┌──→ │  term-bj.xxx.com  │ ←─┐
                                  │    └──────────────────┘   │
┌──────────────┐    HTTPS/WSS     │    ┌──────────────────┐   │  WSS       ┌──────────────┐
│  手机/iPad    │ ───────────────→├──→ │  服务端 B (上海)   │ ←─┤←────────  │   Mac 电脑    │
│  (浏览器/PWA) │   连接最快的节点  │    │  term-sh.xxx.com  │   │  Agent    │  (linkterm    │
└──────────────┘                  │    └──────────────────┘   │  自动选择   │   agent)     │
                                  │    ┌──────────────────┐   │  最优节点   │              │
                                  └──→ │  服务端 C (广州)   │ ←─┘           └──────────────┘
                                       │  term-gz.xxx.com  │
                                       └──────────────────┘

Agent 启动时自动测速所有服务端节点，选择延迟最低的连接。
浏览器通过 Agent 选中的服务端访问终端。
```

### 2.2 数据流（一次终端操作）

```
用户在手机敲键盘
  → 浏览器 xterm.js 捕获按键
  → WebSocket 发送到服务端
  → 服务端通过 Agent 隧道转发到 Mac
  → Mac Agent 写入 PTY stdin
  → Shell 执行命令，输出到 PTY stdout
  → Mac Agent 通过 WebSocket 发送到服务端
  → 服务端转发到浏览器 WebSocket
  → xterm.js 渲染输出
```

### 2.3 技术选型

| 组件 | 技术 | 说明 |
|:---|:---|:---|
| Mac Agent | Go | 交叉编译方便、单二进制分发、PTY 库成熟 |
| 服务端 | Go | 与 Agent 共享协议代码、高并发、单二进制部署 |
| Web 前端 | xterm.js + 原生 JS + PWA | 终端渲染的事实标准，无需框架，保持轻量 |
| WebSocket | `nhooyr.io/websocket` | 比 gorilla 更现代，支持 context、压缩 |
| PTY | `github.com/creack/pty` | Go 生态最成熟的 PTY 库 |
| 菜单栏 | `github.com/energye/systray` | Go 系统托盘，macOS/Linux/Windows 都支持 |
| HTTP 框架 | `net/http`（标准库） | MVP 够用，不引入额外依赖 |
| 部署 | Docker + docker-compose | 服务端一键部署 |

### 2.4 依赖清单

**Agent (`agent/go.mod`)**

| 包 | 版本 | 用途 |
|:---|:---|:---|
| `nhooyr.io/websocket` | v1.8.11 | WebSocket 客户端 |
| `github.com/creack/pty` | v1.1.21 | PTY 管理 |
| `github.com/energye/systray` | v1.0.2 | 菜单栏图标 |
| `gopkg.in/yaml.v3` | v3.0.1 | 配置文件解析 |
| `github.com/skip2/go-qrcode` | latest | 二维码生成 |

**Server (`server/go.mod`)**

| 包 | 版本 | 用途 |
|:---|:---|:---|
| `nhooyr.io/websocket` | v1.8.11 | WebSocket 服务端 |
| `github.com/golang-jwt/jwt/v5` | v5.2.1 | JWT 认证 |
| `gopkg.in/yaml.v3` | v3.0.1 | 配置文件解析 |

**Web 前端（CDN 或本地打包）**

| 包 | 版本 | 用途 |
|:---|:---|:---|
| `xterm` | v5.5.0 | 终端渲染 |
| `@xterm/addon-fit` | v0.10.0 | 终端自适应尺寸 |
| `@xterm/addon-web-links` | v0.11.0 | 可点击链接 |

---

## 3. 功能需求

### 3.1 Mac Agent

| 优先级 | 功能 | 描述 |
|:---|:---|:---|
| P0 | 一键连接 | 配置服务端地址和 Token，自动建立 WebSocket 隧道 |
| P0 | PTY 管理 | 接收服务端指令创建/销毁本地 shell 会话 |
| P0 | 自动重连 | 网络波动/Mac 唤醒后自动恢复，指数退避（0.5s→1s→2s→5s→10s→30s） |
| P0 | 心跳保活 | 每 15 秒发送 ping，30 秒无 pong 判定断线并触发重连 |
| P0 | 菜单栏常驻 | 图标显示连接状态（🟢已连接/🔴未连接/🟡重连中），点击展开操作面板 |
| P0 | 本地缓冲 | 128KB 环形缓冲区，WebSocket 断开期间缓存 PTY 输出，重连后发送 |
| P0 | 连接信息展示 | 显示访问地址、二维码，支持一键复制 |
| P1 | 防睡眠 | IOPMAssertion 阻止系统空闲睡眠（接电源时合盖不断连） |
| P1 | 电源状态检测 | 检测是否接电源、是否合盖，给出针对性提示 |
| P1 | 睡眠/唤醒监听 | 监听 NSWorkspace 通知，唤醒后立即触发重连（不等心跳超时） |
| P1 | 开机自启 | macOS LaunchAgent，可选 |
| P1 | 首次引导 | Setup Wizard：检测 SSH 服务 → 配置连接 → 展示访问信息 |
| P1 | 智能选路 | 配置多个服务端节点，启动时自动测速选择最优，运行期间持续监测并自动切换 |

#### 菜单栏面板

```
┌─────────────────────────────────┐
│  ✅ 已连接                       │
│  服务器: 上海 (12ms)              │
│  在线时长: 3天 12小时             │
│                                 │
│  访问地址:                       │
│  https://term-sh.example.com/t/ │
│  [ 复制 ]  [ 二维码 ]            │
│                                 │
│  活跃终端: 2 个                   │
│  ─────────────────────────────  │
│  ☑ 始终在线                      │
│  ☑ 开机自动连接                   │
│  ─────────────────────────────  │
│  节点状态:                       │
│    上海 12ms ●                   │
│    北京 35ms ○                   │
│    广州 28ms ○                   │
│  ─────────────────────────────  │
│  设置...                         │
│  断开连接                        │
│  退出 LinkTerm                   │
└─────────────────────────────────┘
```

#### 首次引导流程

```
第 1 步：检测 SSH 服务
  ├─ 已开启 → ✅ 跳过
  └─ 未开启 → 引导用户打开「系统设置 → 通用 → 共享 → 远程登录」
     提供 [打开系统设置] 按钮，实时检测状态

第 2 步：配置连接
  ├─ 输入服务端地址（wss://term.example.com）
  └─ 输入 Token

第 3 步：连接成功
  ├─ 显示访问地址 + 二维码
  ├─ 显示访问密码
  └─ 引导用户手机扫码体验
```

### 3.2 服务端

| 优先级 | 功能 | 描述 |
|:---|:---|:---|
| P0 | Agent 接入 | 接收 Agent WebSocket 连接，Token 认证 |
| P0 | 会话路由 | 将浏览器终端数据路由到对应 Agent 的 PTY |
| P0 | Web Terminal | 提供 xterm.js 终端页面和静态资源 |
| P0 | 浏览器认证 | 访问密码 → JWT Token，24 小时有效 |
| P0 | 输出缓冲 | 每个会话 64KB 环形缓冲，浏览器重连时回放 |
| P0 | 心跳检测 | 45 秒无 Agent 心跳判定离线，30 秒无浏览器心跳设为 Detached |
| P1 | 会话保持 | 浏览器断开不关闭 PTY，重连后自动恢复 |
| P1 | Agent 重连恢复 | Agent 重连时上报存活会话，服务端重建路由映射 |
| P1 | 暴力破解防护 | 密码错误 5 次锁定 15 分钟 |
| P2 | 连接统计 | 记录连接时长、在线状态 |
| P2 | 多 Agent 管理 | 支持多个 Mac 同时接入 |

### 3.3 Web 前端

| 优先级 | 功能 | 描述 |
|:---|:---|:---|
| P0 | 登录页 | 输入访问密码，获取 JWT |
| P0 | 终端页 | xterm.js 渲染，WebSocket 双向通信 |
| P0 | 移动端适配 | 响应式布局，触摸友好 |
| P0 | 辅助键工具栏 | Tab、Ctrl、Esc、方向键等虚拟按键 |
| P0 | 自动重连 | visibilitychange 监听，页面恢复可见时自动重连 |
| P0 | 状态提示条 | 顶部展示连接状态（已连接/重连中/Mac离线），给出操作建议 |
| P0 | PWA 支持 | manifest.json + Service Worker，添加到主屏幕后如原生 App |
| P1 | 会话列表 | 显示所有活跃终端，点击切换 |
| P1 | 快速入口 | 仅一个终端时跳过列表直接进入 |
| P1 | 缓冲回放提示 | 重连后显示"已回放离线期间的 N KB 输出" |
| P2 | 终端主题 | 暗色/亮色切换 |
| P2 | 多终端标签 | 同时开多个 shell |

#### 移动端终端界面

```
┌─────────────────────────────┐
│ 🟢 MacBook Pro  在线  ≡      │  ← 顶栏：状态 + 菜单
│─────────────────────────────│
│                             │
│  $ ls -la                   │
│  total 48                   │  ← xterm.js 终端区域
│  drwxr-xr-x  12 user       │
│  $ _                        │
│                             │
│─────────────────────────────│
│ Tab Ctrl Esc  ↑  ↓  ← → ~  │  ← 辅助键工具栏
└─────────────────────────────┘
```

#### 状态提示条

| 状态 | 显示内容 |
|:---|:---|
| 正常连接 | 🟢 已连接 MacBook Pro 在线 2h 15m |
| 浏览器重连中 | 🟡 重连中... 网络波动，正在恢复连接 |
| Mac 离线 | 🔴 Mac 离线 可能已进入睡眠，请唤醒你的 Mac |
| 恢复后 | 🟢 已恢复 已回放离线期间的 2.3KB 输出（3 秒后消失） |
| 节点切换中 | 🟡 正在切换到更快的节点... |
| 节点切换完成 | 🟢 已切换到上海节点 (12ms)（3 秒后消失） |
| 会话已结束 | ⚫ 终端已结束 [新建终端] |

---

## 4. 协议设计

### 4.1 Agent ↔ Server WebSocket 协议

连接地址：`wss://server.com/ws/agent?token=xxx`

消息格式：

```json
{
  "type": "message_type",
  "id": "unique_message_id",
  "payload": { }
}
```

#### 消息类型

| 方向 | Type | Payload | 说明 |
|:---|:---|:---|:---|
| Agent → Server | `auth` | `{token, sessions: [{session_id, created_at, shell}]}` | 认证，附带存活会话列表（支持重连恢复） |
| Server → Agent | `auth_result` | `{ok, agent_id, error}` | 认证结果 |
| 双向 | `ping` | `{ts}` | 心跳 |
| 双向 | `pong` | `{ts}` | 心跳响应 |
| Server → Agent | `session_open` | `{session_id, cols, rows}` | 请求创建 PTY |
| Agent → Server | `session_opened` | `{session_id}` | PTY 创建成功 |
| Agent → Server | `session_open_error` | `{session_id, error}` | PTY 创建失败 |
| Server → Agent | `session_input` | `{session_id, data}` | 终端输入（data 为 base64） |
| Agent → Server | `session_output` | `{session_id, data}` | 终端输出（data 为 base64） |
| Server → Agent | `session_resize` | `{session_id, cols, rows}` | 终端窗口大小变化 |
| Server → Agent | `session_close` | `{session_id}` | 关闭终端 |
| Agent → Server | `session_closed` | `{session_id, exit_code}` | 终端已关闭（shell 退出） |
| Agent → Server | `session_resume` | `{session_id, buffered_output}` | Agent 重连后恢复会话，附带本地缓冲 |
| Server → Agent | `session_resume_ok` | `{session_id}` | 服务端确认会话恢复 |

### 4.2 Browser ↔ Server HTTP API

| 方法 | 路径 | 说明 |
|:---|:---|:---|
| `GET` | `/health/ping` | 测速接口（返回 `{ok, ts, region}`，Agent 智能选路用） |
| `POST` | `/api/auth` | 登录（访问密码 → JWT） |
| `GET` | `/api/agents` | 获取在线 Agent 列表 |
| `POST` | `/api/sessions` | 创建终端会话 |
| `GET` | `/api/sessions` | 获取活跃会话列表 |
| `DELETE` | `/api/sessions/:id` | 关闭终端会话 |

### 4.3 Browser ↔ Server 终端 WebSocket

连接地址：`wss://server.com/ws/terminal/:session_id?token=jwt_xxx`

| 方向 | 帧类型 | 内容 | 说明 |
|:---|:---|:---|:---|
| Browser → Server | 二进制帧 | 原始按键字节 | 终端输入 |
| Server → Browser | 二进制帧 | PTY 输出字节 | 终端输出（直接喂给 xterm.js） |
| Browser → Server | 文本帧 | `{type:"resize", cols, rows}` | 窗口大小变化 |
| Browser → Server | 文本帧 | `{type:"ping", ts}` | 客户端心跳 |
| Server → Browser | 文本帧 | `{type:"pong", ts}` | 心跳响应 |
| Server → Browser | 文本帧 | `{type:"session_status", status}` | 会话状态变化通知 |
| Server → Browser | 文本帧 | `{type:"buffered", size}` | 即将回放缓冲 |
| Server → Browser | 文本帧 | `{type:"closed", exit_code}` | 会话已关闭 |
| Server → Browser | 文本帧 | `{type:"server_switch", new_url, reason}` | Agent 已切换到更优节点，浏览器需重连新地址 |

### 4.4 会话建立时序

```
Agent                          Server                         Browser
  │                               │                               │
  │──── WSS 连接 ────────────────→│                               │
  │──── auth {token} ───────────→│                               │
  │←─── auth_result {ok} ────────│                               │
  │←──── ping ───────────────────│                               │
  │───── pong ──────────────────→│                               │
  │           (保持心跳)           │                               │
  │                               │←─── POST /api/auth ──────────│
  │                               │───→ {jwt_token} ─────────────│
  │                               │                               │
  │                               │←── POST /api/sessions ───────│
  │                               │    {agent_id, cols, rows}     │
  │←── session_open ─────────────│                               │
  │    {session_id, cols, rows}   │                               │
  │                               │                               │
  │  [Agent spawns PTY /bin/zsh]  │                               │
  │                               │                               │
  │──── session_opened ─────────→│                               │
  │                               │──→ {session_id} ─────────────│
  │                               │                               │
  │                               │←── WSS /ws/terminal/:id ─────│
  │                               │                               │
  │                               │←── binary(keystroke) ─────── │
  │←── session_input ────────────│                               │
  │                               │                               │
  │  [PTY processes input]        │                               │
  │                               │                               │
  │──── session_output ─────────→│                               │
  │                               │──→ binary(output) ───────────│
  │                               │                   [xterm 渲染] │
```

---

## 5. 会话管理

### 5.1 核心原则

**会话 = 永驻，像微信聊天窗口。** 用户不主动关闭，会话就永远在。

类比微信：

| 微信 | LinkTerm |
|:---|:---|
| 打开微信 → 聊天记录都在 | 打开浏览器 → 终端还在，光标还在原位 |
| 锁屏再打开 → 一切如常 | 锁屏再打开 → 命令还在跑，输出一直在 |
| 杀掉后台 → 重新打开 → 消息还在 | 关掉浏览器 → 重新打开 → 终端还在 |
| 你不会"关闭"一个聊天 | 你不需要"关闭"一个终端 |

### 5.2 会话状态机

```
                    创建
                     │
                     ▼
              ┌──────────┐
              │  Active   │ ← 浏览器已连接，实时交互
              └────┬─────┘
                   │ 浏览器断开（锁屏/切App/网络断）
                   ▼
              ┌──────────┐
              │ Detached  │ ← PTY 仍在运行，输出写入缓冲
              └────┬─────┘
                   │
          ┌────────┼────────┐
          │        │        │
          ▼        ▼        ▼
     浏览器重连  Agent断开  用户关闭/shell退出
     → Active  → Orphan   → Closed
                   │
                   │ Agent 重连
                   ▼
               Detached
               (等待浏览器)
```

| 状态 | 含义 | PTY | Agent 缓冲 | 服务端缓冲 |
|:---|:---|:---|:---|:---|
| **Active** | 三方全通，实时交互 | 运行中 | 持续写入 | 持续写入 |
| **Detached** | 浏览器断开，其余正常 | 运行中 | 持续写入 | 持续接收 Agent 输出并缓冲 |
| **Orphan** | Agent 断开（Mac 休眠） | 冻结中 | 暂停 | 保留缓冲，等待 Agent 重连 |
| **Closed** | 会话结束 | 已终止 | 已清理 | 已清理 |

### 5.3 会话销毁条件

**仅以下情况才会销毁会话**：

1. 用户在浏览器点击"关闭终端"（需二次确认）
2. Shell 进程自然退出（用户输入 `exit` 或 `logout`）
3. Mac Agent 被卸载/停止
4. 资源保护兜底（见下方）

**不设置空闲超时。** 终端可以挂在那里一周不动，不会被清理。

### 5.4 资源保护（兜底机制）

| 条件 | 动作 | 默认阈值 |
|:---|:---|:---|
| 单个 Agent 终端数超限 | 拒绝创建新终端，提示"请关闭不用的终端" | 10 个 |
| Agent 离线超过 N 天 | 清理服务端会话元信息 | 7 天 |
| Shell 进程已退出 | 浏览器显示"终端已结束"，引导关闭或新建 | 即时 |

### 5.5 两级缓冲架构

```
PTY 输出
  │
  ▼
Agent 本地缓冲 (RingBuffer 128KB)
  │
  ├──→ WebSocket 正常 → 实时发送到服务端 + 写入本地缓冲
  │
  └──→ WebSocket 断开 → 仅写入本地缓冲（重连后发送）
        │
        ▼
服务端缓冲 (RingBuffer 64KB per session)
  │
  ├──→ 浏览器 WS 正常 → 实时转发 + 写入缓冲
  │
  └──→ 浏览器 WS 断开 → 仅写入缓冲（重连后回放）
```

- **Agent 本地缓冲**：解决 Mac 唤醒后到 WebSocket 重连成功之间的输出丢失
- **服务端缓冲**：解决浏览器锁屏到重连之间的输出丢失

两个缓冲独立运作，环形结构固定大小，不增长。

---

## 6. 连接稳定性

### 6.1 断连场景与应对

| 场景 | 原因 | 技术方案 | 用户感知 |
|:---|:---|:---|:---|
| 手机锁屏/切 App | iOS Safari 秒杀后台 WS | visibilitychange 监听 + 自动重连 + 缓冲回放 | 闪一下"重连中"，0.5 秒恢复 |
| 手机进电梯/弱网 | 网络中断 | 客户端心跳检测（10s）+ 自动重连 | 2-3 秒恢复 |
| Mac 合盖（接电源） | 系统空闲睡眠 | IOPMAssertion 阻止睡眠 | 不断连 |
| Mac 合盖（电池） | 系统强制睡眠，无法阻止 | 唤醒后 Agent 监听 didWake 立即重连 | Mac 唤醒后 1-2 秒恢复 |
| Mac WiFi 切有线 | 网络切换 | Agent 心跳超时 + 自动重连 | 2-3 秒恢复 |
| 用户手动睡眠 | 点击"苹果 → 睡眠" | 同合盖电池场景 | Mac 唤醒后 1-2 秒恢复 |
| 服务端重启 | 运维操作 | Agent 自动重连 + 上报存活会话 + 服务端重建映射 | Agent 自动恢复，浏览器刷新后继续 |
| 关闭浏览器后重开 | 用户主动关闭 | 登录后进入会话列表，点击存活的终端恢复 | 登录 → 点击 → 继续 |

### 6.2 Agent 重连策略

```
断线检测（心跳超时 或 系统唤醒事件）
  │
  ▼
快速重连当前节点（1 次尝试，0.5s 超时）
  │
  ├─ 成功 → 恢复会话
  │
  └─ 失败 → 重新测速所有节点 → 选择最优节点
              │
              ▼
         指数退避重连：0.5s → 1s → 2s → 5s → 10s → 30s（最大间隔）
              │
              ▼
         重连成功
              │
              ├─ 发送 auth 消息（包含存活会话列表）
              ├─ 发送每个存活会话的 session_resume（附带本地缓冲输出）
              └─ 恢复正常心跳
```

### 6.3 浏览器重连策略

```javascript
// Page Visibility API 监听
document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') {
        // 页面恢复可见，检查 WebSocket 状态
        if (ws.readyState !== WebSocket.OPEN) {
            reconnect();
        } else if (Date.now() - lastHiddenTime > 30000) {
            // 超过 30 秒，验证连接是否真的存活
            sendPing(); // 3 秒无 pong 则重连
        }
    }
});

// 客户端心跳：每 10 秒，20 秒无 pong 则重连
```

### 6.4 Mac 休眠处理

#### 可阻止的场景

使用 IOPMAssertion 阻止系统空闲睡眠（需用户在 Agent 菜单栏开启"始终在线"）：

| 场景 | 能否阻止 |
|:---|:---|
| 用户长时间不操作 → 自动睡眠 | ✅ 能阻止 |
| 合盖（接电源） | ✅ 能阻止 |
| 合盖（电池） | ❌ 不能（macOS 强制省电） |
| 用户手动点"睡眠" | ❌ 不能 |
| 电池电量极低 | ❌ 不能 |

#### 不可阻止的场景——用户提示

Agent 菜单栏"始终在线"设置面板：

```
┌──────────────────────────────────────┐
│  ⚡ 连接保持                          │
│                                      │
│  始终在线模式   [━━━●] 已开启          │
│                                      │
│  ℹ️ 开启后，Mac 接通电源时合盖不会     │
│    断开连接。使用电池时合盖仍会         │
│    断开，唤醒后会自动恢复。            │
│                                      │
│  ─────────────────────────────────   │
│                                      │
│  💡 如需始终可访问：                   │
│  1. 保持电源连接                      │
│  2. 系统设置 → 锁定屏幕               │
│     → 显示器关闭时间：设为「永不」      │
│     [打开锁定屏幕设置]                 │
│                                      │
│  当前状态: ✅ 已接电源 ✅ 睡眠已阻止    │
│                                      │
└──────────────────────────────────────┘
```

Agent 主动检测电源状态并提示：

| 检测条件 | 提示 |
|:---|:---|
| 电池供电 + 始终在线已开启 | "⚠️ 电池供电，合盖后连接将中断。建议接通电源。" |
| 未开启始终在线 | "💡 开启"始终在线"可在合盖时保持连接（需接电源）" |

### 6.5 智能选路（自动选择最优服务器）

#### 设计目标

Agent 配置多个服务端节点（2-5 个），启动时自动测速并连接延迟最低的节点，运行期间持续监测，节点异常时自动切换。

#### 测速机制

Agent 通过 HTTP 请求服务端的 `/health/ping` 接口测量 RTT（往返延迟）：

```
Agent                              Server A (北京)
  │── GET /health/ping ──────────→ │
  │←─ {"ts":1710000000,"ok":true} ─│   RTT = 35ms
  │                                │
Agent                              Server B (上海)
  │── GET /health/ping ──────────→ │
  │←─ {"ts":1710000000,"ok":true} ─│   RTT = 12ms  ← 最优
  │                                │
Agent                              Server C (广州)
  │── GET /health/ping ──────────→ │
  │←─ {"ts":1710000000,"ok":true} ─│   RTT = 28ms
```

每个节点连续 ping 3 次，取中位数作为该节点的延迟值，避免单次网络抖动的干扰。

#### 选路时机

| 时机 | 行为 |
|:---|:---|
| **Agent 首次启动** | 并发测速所有节点 → 连接最优节点 |
| **连接断开需要重连** | 先快速尝试重连当前节点（1 次）→ 失败则重新测速所有节点 → 连接最优 |
| **运行期间定期检测** | 每 5 分钟后台静默测速所有节点（不影响当前连接） |
| **当前节点延迟异常** | 定期检测发现当前节点延迟比最优节点高 2 倍以上，且持续 3 个周期 → 触发切换 |
| **Mac 唤醒后** | 重新测速所有节点 → 连接最优节点（网络环境可能已变化） |

#### 切换策略

```
定期检测结果：
  当前节点 (上海): 45ms
  候选节点 (北京): 15ms    ← 当前延迟 > 候选 × 2 ?

  第 1 个周期: 是 → 标记 (不切换，可能是临时波动)
  第 2 个周期: 是 → 标记
  第 3 个周期: 是 → 确认切换
                         │
                         ▼
                    执行切换：
                    1. 连接新节点
                    2. 认证 + 恢复会话
                    3. 确认成功后断开旧节点
                    4. 通知浏览器新的访问地址
```

**切换时先连新节点，确认成功后再断旧节点**，避免切换过程中出现双方都不可用的窗口。

#### 切换时的会话保持

服务器切换本质上是 Agent 断开旧服务端、连上新服务端。PTY 进程在 Mac 本地，不受影响：

```
切换前：                          切换后：
  手机 ←→ 服务端A ←→ Agent         手机 ←→ 服务端B ←→ Agent
                      ↕                              ↕
                     PTY                            PTY（同一个进程）
```

切换过程：
1. Agent 连接新服务端 B，发送 `auth`（包含存活会话列表）
2. 服务端 B 建立会话映射
3. Agent 发送 `session_resume` 恢复每个会话
4. Agent 断开旧服务端 A
5. 浏览器端收到通知，自动重连到新服务端 B（通过新地址）

浏览器需要感知切换：Agent 切换服务器后，浏览器的 WebSocket 地址也变了。服务端 A 在 Agent 断开后通知浏览器：

```json
{"type": "server_switch", "new_url": "wss://term-sh.xxx.com", "reason": "lower_latency"}
```

浏览器收到后自动重连到新地址，用户看到短暂的"切换到更快的节点..."提示。

#### `/health/ping` 接口设计

| 方法 | 路径 | 响应 |
|:---|:---|:---|
| `GET` | `/health/ping` | `{"ok":true, "ts":1710000000, "region":"shanghai"}` |

- `ts`：服务端时间戳（Agent 可用于校时）
- `region`：节点区域标识（用于展示）
- 响应体尽量小，确保测速准确

#### 用户感知

菜单栏面板展示当前连接的服务器和延迟：

```
┌─────────────────────────────────┐
│  ✅ 已连接                       │
│  服务器: 上海 (12ms)              │  ← 当前节点和延迟
│  在线时长: 3天 12小时             │
│  ...                             │
└─────────────────────────────────┘
```

节点切换时，菜单栏和浏览器都给出提示：
- 菜单栏：🟡 图标 + "正在切换到更快的节点..."
- 浏览器状态条："🟡 正在切换服务器，请稍候..."→ 切换完成 →"🟢 已切换到上海节点 (12ms)"

#### 异常处理

| 异常 | 处理 |
|:---|:---|
| 所有节点都不可达 | 按指数退避轮流尝试每个节点，菜单栏显示"所有服务器不可达" |
| 测速超时（某节点 > 3 秒无响应） | 标记该节点为不可用，跳过 |
| 切换新节点失败 | 回退到旧节点，不中断当前连接 |
| 所有节点延迟相近（差距 < 20%） | 不切换，保持当前连接稳定 |

---

## 7. 安全设计

### 7.1 认证链路

```
Agent 认证：
  Agent 启动 → WSS 连接服务端 → 发送 Token → 服务端验证 → 返回 agent_id
  Token：随机 32 字节 hex，服务端配置文件中定义

浏览器认证：
  打开页面 → 输入访问密码 → POST /api/auth → 服务端验证 → 返回 JWT
  JWT 有效期 24 小时，存 localStorage
  WebSocket 连接时通过参数携带 JWT
```

### 7.2 安全措施

| 层面 | 措施 |
|:---|:---|
| 传输加密 | 全链路 TLS（WSS + HTTPS） |
| Agent 认证 | Token 认证，泄露可重置 |
| 浏览器认证 | 访问密码 + JWT，密码可随时修改 |
| 暴力破解防护 | 密码错误 5 次 → 锁定 15 分钟 |
| 会话隔离 | 每个 Agent 的会话独立，互不可见 |

---

## 8. 配置文件

### 8.1 Agent 配置 (`~/.linkterm/config.yaml`)

```yaml
servers:                             # 多节点配置，Agent 自动选择最优
  - url: "wss://term-sh.example.com"
    name: "上海"
  - url: "wss://term-bj.example.com"
    name: "北京"
  - url: "wss://term-gz.example.com"
    name: "广州"
token: "a1b2c3d4e5f6..."
shell: ""                            # 默认 shell，空则读取 $SHELL
auto_connect: true                   # 启动后自动连接
prevent_sleep: true                  # 阻止系统空闲睡眠
reconnect_max_interval: 30           # 最大重连间隔（秒）
local_buffer_size: 131072            # 本地缓冲 128KB
max_sessions: 10                     # 最多同时开 10 个终端
selector:
  ping_count: 3                      # 每个节点 ping 次数
  ping_timeout: 3s                   # 单次 ping 超时
  check_interval: 5m                 # 定期检测间隔
  switch_threshold: 2.0              # 切换阈值：当前延迟 > 最优 × 此值
  switch_confirm_rounds: 3           # 连续 N 个周期超阈值才切换
```

单服务器场景也兼容简写：

```yaml
servers:
  - url: "wss://term.example.com"
    name: "默认"
token: "a1b2c3d4e5f6..."
```

### 8.2 服务端配置 (`config.yaml`)

```yaml
listen: ":8080"
region: "shanghai"                   # 节点区域标识，/health/ping 接口返回

tls:
  cert: "/path/to/cert.pem"         # 留空则 HTTP（开发环境）
  key: "/path/to/key.pem"

auth:
  tokens:
    - token: "a1b2c3d4e5f6..."
      name: "My MacBook Pro"
      access_password: "123456"      # 浏览器访问密码

session:
  max_per_agent: 10                  # 每个 Agent 最多 10 个终端
  buffer_size: 65536                 # 服务端回放缓冲 64KB
  orphan_cleanup_days: 7             # Agent 离线 7 天后清理会话

heartbeat:
  interval: 15s                      # 心跳间隔
  timeout: 45s                       # 心跳超时
```

---

## 9. PWA 支持

### 9.1 所需文件

| 文件 | 作用 |
|:---|:---|
| `manifest.json` | PWA 清单，定义名称、图标、启动方式 |
| `sw.js` | Service Worker，缓存页面壳资源 |
| HTML meta 标签 | apple-mobile-web-app-capable 等 iOS 兼容 |

### 9.2 manifest.json

```json
{
  "name": "LinkTerm",
  "short_name": "LinkTerm",
  "description": "Remote terminal for your Mac",
  "start_url": "/",
  "display": "standalone",
  "orientation": "any",
  "background_color": "#1a1b26",
  "theme_color": "#1a1b26",
  "icons": [
    { "src": "/icons/icon-192.png", "sizes": "192x192", "type": "image/png" },
    { "src": "/icons/icon-512.png", "sizes": "512x512", "type": "image/png" }
  ]
}
```

### 9.3 Service Worker 缓存策略

- 静态资源（HTML/CSS/JS/xterm）：Cache First（壳页面秒开）
- API 和 WebSocket 请求：Network Only（不走缓存）
- 离线时展示缓存的壳页面 + "连接中..." 状态

### 9.4 用户体验

添加到主屏幕后：

| 对比 | 普通网页 | PWA |
|:---|:---|:---|
| 入口 | 打开浏览器 → 输入网址 | 点击主屏图标 |
| 外观 | 有地址栏、导航栏 | 全屏，无浏览器 UI |
| 任务切换 | 显示为 Safari 标签 | 独立 App 卡片 |
| 加载速度 | 依赖网络 | 壳页面从缓存秒开 |

---

## 10. 项目结构

```
LinkTerm/
├── proto/                          # 共享协议定义
│   └── message.go
│
├── server/                         # 服务端
│   ├── main.go                     # 入口
│   ├── config.go                   # 配置加载
│   ├── hub.go                      # Agent 连接池管理
│   ├── auth.go                     # Token 验证、JWT 签发、失败锁定
│   ├── session.go                  # 终端会话管理（状态机、缓冲、超时）
│   ├── handler_web.go              # HTTP 路由（页面、API）
│   ├── handler_agent.go            # Agent WebSocket 处理
│   ├── handler_terminal.go         # 浏览器终端 WebSocket 处理
│   ├── web/                        # 前端静态资源（embed 打包进二进制）
│   │   ├── index.html              # 登录页
│   │   ├── terminal.html           # 终端页
│   │   ├── manifest.json           # PWA 清单
│   │   ├── sw.js                   # Service Worker
│   │   ├── css/
│   │   │   └── style.css
│   │   └── js/
│   │       ├── auth.js             # 登录逻辑
│   │       └── terminal.js         # xterm.js 初始化 + WebSocket + 重连
│   ├── go.mod
│   └── go.sum
│
├── agent/                          # Mac Agent
│   ├── main.go                     # 入口
│   ├── config.go                   # 读取 ~/.linkterm/config.yaml
│   ├── tunnel.go                   # WebSocket 隧道（连接、认证、收发、重连、心跳）
│   ├── selector.go                 # 智能选路（多节点测速、选择、定期检测、自动切换）
│   ├── session.go                  # PTY 会话管理（创建、输入、输出、resize、关闭）
│   ├── buffer.go                   # RingBuffer 环形缓冲实现
│   ├── tray.go                     # 菜单栏图标和菜单
│   ├── power.go                    # 防睡眠 + 电源状态检测 + 睡眠/唤醒监听
│   ├── go.mod
│   └── go.sum
│
├── deploy/                         # 部署相关
│   ├── Dockerfile                  # 服务端 Docker 镜像
│   ├── docker-compose.yml          # 一键部署
│   └── nginx.conf                  # 反向代理参考配置
│
├── scripts/
│   ├── build.sh                    # 构建脚本
│   └── install-agent.sh            # Mac Agent 安装脚本
│
├── docs/
│   └── PRD.md                      # 本文档
│
└── README.md
```

---

## 11. 安装与使用

### 11.1 服务端安装（公网服务器，一次性）

前置条件：一台有公网 IP 的 Linux 服务器，已安装 Docker。

```bash
# 1. 下载配置
mkdir -p /opt/linkterm && cd /opt/linkterm
wget https://github.com/xxx/linkterm/releases/latest/download/docker-compose.yml
wget https://github.com/xxx/linkterm/releases/latest/download/config.yaml

# 2. 编辑 config.yaml，设置域名、Token、访问密码

# 3. 启动
docker-compose up -d

# 4. 验证
curl https://term.example.com/health
```

### 11.2 Mac Agent 安装（一次性）

```bash
brew install linkterm
```

或下载 LinkTerm.app，拖入"应用程序"。

首次启动后按引导完成配置（检测 SSH → 填入服务端地址和 Token → 连接成功）。

### 11.3 手机使用

**首次**：手机扫描 Mac 上的二维码 → 输入访问密码 → 进入终端 → 添加到主屏幕。

**日常**：点击主屏幕 LinkTerm 图标 → 直接进入终端。

---

## 12. 开发计划

### Phase 1：骨架打通（第 1 周）

**目标**：浏览器里敲命令，Mac 上执行并返回结果。

| 天 | 任务 | 产出 |
|:---|:---|:---|
| Day 1 | proto 消息定义 + Server 骨架（HTTP + Agent WS） | 服务端能启动，Agent 能连上 |
| Day 2 | Agent 核心：WSS 连接 + PTY spawn + 消息收发 | Agent 连上服务端，能创建 PTY |
| Day 3 | Server 终端 WS + 浏览器 ↔ Agent 数据路由 | 服务端能转发数据 |
| Day 4 | Web 前端：最简 xterm.js 页面 + WebSocket | 浏览器能看到终端 |
| Day 5 | 联调 + 端到端测试 | **可运行的 Demo** |

### Phase 2：认证 + 长连接（第 2 周）

**目标**：安全可用，断连自动恢复，会话持久。

| 天 | 任务 | 产出 |
|:---|:---|:---|
| Day 6 | Agent Token 认证 + 浏览器登录 + JWT | 未授权连接被拒绝 |
| Day 7 | Agent 自动重连 + 心跳 + 系统唤醒监听 | 断网恢复后自动重连 |
| Day 8 | 两级缓冲（Agent 本地 + 服务端） | 断连期间输出不丢 |
| Day 9 | 会话保持 + 浏览器重连 + 缓冲回放 | 手机锁屏再回来终端还在 |
| Day 10 | Agent 重连恢复 + 会话状态机 + TLS | 完整的长连接体系 |
| Day 11 | 暴力破解防护 + 会话资源保护 | 安全加固 |

### Phase 3：体验打磨（第 3 周）

**目标**：真正好用，可以日常使用。

| 天 | 任务 | 产出 |
|:---|:---|:---|
| Day 12 | 菜单栏：状态显示、操作面板、二维码 | Mac 端完整交互 |
| Day 13 | 防睡眠 + 电源检测 + 用户提示系统 | 合盖不断连 + 智能提示 |
| Day 14 | 移动端适配：辅助键工具栏、触摸优化 | 手机上好用 |
| Day 15 | PWA：manifest + Service Worker + 引导安装 | 主屏图标，秒开体验 |
| Day 16 | Web UI 美化 + 会话列表 + 状态提示条 | 视觉完整 |
| Day 17 | Docker 部署打包 + 安装脚本 + 文档 | **可分发的 MVP** |

### Phase 4：增强（第 4 周，可选）

| 任务 | 说明 |
|:---|:---|
| 智能选路 | 多节点测速 + 自动选择 + 定期检测 + 自动切换 + 菜单栏节点状态展示 |
| 开机自启 | macOS LaunchAgent |
| 多终端标签 | 浏览器同时开多个 shell |
| 终端主题 | 亮色/暗色/自定义 |
| 首次引导 Wizard | 完整的 Setup 流程 |
| 多 Agent 支持 | 多台 Mac 同时接入 |

---

## 13. 非功能需求

### 13.1 性能

- WebSocket 隧道建立时间 < 2 秒
- 终端操作延迟 < 150ms（同运营商）
- 浏览器重连时间 < 1 秒
- Agent 唤醒后重连时间 < 2 秒
- 单服务端支持 1000+ 并发 Agent 连接

### 13.2 兼容性

- Mac Agent：macOS 12+，Intel / Apple Silicon 通用二进制
- 服务端：Linux x86_64 / ARM64，Docker 部署
- 浏览器：Safari 15+、Chrome 90+（iOS / Android / 桌面）

### 13.3 可靠性

- Agent 自动重连，无需人工干预
- 会话存活不依赖连接状态
- 两级缓冲防止输出丢失
- 服务端重启后会话可恢复
