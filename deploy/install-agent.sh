#!/bin/bash
set -e

INSTALL_DIR="$HOME/.linkterm"
BIN_PATH="$INSTALL_DIR/linkterm-agent"
CONFIG_PATH="$INSTALL_DIR/config.yaml"
PLIST_PATH="$HOME/Library/LaunchAgents/com.linkterm.agent.plist"

echo "=== LinkTerm Agent Installer ==="
echo ""

mkdir -p "$INSTALL_DIR"

if [ ! -f "$BIN_PATH" ]; then
    echo "Please place the linkterm-agent binary at: $BIN_PATH"
    exit 1
fi

chmod +x "$BIN_PATH"

if [ ! -f "$CONFIG_PATH" ]; then
    cat > "$CONFIG_PATH" << 'YAML'
servers:
  - url: "wss://your-server.com"
    name: "主节点"

token: "CHANGE-ME"
auto_connect: true
prevent_sleep: true
reconnect_max_interval: 30
local_buffer_size: 131072
max_sessions: 10
YAML
    echo "Config created at: $CONFIG_PATH"
    echo "  Please edit it with your server URL and token."
    echo ""
fi

read -p "Install as login item (auto-start on login)? [y/N] " AUTOSTART
if [ "$AUTOSTART" = "y" ] || [ "$AUTOSTART" = "Y" ]; then
    cat > "$PLIST_PATH" << PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.linkterm.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>${BIN_PATH}</string>
        <string>-config</string>
        <string>${CONFIG_PATH}</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>${INSTALL_DIR}/agent.log</string>
    <key>StandardErrorPath</key>
    <string>${INSTALL_DIR}/agent.log</string>
</dict>
</plist>
PLIST
    launchctl load "$PLIST_PATH" 2>/dev/null || true
    echo "Agent installed as login item."
    echo "  To start:   launchctl load $PLIST_PATH"
    echo "  To stop:    launchctl unload $PLIST_PATH"
    echo "  Logs at:    $INSTALL_DIR/agent.log"
else
    echo "To run manually: $BIN_PATH -config $CONFIG_PATH"
fi

echo ""
echo "Done!"
