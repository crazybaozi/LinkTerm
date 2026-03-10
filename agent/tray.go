package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os/exec"
	"strings"
	"time"

	"fyne.io/systray"
)

/** Tray 管理 macOS 菜单栏图标和操作面板 */
type Tray struct {
	tunnel     *Tunnel
	selector   *Selector
	sessions   *SessionManager
	config     *Config
	configPath string
	guard      *SleepGuard

	mStatus    *systray.MenuItem
	mServer    *systray.MenuItem
	mSessions  *systray.MenuItem
	mToken     *systray.MenuItem
	mURL       *systray.MenuItem
	mReconnect *systray.MenuItem
	mQuit      *systray.MenuItem

	iconConnected    []byte
	iconDisconnected []byte
	iconReconnecting []byte
}

func NewTray(cfg *Config, configPath string) *Tray {
	t := &Tray{
		config:     cfg,
		configPath: configPath,
	}
	t.iconConnected = generateLBoltIcon(color.RGBA{R: 76, G: 175, B: 80, A: 255}, 22)
	t.iconDisconnected = generateLBoltIcon(color.RGBA{R: 244, G: 67, B: 54, A: 255}, 22)
	t.iconReconnecting = generateLBoltIcon(color.RGBA{R: 255, G: 193, B: 7, A: 255}, 22)
	return t
}

/** OnReady systray 就绪回调：构建菜单 + 启动 Agent */
func (t *Tray) OnReady() {
	systray.SetIcon(t.iconDisconnected)
	systray.SetTooltip("LinkTerm Agent")

	t.mStatus = systray.AddMenuItem("⚫ 未连接", "连接状态")
	t.mStatus.Disable()

	t.mServer = systray.AddMenuItem("服务器: -", "当前服务器")
	t.mServer.Disable()

	t.mSessions = systray.AddMenuItem("活跃终端: 0", "终端数量")
	t.mSessions.Disable()

	systray.AddSeparator()

	t.mToken = systray.AddMenuItem("访问本机 Token", "点击复制 Token 到剪贴板")
	t.mURL = systray.AddMenuItem("服务器地址", "点击复制服务器地址到剪贴板")

	systray.AddSeparator()

	t.mReconnect = systray.AddMenuItem("重新连接", "断开并重新连接")

	systray.AddSeparator()

	t.mQuit = systray.AddMenuItem("退出 LinkTerm", "退出应用")

	go t.startAgent()
	go t.handleClicks()
	go t.updateLoop()
}

/** OnExit systray 退出回调：清理资源 */
func (t *Tray) OnExit() {
	log.Println("shutting down...")
	if t.selector != nil {
		t.selector.Stop()
	}
	if t.tunnel != nil {
		t.tunnel.Disconnect()
	}
	if t.guard != nil {
		t.guard.Stop()
	}
}

/** startAgent 初始化并启动 Agent 核心逻辑 */
func (t *Tray) startAgent() {
	shell := DetectShell(t.config.Shell)
	log.Printf("LinkTerm Agent starting (shell=%s)", shell)

	t.sessions = NewSessionManager(shell, t.config.LocalBufferSize)
	t.tunnel = NewTunnel(t.config, t.sessions)

	t.tunnel.SetStatusCallback(func(status TunnelStatus, msg string) {
		log.Printf("[status] %s: %s", status, msg)
		t.updateStatus(status)
	})

	if len(t.config.Servers) == 0 {
		log.Println("no servers configured")
		t.mStatus.SetTitle("⚠️ 未配置服务器")
		return
	}

	if t.config.PreventSleep {
		t.guard = NewSleepGuard()
	}

	t.selector = NewSelector(t.config.Servers, t.tunnel)
	t.tunnel.SetReconnectHandler(t.selector.ReconnectBest)

	if err := t.selector.ConnectBest(); err != nil {
		log.Printf("initial connection failed: %v", err)
		log.Println("will keep trying to reconnect...")
		go t.selector.ReconnectBest()
	}

	t.selector.StartMonitor()

	node := t.selector.CurrentNode()
	if node != nil {
		PrintAccessInfo(node.URL, node.Name)
	} else if len(t.config.Servers) > 0 {
		PrintAccessInfo(t.config.Servers[0].URL, t.config.Servers[0].Name)
	}
}

/** handleClicks 处理菜单项点击事件 */
func (t *Tray) handleClicks() {
	for {
		select {
		case <-t.mToken.ClickedCh:
			if t.config.Token != "" {
				copyToClipboard(t.config.Token)
				t.mToken.SetTitle("✅ Token 已复制")
				go t.resetTitle(t.mToken, "访问本机 Token", 2*time.Second)
			}

		case <-t.mURL.ClickedCh:
			url := t.getAccessURL()
			if url != "" {
				copyToClipboard(url)
				t.mURL.SetTitle("✅ 地址已复制")
				go t.resetTitle(t.mURL, fmt.Sprintf("服务器地址: %s", truncate(url, 24)), 2*time.Second)
			}

		case <-t.mReconnect.ClickedCh:
			if t.tunnel != nil && t.selector != nil {
				go func() {
					t.tunnel.Disconnect()
					t.selector.ReconnectBest()
				}()
			}

		case <-t.mQuit.ClickedCh:
			systray.Quit()
		}
	}
}

