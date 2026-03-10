#!/bin/bash
#
# LinkTerm Agent 一键安装脚本
#
# 用法:
#   curl -sSL https://raw.githubusercontent.com/crazybaozi/LinkTerm/main/scripts/install.sh | bash
#   bash scripts/install.sh
#   bash scripts/install.sh --server wss://term.example.com
#   bash scripts/install.sh --uninstall
#
set -e

# ============================================================
#  Constants
# ============================================================
REPO="crazybaozi/LinkTerm"
VERSION="latest"
INSTALL_DIR="$HOME/.linkterm"
BIN_NAME="linkterm-agent"
BIN_PATH="$INSTALL_DIR/$BIN_NAME"
CONFIG_PATH="$INSTALL_DIR/config.yaml"
PLIST_NAME="com.linkterm.agent"
PLIST_PATH="$HOME/Library/LaunchAgents/${PLIST_NAME}.plist"
LOG_PATH="$INSTALL_DIR/agent.log"

# ============================================================
#  Colors & Formatting
# ============================================================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

# ============================================================
#  Output helpers
# ============================================================
print_banner() {
    echo ""
    echo -e "${CYAN}╔══════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║${NC}  ${BOLD}LinkTerm Agent Installer${NC}                    ${CYAN}║${NC}"
    echo -e "${CYAN}║${NC}  ${DIM}手机浏览器远程操作 Mac 终端${NC}                 ${CYAN}║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════════════╝${NC}"
    echo ""
}

step() {
    echo -e "\n${BLUE}[$1/$TOTAL_STEPS]${NC} ${BOLD}$2${NC}"
}

ok() {
    echo -e "  ${GREEN}✓${NC} $1"
}

warn() {
    echo -e "  ${YELLOW}!${NC} $1"
}

fail() {
    echo -e "  ${RED}✗${NC} $1"
}

info() {
    echo -e "  ${DIM}→${NC} $1"
}

ask() {
    local prompt="$1"
    local default="$2"
    local result=""

    if [ -n "$default" ]; then
        prompt="${prompt} ${DIM}(${default})${NC}"
    fi

    echo -en "  ${MAGENTA}?${NC} ${prompt}: " >&2

    # 兼容 curl | bash 模式
    if [ -t 0 ]; then
        read -r result
    else
        read -r result < /dev/tty
    fi

    if [ -z "$result" ] && [ -n "$default" ]; then
        result="$default"
    fi

    echo "$result"
}

ask_yn() {
    local prompt="$1"
    local default="${2:-Y}"
    local hint="Y/n"
    [ "$default" = "n" ] || [ "$default" = "N" ] && hint="y/N"

    local answer
    answer=$(ask "$prompt [$hint]" "")

    if [ -z "$answer" ]; then
        answer="$default"
    fi

    case "$answer" in
        [yY]*) return 0 ;;
        *) return 1 ;;
    esac
}

# ============================================================
#  System detection
# ============================================================
detect_system() {
    local os arch

    os="$(uname -s)"
    if [ "$os" != "Darwin" ]; then
        fail "当前仅支持 macOS，检测到: $os"
        exit 1
    fi

    arch="$(uname -m)"
    case "$arch" in
        arm64|aarch64) ARCH="arm64" ;;
        x86_64)        ARCH="amd64" ;;
        *)
            fail "不支持的架构: $arch"
            exit 1
            ;;
    esac

    OS_VERSION="$(sw_vers -productVersion 2>/dev/null || echo 'unknown')"
    CHIP_NAME=""
    if [ "$ARCH" = "arm64" ]; then
        CHIP_NAME="Apple Silicon"
    else
        CHIP_NAME="Intel"
    fi

    ok "macOS ${OS_VERSION} (${CHIP_NAME} ${ARCH})"
}

# ============================================================
#  Binary installation
# ============================================================
install_binary() {
    mkdir -p "$INSTALL_DIR"

    # 优先级: --local 参数 > 项目 bin 目录 > GitHub Releases > 本地编译
    if [ -n "$LOCAL_BIN" ]; then
        install_from_local "$LOCAL_BIN"
        return
    fi

    # 检查项目 bin 目录（开发者本地安装场景）
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    local project_bin="${script_dir}/../bin/${BIN_NAME}"
    if [ -f "$project_bin" ]; then
        info "检测到项目编译产物"
        install_from_local "$project_bin"
        return
    fi

    # 从 GitHub Releases 下载
    if download_from_github; then
        return
    fi

    # 尝试本地编译
    if build_locally; then
        return
    fi

    fail "无法获取 ${BIN_NAME} 二进制文件"
    echo ""
    echo -e "  请手动编译后重试:"
    echo -e "    ${DIM}cd agent && go build -o ${BIN_PATH} . ${NC}"
    echo -e "  或指定已有的二进制文件:"
    echo -e "    ${DIM}bash install.sh --local /path/to/${BIN_NAME}${NC}"
    exit 1
}

