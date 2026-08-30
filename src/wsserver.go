package main

import (
golog "github.com/donnie4w/go-logger/logger"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"

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

// upgrader 将 HTTP 连接升级为 WebSocket，允许所有来源的跨域请求。
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WSClient 表示一个已连接的 WebSocket 客户端。
// 每个客户端在认证前处于未授权状态，无法接收 MIDI 消息。
type WSClient struct {
	conn          *websocket.Conn // WebSocket 底层连接
	authenticated bool            // 是否已通过密码认证
	ip            string          // 客户端 IP 地址（不含端口）
	mu            sync.Mutex      // 保护 conn 写入和 authenticated 状态
}

// WSServer 管理 WebSocket 连接的生命周期，包括认证、广播和踢出。
// 内部维护一个已连接客户端的集合（无论是否认证）。
type WSServer struct {
	cfg        *Config                // 服务端配置引用
	clients    map[*WSClient]struct{} // 当前所有连接的客户端集合
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
// 启动是非阻塞的——ListenAndServe 在后台 goroutine 中运行。
func (s *WSServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleConnection)

	s.httpServer = &http.Server{
		Addr:    s.cfg.Network.Bind + ":" + itoa(s.cfg.WS.Port),
		Handler: mux,
	}

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			golog.Error("WebSocket server error: " + err.Error())
		}
	}()

	return nil
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

	client := &WSClient{
		conn:          conn,
		authenticated: false,
		ip:            clientIP,
	}

	golog.Info("New client connected: " + clientIP)

	// 注册客户端
	s.mu.Lock()
	s.clients[client] = struct{}{}
	s.mu.Unlock()

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

	s.readLoop(client, authTimer)
}

// readLoop 循环读取客户端消息，根据消息 type 分发到不同处理函数。
func (s *WSServer) readLoop(client *WSClient, authTimer *time.Timer) {
	defer func() {
		authTimer.Stop()
		client.conn.Close()
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

// Broadcast 将 MIDI 消息广播给所有已认证的客户端。
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
		if client.authenticated {
			authedClients = append(authedClients, client)
		}
		client.mu.Unlock()
	}
	s.mu.Unlock()

	for _, client := range authedClients {
		client.sendRaw(payload)
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
	golog.Info("WebSocket server stopped")
}

// sendJSON 将任意值序列化为 JSON 后发送给该客户端。
func (c *WSClient) sendJSON(v interface{}) {
	data, _ := json.Marshal(v)
	c.sendRaw(data)
}

// sendRaw 向该客户端发送原始字节数据。
func (c *WSClient) sendRaw(data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		golog.Warn("Failed to send to client " + c.ip + " error=" + err.Error())
	}
}
