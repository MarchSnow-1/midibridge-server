package main

import (
	"encoding/json"
	"fmt"
	golog "github.com/donnie4w/go-logger/logger"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// defaultPassword 是首次运行时自动生成的初始密码，管理员应尽快修改。
const defaultPassword = "midiBridge123"

// Config 是服务端全部配置的顶层结构，所有子配置直接嵌入 JSON 的顶层字段。
// 内部包含一个读写锁和文件路径，用于线程安全的持久化操作。
type Config struct {
	WS      WSConfig      `json:"ws"`
	Admin   AdminConfig   `json:"admin"`
	Auth    AuthConfig    `json:"auth"`
	MIDI    MIDIConfig    `json:"midi"`
	Logging LoggingConfig `json:"logging"`

	mu   sync.RWMutex // 保护配置字段的并发读写
	path string       // config.json 的绝对路径
}

// WSConfig WebSocket 服务配置
type WSConfig struct {
	Port       int    `json:"port"`       // 监听端口，默认 9001
	AllowedIPs string `json:"allowedIPs"` // IP 白名单，逗号分隔。为空则允许所有 IP 连接
}

// AdminConfig HTTP 管理 API 配置
type AdminConfig struct {
	Port              int    `json:"port"`                 // 监听端口，默认 9002
	AllowedIPs        string `json:"allowedIPs"`           // IP 白名单，仅允许列表内 IP 访问管理端点
	RateLimitWindowMs int    `json:"rateLimitWindowMs"`    // 速率限制时间窗口（毫秒），默认 60000（1 分钟）
	RateLimitMaxReqs  int    `json:"rateLimitMaxRequests"` // 每个时间窗口内允许的最大请求数
}

// AuthConfig 认证相关配置（密码哈希和更新时间）
type AuthConfig struct {
	PasswordHash string `json:"passwordHash"` // bcrypt 哈希后的密码，cost factor = 10
	UpdatedAt    string `json:"updatedAt"`    // 密码最后修改时间（RFC3339 格式）
}

// MIDIConfig MIDI 设备连接配置
type MIDIConfig struct {
	DeviceName          string `json:"deviceName"`          // 目标 MIDI 设备名（子串匹配）。为空则使用首个可用设备
	AutoReconnect       bool   `json:"autoReconnect"`       // 设备断开后是否自动重连
	ReconnectIntervalMs int    `json:"reconnectIntervalMs"` // 重连间隔时间（毫秒），默认 3000
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	File        bool `json:"file"`        // 是否启用文件日志输出，默认 false
	MidiVerbose bool `json:"midiVerbose"` // 是否记录每条 MIDI 按键/控制事件，默认 false
}

// defaultConfig 返回一份带有合理默认值的全新 Config 实例。
// 该函数不涉及任何 I/O 操作，仅构造数据结构。
func defaultConfig() Config {
	return Config{
		WS: WSConfig{
			Port:       9001,
			AllowedIPs: "",
		},
		Admin: AdminConfig{
			Port:              9002,
			AllowedIPs:        "",
			RateLimitWindowMs: 60000,
			RateLimitMaxReqs:  5,
		},
		Auth: AuthConfig{
			PasswordHash: "",
			UpdatedAt:    "",
		},
		MIDI: MIDIConfig{
			DeviceName:          "",
			AutoReconnect:       true,
			ReconnectIntervalMs: 3000,
		},
		Logging: LoggingConfig{
			File:        false,
			MidiVerbose: false,
		},
	}
}

// loadConfig 从磁盘加载配置。若文件不存在（首次运行），自动生成默认配置
// 并使用 bcrypt 加密默认密码后写入磁盘。若文件存在但 JSON 格式错误，
// 会提示用户手动删除损坏的配置文件再重启。
func loadConfig(configPath string) (*Config, error) {
	cfg := defaultConfig()
	cfg.path = configPath

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// 首次运行，生成默认配置
		golog.Info("First run, generating default config...")
		hash, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), 10)
		if err != nil {
			return nil, fmt.Errorf("failed to hash default password: %w", err)
		}
		cfg.Auth.PasswordHash = string(hash)
		cfg.Auth.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := cfg.save(); err != nil {
			return nil, err
		}
		golog.Warn("pls change default password: " + defaultPassword)
		return &cfg, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("corrupted config file, delete it and restart: %w", err)
	}

	golog.Info("Config loaded")
	return &cfg, nil
}

// save 将当前配置以缩进格式写回磁盘。首次保存时自动创建 data 目录。
func (c *Config) save() error {
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.path, data, 0644)
}

// SetPasswordHash 更新密码哈希并立即持久化到磁盘。
// 该方法持有写锁，确保并发修改安全。
func (c *Config) SetPasswordHash(hash string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Auth.PasswordHash = hash
	c.Auth.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return c.save()
}
