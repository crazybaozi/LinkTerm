package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	configPath := flag.String("config", "", "config file path")
	flag.Parse()

	var cfg *Config
	if *configPath != "" {
		var err error
		cfg, err = LoadConfig(*configPath)
		if err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
	} else {
		defaultPath := ConfigPath()
		if _, err := os.Stat(defaultPath); err == nil {
			cfg, _ = LoadConfig(defaultPath)
		}
		if cfg == nil {
			cfg = DefaultConfig()
			log.Println("no config file found, using default dev config")
		}
	}

	shell := DetectShell(cfg.Shell)
	log.Printf("LinkTerm Agent starting (shell=%s)", shell)

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
