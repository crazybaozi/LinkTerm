# Phase 4 — 智能选路 · 主题 · 增强

## 本阶段完成的功能

### 1. 智能选路 (agent/selector.go)

完整实现 PRD 中描述的多节点测速选路机制：

#### 测速
- 并发 ping 所有配置的服务端节点（`/health/ping` 接口）
- 每个节点连续 ping 3 次，取中位数延迟，避免网络抖动干扰
- 超时 3 秒的节点标记为不可达

#### 选路时机
| 时机 | 行为 |
|------|------|
| Agent 首次启动 | 并发测速 → 连接最优节点 |
| 连接断开重连 | 先快试当前节点 → 失败则全量测速选路 |
| 运行期间定期检测 | 每 5 分钟后台测速（不影响当前连接） |
| 当前节点延迟异常 | 连续 3 个周期发现更优节点（延迟差 2 倍以上）→ 自动切换 |

#### 切换策略
- 先连新节点，确认成功后断旧节点（先连后断，避免不可用窗口）
- PTY 进程在本地，切换服务器不受影响
- 切换后自动恢复所有存活会话（`session_resume`）
- 所有节点延迟相近（差距不足 2 倍）时不切换，保持稳定

#### 集成
- `tunnel.go` 新增 `SetReconnectHandler`，Selector 接管重连逻辑
- `main.go` 使用 Selector 替代原先硬编码连接第一个节点

### 2. 终端主题切换

三款内置主题，一键切换：

| 主题 | 风格 | 背景色 |
|------|------|--------|
| **Tokyo Night** (默认) | 深蓝暗色 | `#1a1b26` |
| **One Light** | 浅白亮色 | `#fafafa` |
| **Dracula** | 紫色暗色 | `#282a36` |

- 菜单「主题」区域显示 3 个按钮
- 选择后实时生效，偏好存入 localStorage
- 切换主题同步更新 body 背景色

## 新增/修改的文件

| 文件 | 说明 |
|------|------|
| `agent/selector.go` | 智能选路核心（测速、选择、监测、切换） |
| `agent/tunnel.go` | 新增 `SetReconnectHandler`，支持外部重连处理器 |
| `agent/main.go` | 使用 Selector 管理连接生命周期 |
| `server/web/js/terminal.js` | 主题定义 + 切换逻辑 |
| `server/web/terminal.html` | 菜单新增主题选择区域 |
| `server/web/css/style.css` | 主题按钮样式 |

## 验证结果

- ✅ Server + Agent 编译通过
- ✅ Selector 启动时测速：`local: 1ms`，选择最优节点连接
- ✅ 端到端数据流正常（`echo PHASE4-OK` → 回显确认）
- ✅ Agent 输出美化信息框
- ✅ 单节点场景下 Monitor 不启动（避免无意义检测）
