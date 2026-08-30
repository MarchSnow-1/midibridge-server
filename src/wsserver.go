package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	golog "github.com/donnie4w/go-logger/logger"
	"github.com/gorilla/websocket"
)

// 踢出原因常量
const (
	kickAuthTimeout     = "auth_timeout"
	kickServerShutdown  = "server_shutdown"
	kickPasswordChanged = "password_changed"
)

// authTimeout 定义新连接必须在多长时间内完成认证，超时未认证将被踢出。
const authTimeout = 5 * time.Second

// maxFrameSize 限制单个 WebSocket 帧的最大字节数。
// 协议仅有 auth/ping 等小消息，64KB 绰绰有余；
// 防止恶意/异常客户端发送超大帧导致内存耗尽（gorilla 默认不限）。
const maxFrameSize = 64 * 1024

// wsPingInterval 服务端主动发送协议层 ping 的间隔。
const wsPingInterval = 30 * time.Second

// wsReadTimeout 读超时：超过该时间未收到任何帧（数据或 pong）即判定连接已死。
const wsReadTimeout = 60 * time.Second

// upgrader 将 HTTP 连接升级为 WebSocket，允许所有来源的跨域请求。
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WSClient 表示一个已连接的 WebSocket 客户端。
// 每个客户端在认证前处于未授权状态，无法接收 MIDI 消息。
type WSClient struct {
	conn          *websocket.Conn // WebSocket 底层连接
	authenticated bool            // 是否已通过密码认证
	closed        bool            // 连接是否已进入清理流程（防止向已关闭的 send 队列投递）
	ip            string          // 客户端 IP 地址（不含端口）
	send          chan []byte     // 广播消息发送队列（有界、单写协程消费以保证顺序）
	mu            sync.Mutex      // 保护 conn 写入和 authenticated/closed 状态
}

// sendQueueSize 每个客户端的发送队列容量。
// 满时丢弃最旧策略不可行（需保序），因此直接丢弃新帧并计数告警。
const sendQueueSize = 256

// WSServer 管理 WebSocket 连接的生命周期，包括认证、广播和踢出。
// 内部维护一个已连接客户端的集合（无论是否认证）。
type WSServer struct {
	cfg        *Config                // 服务端配置引用
	clients    map[*WSClient]struct{} // 当前所有连接的客户端集合
	dropped    int64                  // 广播丢弃帧计数（atomic，由 dropMonitor 周期汇总）
	mu         sync.Mutex             // 保护 clients 的并发访问
	httpServer *http.Server           // 底层 HTTP 服务器
	done       chan struct{}          // 停止信号
}

// NewWSServer 创建一个新的 WSServer 实例。需调用 Start() 才开始监听。
func NewWSServer(cfg *Config) *WSServer {
	return &WSServer{
		cfg:     cfg,
		clients: make(map[*WSClient]struct{}),
		done:    make(chan struct{}),
	}
}

// Start 启动 WebSocket 服务器，在 cfg.WS.Port 上监听。
// 监听器在本方法内同步创建，失败时立即返回错误（避免监听失败被后台
// goroutine 静默吞掉、进程"假活"）。监听建立后由后台 goroutine 提供连接。
func (s *WSServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleConnection)

	addr := s.cfg.Network.Bind + ":" + itoa(s.cfg.WS.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen WebSocket on %s: %w", addr, err)
	}

	// 启用 TLS 前先同步验证证书可用，避免监听后异步失败导致"假活"
	useTLS := s.cfg.TLSEnabled()
	if useTLS {
		if _, err := tls.LoadX509KeyPair(s.cfg.TLS.Cert, s.cfg.TLS.Key); err != nil {
			ln.Close()
			return fmt.Errorf("invalid TLS cert/key for WebSocket server: %w", err)
		}
	}

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		var serveErr error
		if useTLS {
			serveErr = s.httpServer.ServeTLS(ln, s.cfg.TLS.Cert, s.cfg.TLS.Key)
		} else {
			serveErr = s.httpServer.Serve(ln)
		}
		if serveErr != nil && serveErr != http.ErrServerClosed {
			golog.Error("WebSocket server error: " + serveErr.Error())
		}
	}()

	// 丢弃计数监控：使慢消费者导致的丢帧可观测
	go s.dropMonitor()

	return nil
}

