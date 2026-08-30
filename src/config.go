package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	golog "github.com/donnie4w/go-logger/logger"
	"golang.org/x/crypto/bcrypt"
)

// initialPasswordLength 首次运行时生成的随机初始密码长度。
const initialPasswordLength = 16

// generateInitialPassword 使用加密安全随机数（crypto/rand）生成初始密码。
// 字符集排除了易混淆字符（0/O、1/l/I）。生成的密码仅在首次启动时打印一次到控制台，
// 不落日志文件、配置文件中只保存其 bcrypt 哈希。
func generateInitialPassword() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	buf := make([]byte, initialPasswordLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate random password: %w", err)
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf), nil
}

// Config 是服务端全部配置的顶层结构，所有子配置直接嵌入 JSON 的顶层字段。
// 内部包含一个读写锁和文件路径，用于线程安全的持久化操作。
type Config struct {
	WS      WSConfig      `json:"ws"`
	Admin   AdminConfig   `json:"admin"`
	Auth    AuthConfig    `json:"auth"`
	MIDI    MIDIConfig    `json:"midi"`
	Logging LoggingConfig `json:"logging"`
	Network NetworkConfig `json:"network"`
	TLS     TLSConfig     `json:"tls"`

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

// NetworkConfig 网络监听配置
type NetworkConfig struct {
	Bind string `json:"bind"` // 监听地址。为空表示监听所有网络接口
}

// TLSConfig TLS 证书配置。两个路径同时配置时启用 TLS
//（WebSocket 端口升级为 wss、管理端口升级为 https），任一为空则保持明文。
type TLSConfig struct {
	Cert string `json:"cert"` // 证书文件路径（PEM）
	Key  string `json:"key"`  // 私钥文件路径（PEM）
}

// TLSEnabled 返回是否同时配置了证书与私钥（即是否启用 TLS）。
func (c *Config) TLSEnabled() bool {
	return c.TLS.Cert != "" && c.TLS.Key != ""
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
		Network: NetworkConfig{
			Bind: "",
		},
		TLS: TLSConfig{
			Cert: "",
			Key:  "",
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
		// 首次运行：生成随机初始密码并保存其哈希
		golog.Info("First run, generating default config with a random initial password...")
		initialPassword, err := generateInitialPassword()
		if err != nil {
			return nil, err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(initialPassword), 10)
		if err != nil {
			return nil, fmt.Errorf("failed to hash initial password: %w", err)
		}
		cfg.Auth.PasswordHash = string(hash)
		cfg.Auth.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := cfg.save(); err != nil {
			return nil, err
		}
		// 初始密码只打印到控制台一次（不走日志系统，避免落入日志文件）。
		fmt.Println("==============================================================")
		fmt.Println("  INITIAL ADMIN PASSWORD (printed only once):")
		fmt.Println("  " + initialPassword)
		fmt.Println("  Change it immediately via /admin/change-password")
		fmt.Println("==============================================================")
		golog.Warn("A random initial password was generated and printed to the console. Change it immediately.")
		return &cfg, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("corrupted config file, delete it and restart: %w", err)
	}

	// 检测未知顶层键（如拼错的 allowedIps），仅告警不拒绝加载——
	// 避免安全关键配置因拼写错误而静默失效
	var rawKeys map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawKeys); err == nil {
		known := map[string]bool{"ws": true, "admin": true, "auth": true, "midi": true, "logging": true, "network": true, "tls": true}
		for k := range rawKeys {
			if !known[k] {
				golog.Warn("Unknown config key \"" + k + "\" ignored — check for typos")
			}
		}
	}

	// 校验并纠正非法配置值
	cfg.validate()

	golog.Info("Config loaded")
	return &cfg, nil
}

// validate 校验加载后的配置值。非法值回退为默认值并记录告警，
// 防止畸形配置（如 rateLimitWindowMs<=0）静默禁用速率限制等安全机制。
func (c *Config) validate() {
	if c.WS.Port < 1 || c.WS.Port > 65535 {
		golog.Warn(fmt.Sprintf("Invalid ws.port %d, falling back to 9001", c.WS.Port))
		c.WS.Port = 9001
	}
	if c.Admin.Port < 1 || c.Admin.Port > 65535 {
		golog.Warn(fmt.Sprintf("Invalid admin.port %d, falling back to 9002", c.Admin.Port))
		c.Admin.Port = 9002
	}
	if c.Admin.RateLimitWindowMs <= 0 {
		golog.Warn(fmt.Sprintf("Invalid admin.rateLimitWindowMs %d, falling back to 60000", c.Admin.RateLimitWindowMs))
		c.Admin.RateLimitWindowMs = 60000
	}
	if c.Admin.RateLimitMaxReqs <= 0 {
		golog.Warn(fmt.Sprintf("Invalid admin.rateLimitMaxRequests %d, falling back to 5", c.Admin.RateLimitMaxReqs))
		c.Admin.RateLimitMaxReqs = 5
	}
	if c.MIDI.ReconnectIntervalMs <= 0 {
		golog.Warn(fmt.Sprintf("Invalid midi.reconnectIntervalMs %d, falling back to 3000", c.MIDI.ReconnectIntervalMs))
		c.MIDI.ReconnectIntervalMs = 3000
	}
	// TLS 证书与私钥必须成对配置，只配其一视为无效并告警
	if (c.TLS.Cert == "") != (c.TLS.Key == "") {
		golog.Warn("tls.cert and tls.key must be configured together — TLS disabled")
		c.TLS.Cert = ""
		c.TLS.Key = ""
	}
}

// save 将当前配置以缩进格式写回磁盘。首次保存时自动创建 data 目录。
// 安全性与健壮性：
//   - 目录权限 0700、文件权限 0600（内含密码的 bcrypt 哈希，
//     防止同机其他用户读取后离线爆破）
//   - 先写临时文件再 rename 的原子替换：崩溃/断电不会留下半截 JSON
//     导致下次启动无法加载配置
func (c *Config) save() error {
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
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

// GetAuthSnapshot 在读锁下返回密码哈希与更新时间的快照。
// 所有读取方必须经由本接口，不得直接访问 Auth 字段，
// 否则与写锁路径构成数据竞争。
func (c *Config) GetAuthSnapshot() (hash, updatedAt string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Auth.PasswordHash, c.Auth.UpdatedAt
}

// setPasswordHashLocked 在调用方已持有写锁时更新哈希并落盘。
// 供 changePassword 在"验证旧密码→写入新哈希"的原子区间内使用，
// 避免嵌套调用 SetPasswordHash 造成死锁。
func (c *Config) setPasswordHashLocked(hash string) error {
	c.Auth.PasswordHash = hash
	c.Auth.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return c.save()
}