/** updateLoop 定期更新活跃终端数 */
func (t *Tray) updateLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if t.sessions != nil {
			count := t.sessions.Count()
			t.mSessions.SetTitle(fmt.Sprintf("活跃终端: %d", count))
		}
	}
}

/** updateStatus 根据隧道状态更新图标和菜单文字 */
func (t *Tray) updateStatus(status TunnelStatus) {
	switch status {
	case StatusConnected:
		systray.SetIcon(t.iconConnected)
		t.mStatus.SetTitle("🟢 已连接")
		if t.selector != nil {
			if node := t.selector.CurrentNode(); node != nil {
				t.mServer.SetTitle(fmt.Sprintf("服务器: %s", node.Name))
			}
		}
		url := t.getAccessURL()
		if url != "" {
			t.mURL.SetTitle(fmt.Sprintf("服务器地址: %s", truncate(url, 24)))
		}

	case StatusDisconnected:
		systray.SetIcon(t.iconDisconnected)
		t.mStatus.SetTitle("🔴 未连接")

	case StatusConnecting:
		systray.SetIcon(t.iconReconnecting)
		t.mStatus.SetTitle("🟡 连接中...")

	case StatusReconnecting:
		systray.SetIcon(t.iconReconnecting)
		t.mStatus.SetTitle("🟡 重连中...")
	}
}

func (t *Tray) getAccessURL() string {
	if t.selector != nil {
		if node := t.selector.CurrentNode(); node != nil {
			return node.URL
		}
	}
	if len(t.config.Servers) > 0 {
		return t.config.Servers[0].URL
	}
	return ""
}

func (t *Tray) resetTitle(item *systray.MenuItem, title string, delay time.Duration) {
	time.Sleep(delay)
	item.SetTitle(title)
}

/** copyToClipboard 复制文本到 macOS 剪贴板 */
func copyToClipboard(text string) {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		log.Printf("[tray] clipboard copy failed: %v", err)
	}
}

/**
 * generateLBoltIcon 生成白色 L + 彩色闪电的菜单栏图标
 * L 用 SDF 圆角矩形绘制（白色），闪电用多边形填充（boltColor 表示连接状态）
 */
func generateLBoltIcon(boltColor color.Color, size int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	s := float64(size)

	// L 形参数
	margin := s * 0.08
	strokeW := s * 0.30
	radius := strokeW * 0.30

	vCx := margin + strokeW/2
	vCy := s / 2
	vHw := strokeW / 2
	vHh := (s - 2*margin) / 2

	hCx := s / 2
	hCy := s - margin - strokeW/2
	hHw := (s - 2*margin) / 2
	hHh := strokeW / 2

	// 闪电多边形（8 个顶点，两段平行笔画 + 阶梯错位构成 ⚡）
	bolt := [][2]float64{
		{s * 0.50, s * 0.09},
		{s * 0.64, s * 0.09},
		{s * 0.45, s * 0.48},
		{s * 0.64, s * 0.48},
		{s * 0.45, s * 0.86},
		{s * 0.32, s * 0.86},
		{s * 0.50, s * 0.48},
		{s * 0.32, s * 0.48},
	}

	br, bg, bb, _ := boltColor.RGBA()
	bcr := uint8(br >> 8)
	bcg := uint8(bg >> 8)
	bcb := uint8(bb >> 8)

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx := float64(x) + 0.5
			fy := float64(y) + 0.5

			// 闪电：2x2 超采样抗锯齿
			boltAlpha := 0.0
			for sy := 0; sy < 2; sy++ {
				for sx := 0; sx < 2; sx++ {
					subX := float64(x) + (float64(sx)+0.5)/2.0
					subY := float64(y) + (float64(sy)+0.5)/2.0
					if pointInPolygon(subX, subY, bolt) {
						boltAlpha += 0.25
					}
				}
			}

			if boltAlpha > 0 {
				img.Set(x, y, color.RGBA{R: bcr, G: bcg, B: bcb, A: uint8(boltAlpha * 255)})
				continue
			}

			// L 形：SDF 抗锯齿
			d1 := sdRoundedRect(fx, fy, vCx, vCy, vHw, vHh, radius)
			d2 := sdRoundedRect(fx, fy, hCx, hCy, hHw, hHh, radius)
			d := math.Min(d1, d2)
			lAlpha := clamp01(0.5 - d)

			if lAlpha > 0 {
				img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: uint8(lAlpha * 255)})
			}
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

/** pointInPolygon 射线法判断点是否在多边形内 */
func pointInPolygon(px, py float64, poly [][2]float64) bool {
	n := len(poly)
	inside := false
	j := n - 1
	for i := 0; i < n; i++ {
		xi, yi := poly[i][0], poly[i][1]
		xj, yj := poly[j][0], poly[j][1]
		if ((yi > py) != (yj > py)) && (px < (xj-xi)*(py-yi)/(yj-yi)+xi) {
			inside = !inside
		}
		j = i
	}
	return inside
}

/** sdRoundedRect 圆角矩形的有符号距离场，负值表示在形状内部 */
func sdRoundedRect(px, py, cx, cy, hw, hh, r float64) float64 {
	dx := math.Abs(px-cx) - hw + r
	dy := math.Abs(py-cy) - hh + r
	outside := math.Sqrt(math.Max(dx, 0)*math.Max(dx, 0)+math.Max(dy, 0)*math.Max(dy, 0)) - r
	inside := math.Min(math.Max(dx, dy), 0.0)
	return outside + inside
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