// dropMonitor 周期性汇总广播丢弃计数。若无此监控，慢消费者导致的
// MIDI 丢帧将完全不可见（两级静默丢弃）。
func (s *WSServer) dropMonitor() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			if n := atomic.SwapInt64(&s.dropped, 0); n > 0 {
				golog.Warn(fmt.Sprintf("Dropped %d MIDI broadcast frame(s) in the last 5s (slow client queues full)", n))
			}
		}
	}
}

// writePump 是该客户端的唯一帧写入协程，按入队顺序串行发送，
// 保证每个客户端收到的消息有序；队列在 readLoop 清理时关闭使本协程退出。
func (s *WSServer) writePump(client *WSClient) {
	for payload := range client.send {
		client.sendRaw(payload)
	}
}

// handleConnection 处理每个新进的 HTTP 请求。
// 流程：IP 白名单检查 → WebSocket 升级 → 注册客户端 → 启动认证超时计时器 → 进入消息读取循环。
func (s *WSServer) handleConnection(w http.ResponseWriter, r *http.Request) {
	clientIP := r.RemoteAddr
	// 去掉端口号，只保留 IP
	if host, _, err := net.SplitHostPort(clientIP); err == nil {
		clientIP = host
	}

	// IP 白名单检查——未通过则直接返回 403
	if !isAllowed(clientIP, s.cfg.WS.AllowedIPs) {
		golog.Warn("Rejected client " + clientIP + " (not in allowlist)")
		http.Error(w, "Forbidden", 403)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		golog.Warn("WebSocket upgrade failed ip=" + clientIP + " error=" + err.Error())
		return
	}
	// 限制单帧大小，防止超大帧耗尽内存（必须在读取任何消息前设置）
	conn.SetReadLimit(maxFrameSize)

	// 读超时 + pong 续期：半开/死亡连接将在读超时后被 ReadMessage 检出并清理
	conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
		return nil
	})

	client := &WSClient{
		conn:          conn,
		authenticated: false,
		ip:            clientIP,
		send:          make(chan []byte, sendQueueSize),
	}

	golog.Info("New client connected: " + clientIP)

	// 注册客户端
	s.mu.Lock()
	s.clients[client] = struct{}{}
	s.mu.Unlock()

	// 发送协程：单写者消费发送队列，保证该客户端收到的消息有序
	go s.writePump(client)

	// 心跳协程：周期性发送协议层 ping，发送失败即关闭死连接
	stopHeartbeat := make(chan struct{})
	go s.heartbeatLoop(client, stopHeartbeat)

	// 认证超时定时器：5 秒内未完成认证则断开连接
	authTimer := time.AfterFunc(authTimeout, func() {
		client.mu.Lock()
		authed := client.authenticated
		client.mu.Unlock()
		if !authed {
			golog.Warn("Client " + clientIP + " auth timed out")
			client.sendJSON(map[string]interface{}{
				"type":   "kicked",
				"reason": kickAuthTimeout,
			})
			client.conn.Close()
			s.removeClient(client)
		}
	})

	s.readLoop(client, authTimer, stopHeartbeat)
}

// heartbeatLoop 周期性向客户端发送 WebSocket 协议层 ping。
// 若发送失败（对端已死/半开连接），主动关闭连接使 readLoop 退出并完成清理。
// WriteControl 按 gorilla 约定可与 WriteMessage 并发，且自带截止时间参数。
func (s *WSServer) heartbeatLoop(client *WSClient, stop chan struct{}) {
	ticker := time.NewTicker(wsPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			client.mu.Lock()
			conn := client.conn
			client.mu.Unlock()
			if conn == nil {
				return
			}
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				golog.Warn("Ping failed, closing dead connection ip=" + client.ip + " error=" + err.Error())
				conn.Close()
				return
			}
		}
	}
}

// readLoop 循环读取客户端消息，根据消息 type 分发到不同处理函数。
func (s *WSServer) readLoop(client *WSClient, authTimer *time.Timer, stopHeartbeat chan struct{}) {
	defer func() {
		authTimer.Stop()
		close(stopHeartbeat)
		client.conn.Close()
		// 标记关闭并关闭发送队列，使 writePump 退出且后续投递被跳过
		client.mu.Lock()
		if !client.closed {
			client.closed = true
			close(client.send)
		}
		client.mu.Unlock()
		s.removeClient(client)
	}()

	for {
		_, rawMsg, err := client.conn.ReadMessage()
		if err != nil {
			client.mu.Lock()
			authed := client.authenticated
			client.mu.Unlock()
			if authed {
				golog.Info("Client disconnected: " + client.ip + " error=" + err.Error())
			}
			return
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			golog.Warn("Failed to parse message from " + client.ip + " error=" + err.Error())
			continue
		}

		msgType, _ := msg["type"].(string)

		switch msgType {
		case "auth":
			s.handleAuth(client, msg, authTimer)

		case "ping":
			client.sendJSON(map[string]string{"type": "pong"})

		default:
			golog.Warn("Unknown message type " + msgType + " from " + client.ip)
		}
	}
}

