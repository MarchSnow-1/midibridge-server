package main

import (
golog "github.com/donnie4w/go-logger/logger"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// maxBodySize HTTP 请求体的最大大小（10KB），防止内存攻击。
const maxBodySize = 10 * 1024

// rateLimiter 基于滑动窗口的 IP 速率限制器。
// 每个 IP 在配置的时间窗口内最多允许 maxReqs 次请求，超出后整个窗口期间被封锁。
type rateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rateEntry // IP → 该 IP 的速率统计条目
}

// rateEntry 记录单个 IP 的请求统计和封锁状态。
type rateEntry struct {
	windowStart  time.Time // 当前窗口的起始时间
	count        int       // 当前窗口内的请求计数
	blockedUntil time.Time // 封锁到期时间（零值表示未封锁）
}

// newRateLimiter 创建一个新的速率限制器，并启动后台清理协程。
// 清理协程每 60 秒运行一次，删除超过 120 秒未活动且已解除封锁的条目。
func newRateLimiter() *rateLimiter {
	rl := &rateLimiter{
		entries: make(map[string]*rateEntry),
	}
	// 定期清理过期条目，避免内存泄漏
	go func() {
		for {
			time.Sleep(60 * time.Second)
			rl.cleanup()
		}
	}()
	return rl
}

// isLimited 检查指定 IP 是否被速率限制。返回 true 表示该请求应被拒绝。
// 如果 IP 还未建立条目或窗口已过期，会创建新的统计窗口。
func (rl *rateLimiter) isLimited(ip string, windowMs, maxReqs int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	window := time.Duration(windowMs) * time.Millisecond

	entry, ok := rl.entries[ip]

	// 如果该 IP 仍在封锁期内，直接拒绝
	if ok && entry.blockedUntil.After(now) {
		return true
	}

	// 新的 IP 或窗口已过期：创建新的统计窗口
	if !ok || now.Sub(entry.windowStart) > window {
		rl.entries[ip] = &rateEntry{
			windowStart: now,
			count:       1,
		}
		return false
	}

	// 窗口内计数递增，超出上限则封锁
	entry.count++
	if entry.count > maxReqs {
		entry.blockedUntil = now.Add(window)
		golog.Warn("Rate limit exceeded for " + ip)
		return true
	}

	return false
}

// cleanup 删除已过期且已解除封锁的条目，防止 entries map 无限增长。
func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := time.Now().Add(-120 * time.Second)
	for ip, e := range rl.entries {
		if e.windowStart.Before(cutoff) && e.blockedUntil.Before(time.Now()) {
			delete(rl.entries, ip)
		}
	}
}

// AdminServer 提供 HTTP 管理 API，包括状态查询和密码修改。
// 内建 IP 白名单、速率限制和请求体大小限制等安全措施。
type AdminServer struct {
	cfg        *Config
	wsServer   *WSServer   // 用于获取客户端数和踢出客户端
	midiReader *MidiReader // 用于查询 MIDI 连接状态
	limiter    *rateLimiter
	httpServer *http.Server
}

// NewAdminServer 创建一个新的 AdminServer 实例。
func NewAdminServer(cfg *Config, ws *WSServer, mr *MidiReader) *AdminServer {
	return &AdminServer{
		cfg:        cfg,
		wsServer:   ws,
		midiReader: mr,
		limiter:    newRateLimiter(),
	}
}

// Start 启动 HTTP 管理服务器，注册路由并开始监听。
func (a *AdminServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/status", a.handleStatus)
	mux.HandleFunc("/admin/change-password", a.handleChangePassword)

	a.httpServer = &http.Server{
		Addr:    a.cfg.Network.Bind + ":" + itoa(a.cfg.Admin.Port),
		Handler: mux,
	}

	go func() {
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			golog.Error("HTTP admin server error: " + err.Error())
		}
	}()

	return nil
}

// Stop 关闭 HTTP 管理服务器。
func (a *AdminServer) Stop() {
	a.httpServer.Close()
}

// clientIP 从 http.Request 中提取客户端真实 IP，去除端口号。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// handleStatus 返回服务端运行状态，包括在线客户端数、MIDI 连接状态
// 和密码最后修改时间。仅接受 GET 请求，受 IP 白名单约束。
func (a *AdminServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}

	ip := clientIP(r)
	// IP 白名单检查
	if !isAllowed(ip, a.cfg.Admin.AllowedIPs) {
		writeJSON(w, 403, map[string]string{"error": "Forbidden"})
		return
	}

	updatedAt := a.cfg.Auth.UpdatedAt
	if updatedAt == "" {
		updatedAt = "N/A"
	}

	writeJSON(w, 200, map[string]interface{}{
		"status":              "running",
		"connectedClients":    a.wsServer.ClientCount(),
		"midiConnected":       a.midiReader.IsConnected(),
		"passwordLastUpdated": updatedAt,
	})
}

// handleChangePassword 处理密码修改请求。
// 安全流程：IP 白名单 → 速率限制 → 请求体大小限制 → JSON 解析 → 旧密码验证 → 新密码更新。
// 密码修改成功后，会踢出所有已认证的 WebSocket 客户端并断开它们的连接。
func (a *AdminServer) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}

	if r.Method != "POST" {
		writeJSON(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}

	ip := clientIP(r)

	// 1. IP 白名单
	if !isAllowed(ip, a.cfg.Admin.AllowedIPs) {
		writeJSON(w, 403, map[string]string{"error": "Forbidden"})
		return
	}

	// 2. 速率限制
	if a.limiter.isLimited(ip, a.cfg.Admin.RateLimitWindowMs, a.cfg.Admin.RateLimitMaxReqs) {
		writeJSON(w, 429, map[string]string{"error": "Too many requests. Try again later."})
		return
	}

	// 3. 限制 body 大小（10KB）
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, 400, map[string]interface{}{
			"success": false,
			"error":   "Request body too large or unreadable",
		})
		return
	}

	// 4. 解析 JSON
	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, 400, map[string]interface{}{
			"success": false,
			"error":   "Invalid JSON",
		})
		return
	}

	// 5. 字段必填校验
	if req.OldPassword == "" || req.NewPassword == "" {
		writeJSON(w, 400, map[string]interface{}{
			"success": false,
			"error":   "Both oldPassword and newPassword are required",
		})
		return
	}

	// 6. 执行密码修改
	err = changePassword(a.cfg, req.OldPassword, req.NewPassword)
	if err != nil {
		status := 400
		if err == errOldPasswordIncorrect {
			status = 403 // 旧密码错误返回 403
		}
		writeJSON(w, status, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// 7. 密码已更改，踢出所有客户端强制重新认证
	a.wsServer.KickAllClients(kickPasswordChanged)

	writeJSON(w, 200, map[string]interface{}{
		"success": true,
		"message": "Password changed successfully",
	})
}

// setCORS 设置跨域响应头和 Content-Type，使管理 API 可以从浏览器访问。
func setCORS(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// writeJSON 以指定 HTTP 状态码返回 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