install_from_local() {
    local src="$1"
    if [ ! -f "$src" ]; then
        fail "文件不存在: $src"
        exit 1
    fi
    cp "$src" "$BIN_PATH"
    chmod +x "$BIN_PATH"
    ok "已安装到 ${DIM}${BIN_PATH}${NC}"
}

download_from_github() {
    local url="https://github.com/${REPO}/releases/latest/download/${BIN_NAME}-darwin-${ARCH}"

    info "从 GitHub Releases 下载..."

    local http_code
    if command -v curl &>/dev/null; then
        http_code=$(curl -fsSL -w "%{http_code}" -o "$BIN_PATH" "$url" 2>/dev/null) || true
    elif command -v wget &>/dev/null; then
        wget -q -O "$BIN_PATH" "$url" 2>/dev/null && http_code="200" || true
    fi

    if [ "$http_code" = "200" ] && [ -f "$BIN_PATH" ] && [ -s "$BIN_PATH" ]; then
        chmod +x "$BIN_PATH"
        ok "已下载并安装到 ${DIM}${BIN_PATH}${NC}"
        return 0
    fi

    rm -f "$BIN_PATH"
    warn "GitHub Releases 下载失败，尝试其他方式..."
    return 1
}

build_locally() {
    if ! command -v go &>/dev/null; then
        warn "未安装 Go，跳过本地编译"
        return 1
    fi

    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    local agent_dir="${script_dir}/../agent"

    if [ ! -f "${agent_dir}/main.go" ]; then
        warn "未找到 agent 源码目录，跳过本地编译"
        return 1
    fi

    info "使用 Go 本地编译..."
    (cd "$agent_dir" && GOOS=darwin GOARCH="$ARCH" go build -o "$BIN_PATH" .) || {
        warn "编译失败"
        return 1
    }
    chmod +x "$BIN_PATH"
    ok "已编译并安装到 ${DIM}${BIN_PATH}${NC}"
    return 0
}

# ============================================================
#  Configuration
# ============================================================
generate_token() {
    echo "lt_$(openssl rand -hex 32)"
}

setup_config() {
    if [ -f "$CONFIG_PATH" ]; then
        warn "已存在配置文件: ${CONFIG_PATH}"
        if ! ask_yn "是否覆盖现有配置?" "n"; then
            ok "保留现有配置"
            TOKEN=$(grep -E '^\s*token:' "$CONFIG_PATH" | sed 's/.*token:\s*"\{0,1\}\([^"]*\)"\{0,1\}/\1/' | tr -d ' ')
            return
        fi
    fi

    # 服务端地址
    if [ -z "$SERVER_URL" ]; then
        echo ""
        echo -e "  ${DIM}服务端地址格式: wss://your-server.com 或 ws://ip:port${NC}"
        SERVER_URL=$(ask "请输入服务端地址" "ws://127.0.0.1:8080")
    fi

    # 生成 Token
    TOKEN=$(generate_token)

    # 服务端名称
    local server_name
    server_name=$(echo "$SERVER_URL" | sed 's|wss\{0,1\}://||' | sed 's|/.*||')

    # 写入配置
    cat > "$CONFIG_PATH" << YAML
servers:
  - url: "${SERVER_URL}"
    name: "${server_name}"

token: "${TOKEN}"
name: ""
auto_connect: true
prevent_sleep: true
reconnect_max_interval: 30
local_buffer_size: 131072
max_sessions: 10
YAML

    chmod 600 "$CONFIG_PATH"
    ok "配置已写入 ${DIM}${CONFIG_PATH}${NC}"
    ok "Token 已自动生成"
}

# ============================================================
#  LaunchAgent (auto-start on login)
# ============================================================
setup_launchagent() {
    if ! ask_yn "是否设置开机自动启动?"; then
        info "跳过开机自启配置"
        AUTOSTART=false
        return
    fi

    mkdir -p "$HOME/Library/LaunchAgents"

    cat > "$PLIST_PATH" << PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>${PLIST_NAME}</string>
    <key>ProgramArguments</key>
    <array>
        <string>${BIN_PATH}</string>
        <string>-config</string>
        <string>${CONFIG_PATH}</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <dict>
        <key>NetworkState</key>
        <true/>
    </dict>
    <key>StandardOutPath</key>
    <string>${LOG_PATH}</string>
    <key>StandardErrorPath</key>
    <string>${LOG_PATH}</string>
    <key>ThrottleInterval</key>
    <integer>5</integer>
</dict>
</plist>
PLIST

    AUTOSTART=true
    ok "已注册为登录项"
}

