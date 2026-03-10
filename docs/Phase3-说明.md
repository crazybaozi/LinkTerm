# Phase 3 — 体验打磨 · 部署打包

## 本阶段完成的功能

### 1. Docker 服务端部署
- `server/Dockerfile`：两阶段构建（builder + alpine），最终镜像极小
- `docker-compose.yml`：一键启动，挂载 config 和 TLS 证书
- `deploy/config.yaml`：生产配置模板，标注所有必须修改的字段

### 2. JWT Secret 安全化
- `AuthConfig` 新增 `jwt_secret` 字段，从 config.yaml 读取
- `auth.go` 不再硬编码 JWT 密钥
- 开发模式自动使用默认密钥

### 3. Agent 防睡眠
- `agent/sleep.go`：通过 `caffeinate -di` 阻止 Mac 空闲睡眠（display + idle）
- 配置 `prevent_sleep: true` 启用
- 进程退出时自动停止 caffeinate

### 4. Agent 启动信息美化
- `agent/qrcode.go`：启动后在终端显示格式化的连接信息框
- 展示服务器名称和访问 URL

### 5. PWA 完善
- SVG 图标 (`icons/icon.svg`)：终端风格 > 和 _ 图标
- `manifest.json` 更新为 SVG 图标引用
- 登录页增加 PWA 安装引导横幅（beforeinstallprompt）
- 两个页面均注册 Service Worker

### 6. 移动端 UX 优化
- **字体大小调节**：工具栏 A+/A- 按钮，偏好存入 localStorage
- **粘贴按钮**：调用 Clipboard API 读取剪贴板并发送到终端
- **新增常用键**：`-` `/` `$`
- **横屏适配**：`max-height: 500px` 时压缩状态栏和工具栏高度
- **退出登录**：菜单新增退出按钮，清除 token

### 7. 生产部署方案
- `deploy/config.yaml`：服务端配置模板
- `deploy/agent-config.yaml`：Agent 配置模板
- `deploy/install-agent.sh`：macOS Agent 安装脚本（含 LaunchAgent 开机自启）
- `docs/部署指南.md`：完整部署文档（Docker + TLS + Nginx + Agent + 常见问题）

## 新增/修改的文件

| 文件 | 说明 |
|------|------|
| `server/Dockerfile` | 两阶段 Docker 构建 |
| `docker-compose.yml` | 一键部署 |
| `deploy/config.yaml` | 服务端配置模板 |
| `deploy/agent-config.yaml` | Agent 配置模板 |
| `deploy/install-agent.sh` | Agent 安装脚本 |
| `server/config.go` | AuthConfig 增加 jwt_secret |
| `server/auth.go` | JWT 密钥从配置读取 |
| `agent/sleep.go` | caffeinate 防睡眠 |
| `agent/qrcode.go` | 启动信息展示 |
| `agent/main.go` | 集成防睡眠 + 信息展示 |
| `server/web/icons/icon.svg` | PWA 图标 |
| `server/web/manifest.json` | SVG 图标 |
| `server/web/index.html` | 安装横幅 + SW 注册 |
| `server/web/terminal.html` | 新增工具按钮 + SW 注册 |
| `server/web/js/terminal.js` | 字体调节 + 粘贴 + 退出 |
| `server/web/css/style.css` | 安装横幅 + 工具按钮 + 横屏适配 |
| `docs/部署指南.md` | 完整部署文档 |

## 验证结果

- ✅ Server + Agent 编译通过
- ✅ API 全流程正常（auth → agents → sessions → terminal WS）
- ✅ 端到端输入输出正常
- ✅ 所有静态资源正确提供（HTML/CSS/JS/manifest/SW/icon）
- ✅ Agent 启动显示格式化连接信息
