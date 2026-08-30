package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	golog "github.com/donnie4w/go-logger/logger"
)

// version 在编译时通过 ldflags 注入，默认 "dev" 表示开发构建。
// 构建命令: go build -ldflags "-X main.version=v1.2.3" ./src/
var version = "dev"

// main 是 MIDI Bridge Server 的入口函数。启动流程：
//  1. 初始化控制台日志
//  2. 加载配置文件（首次运行自动生成默认配置）
//  3. 根据需要启用文件日志
//  4. 创建三大核心模块：MIDI 读取器、WebSocket 服务器、HTTP 管理 API
//  5. 启动各模块并搭建事件总线（MIDI 消息 → WebSocket 广播）
//  6. 等待 SIGINT/SIGTERM 信号后优雅关闭
func main() {
	// 0. 版本标志：--version 或 -v 打印版本后退出
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Println("midibridge-server", version)
			return
		}
	}

	// 1. 初始化日志（先仅控制台）
	initLogger(false)

	golog.Info("MIDIBridge Server " + version + " starting...")

	// 2. 加载配置
	configPath := filepath.Join(".", "data", "config.json")
	cfg, err := loadConfig(configPath)
	if err != nil {
		golog.Error("Startup failed: " + err.Error())
		os.Exit(1)
	}

	// 3. 按需启用文件日志
	if cfg.Logging.File {
		enableFileLogging()
	}

	// 3.5 暴露面警示：监听所有接口或白名单为空时，显著提醒管理员
	bind := cfg.Network.Bind
	if bind == "" || bind == "0.0.0.0" || bind == "::" {
		if cfg.TLSEnabled() {
			golog.Warn("Listening on ALL network interfaces (TLS enabled)")
		} else {
			golog.Warn("Listening on ALL network interfaces (transport is plaintext) — do not expose to untrusted networks")
		}
	}
	if cfg.WS.AllowedIPs == "" {
		golog.Warn("ws.allowedIPs is empty: ANY IP may connect to the WebSocket port")
	}
	if cfg.Admin.AllowedIPs == "" {
		golog.Warn("admin.allowedIPs is empty: the admin API is reachable from ANY IP — strongly consider setting an allowlist")
	}

	// 4. 创建各模块
	midiReader := NewMidiReader()
	wsServer := NewWSServer(cfg)
	adminServer := NewAdminServer(cfg, wsServer, midiReader)

	// 5. 启动（任一监听失败立即退出，绝不"假活"）
	if err := adminServer.Start(); err != nil {
		golog.Error("Startup failed: " + err.Error())
		os.Exit(1)
	}
	if err := wsServer.Start(); err != nil {
		golog.Error("Startup failed: " + err.Error())
		os.Exit(1)
	}
	midiReader.Start(cfg.MIDI.DeviceName, cfg.MIDI.ReconnectIntervalMs, cfg.MIDI.AutoReconnect)

	// 6. 事件总线
	// MIDI 消息 → WebSocket 广播给所有已认证客户端
	verboseLog := cfg.Logging.MidiVerbose
	go func() {
		for msg := range midiReader.Msgs {
			if verboseLog {
				if s := midiVerbose(msg.Data); s != "" {
					golog.Info(s)
				}
			}
			wsServer.Broadcast(msg)
		}
	}()

	// 设备连接事件 → 日志记录
	go func() {
		for evt := range midiReader.Connects {
			golog.Info(fmt.Sprintf("MIDI device connected: [%d] \"%s\"", evt.Index, evt.Name))
		}
	}()

	// 设备断开事件 → 日志告警
	go func() {
		for range midiReader.Disconnects {
			golog.Warn("MIDI device disconnected, waiting for reconnect...")
		}
	}()

	// 7. 打印就绪信息
	displayBind := cfg.Network.Bind
	if displayBind == "" {
		displayBind = "0.0.0.0"
	}
	wsScheme, adminScheme := "ws", "http"
	if cfg.TLSEnabled() {
		wsScheme, adminScheme = "wss", "https"
	}
	golog.Info("WebSocket server: " + wsScheme + "://" + displayBind + ":" + itoa(cfg.WS.Port))
	golog.Info("HTTP admin API: " + adminScheme + "://" + displayBind + ":" + itoa(cfg.Admin.Port))
	deviceLabel := cfg.MIDI.DeviceName
	if deviceLabel == "" {
		deviceLabel = "(auto)"
	}
	golog.Info("Target MIDI device: " + deviceLabel)
	golog.Info("Ready, waiting for connections")

	// 8. 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh

	golog.Info("Received " + sig.String() + ", shutting down...")

	// 二次信号逃生门：关停过程若卡住，再按一次直接强制退出
	go func() {
		sig2 := <-sigCh
		golog.Warn("Received " + sig2.String() + " again during shutdown — forcing exit")
		os.Exit(1)
	}()

	// 带超时上限的优雅关闭：先停 MIDI（断开硬件），再停 WebSocket（踢出客户端），最后停 HTTP
	shutdownDone := make(chan struct{})
	go func() {
		midiReader.Stop()
		wsServer.Stop()
		adminServer.Stop()
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
		golog.Info("Goodbye.")
	case <-time.After(10 * time.Second):
		golog.Error("Shutdown timed out after 10s — forcing exit")
		os.Exit(1)
	}
}
