package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"fyne.io/systray"
)

func main() {
	configPath := flag.String("config", "", "config file path")
	headless := flag.Bool("no-tray", false, "run without menu bar icon (headless mode)")
	flag.Parse()

	cfg, cfgPath := loadConfiguration(*configPath)

	if *headless {
		runHeadless(cfg)
		return
	}

	tray := NewTray(cfg, cfgPath)
	systray.Run(tray.OnReady, tray.OnExit)
}

/** loadConfiguration 加载配置文件，按优先级：命令行参数 > 默认路径 > 内置默认值 */
func loadConfiguration(configPath string) (*Config, string) {
	if configPath != "" {
		cfg, err := LoadConfig(configPath)
		if err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
		return cfg, configPath
	}

	defaultPath := ConfigPath()
	if _, err := os.Stat(defaultPath); err == nil {
		cfg, err := LoadConfig(defaultPath)
		if err == nil && cfg != nil {
			return cfg, defaultPath
		}
	}

	cfg := DefaultConfig()
	log.Println("no config file found, using default dev config")
	return cfg, defaultPath
}

/** runHeadless 无菜单栏图标模式运行（通过 --no-tray 启用） */
func runHeadless(cfg *Config) {
	shell := DetectShell(cfg.Shell)
	log.Printf("LinkTerm Agent starting in headless mode (shell=%s)", shell)

	sessions := NewSessionManager(shell, cfg.LocalBufferSize)
	tunnel := NewTunnel(cfg, sessions)

	tunnel.SetStatusCallback(func(status TunnelStatus, msg string) {
		log.Printf("[status] %s: %s", status, msg)
	})

	if len(cfg.Servers) == 0 {
		log.Fatal("no servers configured")
	}

	var guard *SleepGuard
	if cfg.PreventSleep {
		guard = NewSleepGuard()
	}

	selector := NewSelector(cfg.Servers, tunnel)
	tunnel.SetReconnectHandler(selector.ReconnectBest)

	if err := selector.ConnectBest(); err != nil {
		log.Printf("initial connection failed: %v", err)
		log.Println("will keep trying to reconnect...")
		go selector.ReconnectBest()
	}

	selector.StartMonitor()

	node := selector.CurrentNode()
	if node != nil {
		PrintAccessInfo(node.URL, node.Name)
	} else {
		PrintAccessInfo(cfg.Servers[0].URL, cfg.Servers[0].Name)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("shutting down...")
	selector.Stop()
	tunnel.Disconnect()
	if guard != nil {
		guard.Stop()
	}
}
