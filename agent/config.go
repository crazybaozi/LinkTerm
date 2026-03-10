package main

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Servers              []ServerNode `yaml:"servers"`
	Token                string       `yaml:"token"`
	Name                 string       `yaml:"name"`
	Shell                string       `yaml:"shell"`
	AutoConnect          bool         `yaml:"auto_connect"`
	PreventSleep         bool         `yaml:"prevent_sleep"`
	ReconnectMaxInterval int          `yaml:"reconnect_max_interval"`
	LocalBufferSize      int          `yaml:"local_buffer_size"`
	MaxSessions          int          `yaml:"max_sessions"`
}

type ServerNode struct {
	URL  string `yaml:"url"`
	Name string `yaml:"name"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if cfg.Token == "" {
		cfg.Token = GenerateToken()
		log.Printf("[config] generated new token: %s", cfg.Token)
		if err := SaveConfig(path, cfg); err != nil {
			log.Printf("[config] warning: failed to save generated token: %v", err)
		}
	}

	if cfg.Name == "" {
		hostname, _ := os.Hostname()
		if hostname != "" {
			cfg.Name = hostname
		} else {
			cfg.Name = "Mac"
		}
	}

	return cfg, nil
}

func DefaultConfig() *Config {
	return &Config{
		Servers: []ServerNode{
			{URL: "ws://127.0.0.1:8080", Name: "local"},
		},
		AutoConnect:          true,
		PreventSleep:         false,
		ReconnectMaxInterval: 30,
		LocalBufferSize:      131072,
		MaxSessions:          10,
	}
}

/** ConfigPath 返回默认配置文件路径 ~/.linkterm/config.yaml */
func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".linkterm", "config.yaml")
}

/** DetectShell 获取用户默认 shell */
func DetectShell(configured string) string {
	if configured != "" {
		return configured
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/zsh"
}

/** GenerateToken 生成随机 Agent Token（lt_ 前缀 + 32字节 hex） */
func GenerateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return "lt_" + hex.EncodeToString(b)
}

/** SaveConfig 将配置写回 YAML 文件 */
func SaveConfig(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

/** RegenerateToken 生成新 token 并保存到配置文件 */
func RegenerateToken(path string, cfg *Config) string {
	cfg.Token = GenerateToken()
	if err := SaveConfig(path, cfg); err != nil {
		log.Printf("[config] warning: failed to save regenerated token: %v", err)
	}
	log.Printf("[config] token regenerated: %s", cfg.Token)
	return cfg.Token
}
