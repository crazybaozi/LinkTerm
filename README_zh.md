[English](README.md) | **中文**

# LinkTerm

**手机上用 Claude Code / Kimi K2 — 浏览器远程操作 Mac 终端，无需安装任何 App。**

LinkTerm 让你的手机变成 Mac 的远程终端。专为使用 [Claude Code](https://docs.anthropic.com/en/docs/claude-code)、[Kimi K2](https://kimi.ai) 等 AI 编程工具的开发者打造，无论在沙发上、咖啡馆里还是通勤路上，打开浏览器就能写代码。

```
手机/iPad (浏览器)  ──HTTPS/WSS──>  服务端 (云服务器)  <──WSS──  Mac Agent
```

## 为什么用 LinkTerm？

- **手机当终端** — 打开浏览器，连上 Mac，运行 `claude` 或任何命令行工具
- **手机零安装** — 不用下载 App，Safari / Chrome 直接用
- **会话不丢失** — 锁屏、切应用，回来后一切都还在
- **共享 Mac 终端** — 执行 `linkterm-agent share`，手机实时同步查看和操作
- **一行命令安装** — 几秒钟装好，随时随地开始编程

---

# 一、快速体验

无需部署服务端、无需下载源码，一行命令即可体验全部功能。

## 安装

在 Mac 终端执行：

```bash
curl -fsSL https://raw.githubusercontent.com/crazybaozi/LinkTerm/main/scripts/install.sh | bash -s -- --server ws://linkterm1.lbai.ai:8080
```

安装完成后，菜单栏出现 **L⚡** 图标，表示 Agent 已在后台运行。

## 手机连接

1. 手机浏览器打开 **http://linkterm1.lbai.ai:8080**
2. 点击 Mac 菜单栏 **L⚡** 图标 → 复制 Token
3. 在手机页面输入 Token，即可远程操作 Mac 终端

> 添加到主屏幕体验更好：iOS Safari 分享 → 添加到主屏幕 / Android Chrome → 安装应用

## 手机电脑共享同一终端

在 Mac 的 Terminal.app / iTerm2 中执行，手机和电脑共享同一个终端，适合监控 Claude Code 等长时间任务：

```bash
linkterm-agent share
```

Mac 和手机看到相同输出，双端都可以输入。退出方式：输入 `exit` 或 `Ctrl+D`。

示例：共享终端后启动 Claude Code

```
$ linkterm-agent share
[LinkTerm] Terminal shared (session: share-1773206147041873000)
[LinkTerm] Phone can now connect to see this terminal

$ claude
╭────────────────────────────────────────╮
│ ✻ Welcome to Claude Code!              │
│   /help for help                       │
╰────────────────────────────────────────╯
>
```

> **注意：** 需要在启动任务**之前**执行 `linkterm-agent share`，无法回溯接入已运行的进程。

## Mac 菜单栏

闪电颜色表示连接状态：🟢 已连接 / 🔴 未连接 / 🟡 连接中。

点击图标可以：
- **访问本机 Token** — 复制后粘贴到手机浏览器登录
- **服务器地址** — 点击复制
- **重新连接** — 手动重连
- **退出 LinkTerm** — 停止 Agent

## Agent 管理

```bash
# 查看状态
curl -fsSL https://raw.githubusercontent.com/crazybaozi/LinkTerm/main/scripts/install.sh | bash -s -- --status

# 卸载
curl -fsSL https://raw.githubusercontent.com/crazybaozi/LinkTerm/main/scripts/install.sh | bash -s -- --uninstall

# 手动停止 / 启动
launchctl bootout gui/$(id -u)/com.linkterm.agent
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.linkterm.agent.plist

# 查看日志
tail -f ~/.linkterm/agent.log
```

## 常见问题

| 问题 | 解决 |
|:---|:---|
| Mac 离线 | 检查 Agent 是否运行：`pgrep -f linkterm-agent` |
| 查看 Token | 菜单栏点击 **L⚡** 图标 →「访问本机 Token」 |
| Token 泄露 | 删除 `~/.linkterm/config.yaml` 中 token 字段后重启 Agent，旧 Token 立即失效 |
| 手机锁屏回来卡住 | 1-3 秒自动重连；如果失败，页面有「重新连接」按钮 |
| 合盖断连 | 确认 `prevent_sleep: true` 且 Mac **接电源** |

> 测试服务器仅供体验，正式使用请参考下方自行部署。

---

# 二、自行部署

### 1. 部署服务端

```bash
git clone https://github.com/crazybaozi/LinkTerm.git
cd LinkTerm
docker compose build && docker compose up -d
```

### 2. 安装 Agent

**远程安装（无需 clone）：**

```bash
curl -fsSL https://raw.githubusercontent.com/crazybaozi/LinkTerm/main/scripts/install.sh | bash -s -- --server wss://你的域名
```

**本地安装（已 clone 项目）：**

```bash
bash scripts/install.sh
```

### 3. 配置参考

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

## License

MIT
