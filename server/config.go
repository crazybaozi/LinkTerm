package main

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen    string          `yaml:"listen"`
	Region    string          `yaml:"region"`
	TLS       TLSConfig       `yaml:"tls"`
	Auth      AuthConfig      `yaml:"auth"`
	Session   SessionConfig   `yaml:"session"`
	Heartbeat HeartbeatConfig `yaml:"heartbeat"`
}

type TLSConfig struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

type AuthConfig struct {
	JWTSecret string `yaml:"jwt_secret"`
	DataDir   string `yaml:"data_dir"`
}

type SessionConfig struct {
	MaxPerAgent       int `yaml:"max_per_agent"`
	BufferSize        int `yaml:"buffer_size"`
	OrphanCleanupDays int `yaml:"orphan_cleanup_days"`
}

type HeartbeatConfig struct {
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Listen: ":8080",
		Region: "default",
		Auth: AuthConfig{
			DataDir: "./data",
		},
		Session: SessionConfig{
			MaxPerAgent:       10,
			BufferSize:        65536,
			OrphanCleanupDays: 7,
		},
		Heartbeat: HeartbeatConfig{
			Interval: 15 * time.Second,
			Timeout:  45 * time.Second,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.Auth.DataDir == "" {
		cfg.Auth.DataDir = "./data"
	}
	return cfg, nil
}

/** DefaultConfig 返回一个用于开发环境的默认配置 */
func DefaultConfig() *Config {
	return &Config{
		Listen: ":8080",
		Region: "local",
		Auth: AuthConfig{
			DataDir: "./data",
		},
		Session: SessionConfig{
			MaxPerAgent:       10,
			BufferSize:        65536,
			OrphanCleanupDays: 7,
		},
		Heartbeat: HeartbeatConfig{
			Interval: 15 * time.Second,
			Timeout:  45 * time.Second,
		},
	}
}
