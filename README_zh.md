[English](README.md) | **中文**

# LinkTerm

**手机上用 Claude Code / Kimi K2 — 浏览器远程操作 Mac 终端，无需安装任何 App。**

LinkTerm 让你的手机变成 Mac 的远程终端。专为使用 [Claude Code](https://docs.anthropic.com/en/docs/claude-code)、[Kimi K2](https://kimi.ai) 等 AI 编程工具的开发者打造，无论在沙发上、咖啡馆里还是通勤路上，打开浏览器就能写代码。

```
手机/iPad (浏览器)  ──HTTPS/WSS──>  服务端 (云服务器)  <──WSS──  Mac Agent
```

## 为什么用 LinkTerm？

Claude Code、Kimi K2 等 AI 编程助手运行在终端里，但你不可能一直坐在电脑前。LinkTerm 解决这个问题：

- **手机当终端** — 打开浏览器，连上 Mac，运行 `claude` 或任何命令行工具
- **手机零安装** — 不用下载 App，Safari / Chrome 直接用
- **会话不丢失** — 锁屏、切应用，回来后一切都还在
- **共享 Mac 终端** — 在 Terminal.app 中执行 `linkterm-agent share`，手机实时同步查看和操作
- **一行命令安装** — 几秒钟装好 Mac 端，随时随地开始编程

## 快速测试

无需部署服务端，使用公共测试服务器，一行命令体验：

```bash
curl -fsSL https://raw.githubusercontent.com/crazybaozi/LinkTerm/main/scripts/install.sh | bash -s -- --server ws://linkterm1.lbai.ai:8080
```

安装完成后：
1. 菜单栏出现 **L⚡** 图标，点击复制 Token
2. 手机浏览器打开 `http://linkterm1.lbai.ai:8080`
3. 输入 Token，即可远程操作 Mac 终端

> 测试服务器仅供体验，正式使用请自行部署。

---

## 快速开始

### 1. 部署服务端

```bash
git clone https://github.com/crazybaozi/LinkTerm.git
cd LinkTerm
docker compose build && docker compose up -d
```

### 2. 在 Mac 电脑上安装 Agent

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

## 终端共享

将 Mac 终端共享到手机，适合监控 Claude Code 等长时间运行的任务：

```bash
# 共享一个新的 shell 会话
linkterm-agent share

# 共享指定命令
linkterm-agent share -- claude
```

在 Terminal.app / iTerm2 中执行后，手机端自动出现该会话。Mac 和手机看到相同输出，双端都可以输入。

> **注意：** 需要在启动任务**之前**执行 `linkterm-agent share`，无法回溯接入已运行的进程。

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