// handleAuth 处理客户端认证消息。
func (s *WSServer) handleAuth(client *WSClient, msg map[string]interface{}, authTimer *time.Timer) {
	client.mu.Lock()
	if client.authenticated {
		client.mu.Unlock()
		client.sendJSON(map[string]string{"type": "auth_ok"})
		return
	}
	client.mu.Unlock()

	password, _ := msg["password"].(string)
	if verifyPassword(s.cfg.Auth.PasswordHash, password) {
		client.mu.Lock()
		client.authenticated = true
		client.mu.Unlock()
		authTimer.Stop()
		client.sendJSON(map[string]string{"type": "auth_ok"})
		golog.Info("Client authenticated: " + client.ip)
	} else {
		golog.Warn("Client auth failed: " + client.ip)
		client.sendJSON(map[string]interface{}{
			"type":   "auth_fail",
			"reason": "Incorrect password",
		})
		client.conn.Close()
	}
}

// removeClient 将客户端从连接集合中移除。
func (s *WSServer) removeClient(client *WSClient) {
	s.mu.Lock()
	delete(s.clients, client)
	s.mu.Unlock()
}

// Broadcast 将 MIDI 消息非阻塞地投递给所有已认证客户端的发送队列。
// 每个客户端由独立的 writePump 串行消费队列：单个慢/死客户端不再阻塞
// 其他客户端与 MIDI 读取链；队列满时丢弃并计数（dropMonitor 周期告警）。
func (s *WSServer) Broadcast(data MidiMessage) {
	payload, _ := json.Marshal(map[string]interface{}{
		"type": "midi",
		"data": map[string]interface{}{
			"t": data.DeltaTime,
			"m": data.Data,
		},
	})

	s.mu.Lock()
	authedClients := make([]*WSClient, 0, len(s.clients))
	for client := range s.clients {
		client.mu.Lock()
		if client.authenticated && !client.closed {
			authedClients = append(authedClients, client)
		}
		client.mu.Unlock()
	}
	s.mu.Unlock()

	for _, client := range authedClients {
		client.mu.Lock()
		if !client.closed {
			select {
			case client.send <- payload:
			default:
				// 该客户端队列已满（慢消费者）：丢弃本帧并计数，防止背压拖垮全局
				atomic.AddInt64(&s.dropped, 1)
			}
		}
		client.mu.Unlock()
	}
}

// KickAllClients 踢出当前所有连接的客户端。
// 流程：发送 kicked 消息 → 关闭所有连接 → 清空客户端列表。
func (s *WSServer) KickAllClients(reason string) {
	s.mu.Lock()
	clients := make([]*WSClient, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.mu.Unlock()

	kickMsg, _ := json.Marshal(map[string]interface{}{
		"type":   "kicked",
		"reason": reason,
	})

	// 发 kicked 通知并关闭连接
	for _, client := range clients {
		client.sendRaw(kickMsg)
		client.conn.Close()
	}

	s.mu.Lock()
	s.clients = make(map[*WSClient]struct{})
	s.mu.Unlock()

	golog.Info("Kicked all authenticated clients (" + reason + ")")
}

// ClientCount 返回当前连接的客户端总数（包括未认证的）。
func (s *WSServer) ClientCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients)
}

// Stop 优雅关闭 WebSocket 服务器：先踢出所有客户端，再关闭 HTTP 监听器。
func (s *WSServer) Stop() {
	s.KickAllClients(kickServerShutdown)
	s.httpServer.Close()
	close(s.done)
	golog.Info("WebSocket server stopped")
}

// sendJSON 将任意值序列化为 JSON 后发送给该客户端。
func (c *WSClient) sendJSON(v interface{}) {
	data, _ := json.Marshal(v)
	c.sendRaw(data)
}

// sendRaw 向该客户端发送原始字节数据。
// 写入失败（对端断开/超时）时立即关闭连接，唤醒阻塞中的 ReadMessage，
// 让 readLoop 的 defer 统一完成客户端移除——死客户端不再滞留。
func (c *WSClient) sendRaw(data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		golog.Warn("Failed to send to client " + c.ip + " error=" + err.Error() + " — closing connection")
		c.conn.Close()
	}
}