# ============================================================
#  Start agent
# ============================================================
start_agent() {
    # 停止已有实例
    launchctl bootout "gui/$(id -u)/${PLIST_NAME}" 2>/dev/null || true
    pkill -f "$BIN_PATH" 2>/dev/null || true
    sleep 1

    if [ "$AUTOSTART" = true ] && [ -f "$PLIST_PATH" ]; then
        launchctl bootstrap "gui/$(id -u)" "$PLIST_PATH" 2>/dev/null || \
        launchctl load "$PLIST_PATH" 2>/dev/null || true
        ok "Agent 已通过 LaunchAgent 启动"
    else
        nohup "$BIN_PATH" -config "$CONFIG_PATH" > "$LOG_PATH" 2>&1 &
        ok "Agent 已在后台启动 (PID: $!)"
    fi

    # 等待启动
    sleep 2

    if pgrep -f "$BIN_NAME" > /dev/null 2>&1; then
        ok "Agent 运行中"
    else
        warn "Agent 可能未成功启动，请检查日志: ${LOG_PATH}"
    fi
}

# ============================================================
#  Print result
# ============================================================
print_result() {
    local display_url
    display_url=$(echo "$SERVER_URL" | sed 's|^wss://|https://|' | sed 's|^ws://|http://|')

    local token_short
    if [ ${#TOKEN} -gt 24 ]; then
        token_short="${TOKEN:0:20}..."
    else
        token_short="$TOKEN"
    fi

    echo ""
    echo -e "${GREEN}╔══════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║${NC}  ${BOLD}${GREEN}安装完成!${NC}                                   ${GREEN}║${NC}"
    echo -e "${GREEN}╠══════════════════════════════════════════════╣${NC}"
    echo -e "${GREEN}║${NC}                                              ${GREEN}║${NC}"
    echo -e "${GREEN}║${NC}  ${BOLD}服务端地址${NC}  ${display_url}"
    echo -e "${GREEN}║${NC}  ${BOLD}访问Token${NC}  ${CYAN}${TOKEN}${NC}"
    echo -e "${GREEN}║${NC}                                              ${GREEN}║${NC}"
    echo -e "${GREEN}║${NC}  ${DIM}手机浏览器打开上面的服务端地址${NC}              ${GREEN}║${NC}"
    echo -e "${GREEN}║${NC}  ${DIM}输入 Token 即可远程访问当前电脑终端${NC}         ${GREEN}║${NC}"
    echo -e "${GREEN}║${NC}                                              ${GREEN}║${NC}"
    echo -e "${GREEN}╠══════════════════════════════════════════════╣${NC}"
    echo -e "${GREEN}║${NC}  ${BOLD}常用命令${NC}                                      ${GREEN}║${NC}"
    echo -e "${GREEN}║${NC}                                              ${GREEN}║${NC}"
    echo -e "${GREEN}║${NC}  查看日志  ${DIM}tail -f ${LOG_PATH}${NC}"
    echo -e "${GREEN}║${NC}  查看Token ${DIM}grep token ${CONFIG_PATH}${NC}"
    echo -e "${GREEN}║${NC}  编辑配置  ${DIM}vi ${CONFIG_PATH}${NC}"

    if [ "$AUTOSTART" = true ]; then
    echo -e "${GREEN}║${NC}  停止      ${DIM}launchctl bootout gui/$(id -u)/${PLIST_NAME}${NC}"
    echo -e "${GREEN}║${NC}  启动      ${DIM}launchctl bootstrap gui/$(id -u) ${PLIST_PATH}${NC}"
    else
    echo -e "${GREEN}║${NC}  停止      ${DIM}pkill -f ${BIN_NAME}${NC}"
    echo -e "${GREEN}║${NC}  启动      ${DIM}${BIN_PATH} -config ${CONFIG_PATH}${NC}"
    fi

    echo -e "${GREEN}║${NC}  卸载      ${DIM}bash $0 --uninstall${NC}"
    echo -e "${GREEN}║${NC}                                              ${GREEN}║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════╝${NC}"
    echo ""
}

# ============================================================
#  Uninstall
# ============================================================
do_uninstall() {
    echo ""
    echo -e "${BOLD}LinkTerm Agent 卸载${NC}"
    echo ""

    if ! ask_yn "确定要卸载 LinkTerm Agent 吗?" "n"; then
        echo "已取消"
        exit 0
    fi

    echo ""

    # 停止服务
    launchctl bootout "gui/$(id -u)/${PLIST_NAME}" 2>/dev/null || true
    launchctl unload "$PLIST_PATH" 2>/dev/null || true
    pkill -f "$BIN_NAME" 2>/dev/null || true
    ok "已停止 Agent"

    # 移除 LaunchAgent
    if [ -f "$PLIST_PATH" ]; then
        rm -f "$PLIST_PATH"
        ok "已移除开机自启配置"
    fi

    # 是否保留配置
    if [ -d "$INSTALL_DIR" ]; then
        if ask_yn "是否保留配置文件和日志? (${INSTALL_DIR})" "Y"; then
            rm -f "$BIN_PATH"
            ok "已移除二进制文件，配置已保留"
        else
            rm -rf "$INSTALL_DIR"
            ok "已移除所有文件 (${INSTALL_DIR})"
        fi
    fi

    echo ""
    echo -e "${GREEN}卸载完成${NC}"
    echo ""
}

# ============================================================
#  Upgrade
# ============================================================
do_upgrade() {
    echo ""
    echo -e "${BOLD}LinkTerm Agent 升级${NC}"
    echo ""

    if [ ! -f "$BIN_PATH" ]; then
        fail "未找到已安装的 Agent，请先执行安装"
        exit 1
    fi

    TOTAL_STEPS=3

    step 1 "检测系统环境..."
    detect_system

    step 2 "下载新版本..."
    local old_bin="${BIN_PATH}.old"
    cp "$BIN_PATH" "$old_bin"

    if ! download_from_github; then
        cp "$old_bin" "$BIN_PATH"
        rm -f "$old_bin"
        fail "升级失败，已回滚"
        exit 1
    fi
    rm -f "$old_bin"

    step 3 "重启 Agent..."
    start_agent

    echo ""
    echo -e "${GREEN}升级完成${NC}"
    echo ""
}

# ============================================================
#  Status
# ============================================================
do_status() {
    echo ""
    echo -e "${BOLD}LinkTerm Agent 状态${NC}"
    echo ""

    if [ -f "$BIN_PATH" ]; then
        ok "二进制文件: ${BIN_PATH}"
    else
        fail "二进制文件: 未安装"
    fi

    if [ -f "$CONFIG_PATH" ]; then
        ok "配置文件: ${CONFIG_PATH}"
        local token
        token=$(grep -E '^\s*token:' "$CONFIG_PATH" | sed 's/.*token:\s*"\{0,1\}\([^"]*\)"\{0,1\}/\1/' | tr -d ' ')
        if [ -n "$token" ]; then
            info "Token: ${CYAN}${token}${NC}"
        fi
    else
        fail "配置文件: 未创建"
    fi

    if [ -f "$PLIST_PATH" ]; then
        ok "开机自启: 已配置"
    else
        info "开机自启: 未配置"
    fi

    if pgrep -f "$BIN_NAME" > /dev/null 2>&1; then
        local pid
        pid=$(pgrep -f "$BIN_NAME" | head -1)
        ok "运行状态: ${GREEN}运行中${NC} (PID: ${pid})"
    else
        fail "运行状态: ${RED}未运行${NC}"
    fi

    echo ""
}

# ============================================================
#  Parse arguments
# ============================================================
TOTAL_STEPS=4
SERVER_URL=""
LOCAL_BIN=""
TOKEN=""
AUTOSTART=""
ACTION="install"

while [ $# -gt 0 ]; do
    case "$1" in
        --uninstall|uninstall)
            ACTION="uninstall"
            shift
            ;;
        --upgrade|upgrade)
            ACTION="upgrade"
            shift
            ;;
        --status|status)
            ACTION="status"
            shift
            ;;
        --server)
            SERVER_URL="$2"
            shift 2
            ;;
        --local)
            LOCAL_BIN="$2"
            shift 2
            ;;
        --help|-h)
            echo "LinkTerm Agent 安装脚本"
            echo ""
            echo "用法:"
            echo "  bash install.sh              交互式安装"
            echo "  bash install.sh --server URL 指定服务端地址安装"
            echo "  bash install.sh --local PATH 从本地文件安装"
            echo "  bash install.sh --uninstall  卸载"
            echo "  bash install.sh --upgrade    升级"
            echo "  bash install.sh --status     查看状态"
            echo ""
            exit 0
            ;;
        *)
            echo "未知参数: $1 (使用 --help 查看帮助)"
            exit 1
            ;;
    esac
done

# ============================================================
#  Main
# ============================================================
case "$ACTION" in
    uninstall) do_uninstall; exit 0 ;;
    upgrade)   do_upgrade;   exit 0 ;;
    status)    do_status;    exit 0 ;;
esac

print_banner

step 1 "检测系统环境..."
detect_system

step 2 "安装 Agent..."
install_binary

step 3 "配置 Agent..."
setup_config

step 4 "设置开机自启..."
setup_launchagent

echo ""
echo -e "${BLUE}[*]${NC} ${BOLD}启动 Agent...${NC}"
start_agent

print_result
