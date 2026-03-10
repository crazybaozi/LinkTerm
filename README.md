# LinkTerm

**手机浏览器远程操作 Mac 终端，打开即用，无需安装任何 App。**

```
手机/iPad (浏览器)  ──HTTPS/WSS──>  服务端 (云服务器)  <──WSS──  Mac Agent
```

## 快速开始

### 1. 部署服务端

```bash
git clone https://github.com/crazybaozi/LinkTerm.git
cd LinkTerm
docker compose build && docker compose up -d
```

### 2. 安装 Mac Agent

**一键远程安装（推荐，无需 clone 项目）：**

```bash
curl -fsSL https://raw.githubusercontent.com/crazybaozi/LinkTerm/main/scripts/install.sh | bash
```

指定地址跳过交互：

```bash
curl -fsSL https://raw.githubusercontent.com/crazybaozi/LinkTerm/main/scripts/install.sh | bash -s -- --server wss://你的域名
```

**本地安装（已 clone 项目）：**

```bash
bash scripts/install.sh
```

脚本自动完成：检测架构 → 下载二进制 → 配置服务端地址 → 生成 Token → 开机自启 → 启动。

### 3. 手机访问

1. 手机浏览器打开 `https://你的服务器域名`
2. 输入 Token（Mac 菜单栏点击图标即可复制）
3. 进入终端，开始操作

> 添加到主屏幕体验更好：iOS Safari 分享 → 添加到主屏幕 / Android Chrome → 安装应用

---

## Mac 菜单栏

Agent 启动后在菜单栏显示 **L⚡** 图标，闪电颜色表示状态：🟢 已连接 / 🔴 未连接 / 🟡 连接中。

点击图标可以：
- **访问本机 Token** — 点击复制，粘贴到手机浏览器登录
- **服务器地址** — 点击复制
- **重新连接** — 手动重连
- **退出 LinkTerm** — 停止 Agent

---

## Agent 管理

```bash
# 查看状态（二选一）
bash scripts/install.sh --status
curl -fsSL https://raw.githubusercontent.com/crazybaozi/LinkTerm/main/scripts/install.sh | bash -s -- --status

# 卸载（二选一）
bash scripts/install.sh --uninstall
curl -fsSL https://raw.githubusercontent.com/crazybaozi/LinkTerm/main/scripts/install.sh | bash -s -- --uninstall

# 手动停止 / 启动
launchctl bootout gui/$(id -u)/com.linkterm.agent
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.linkterm.agent.plist

# 查看日志
tail -f ~/.linkterm/agent.log
```

---

## 配置

**服务端** `deploy/config.yaml`

```yaml
listen: ":8080"
session:
  max_per_agent: 10
  buffer_size: 65536
```

**Agent** `~/.linkterm/config.yaml`（安装脚本自动生成）

```yaml
servers:
  - url: "wss://your-server.com"
    name: "主节点"
token: ""              # 留空自动生成
prevent_sleep: true    # 合盖接电源不断连
max_sessions: 10
```

---

## 常见问题

| 问题 | 解决 |
|:---|:---|
| Mac 离线 | 检查 Agent 是否运行：`pgrep -f linkterm-agent` 或 `bash scripts/install.sh --status` |
| 查看 Token | 菜单栏点击图标 →「访问本机 Token」 |
| Token 泄露 | 删除 config.yaml 中 token 字段后重启 Agent，旧 Token 立即失效 |
| 手机锁屏回来卡住 | 1-3 秒自动重连；如果失败，页面有「重新连接」按钮 |
| 合盖断连 | 确认 `prevent_sleep: true` 且 **接电源** |

---

## License

MIT
