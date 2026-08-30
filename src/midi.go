package main

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	golog "github.com/donnie4w/go-logger/logger"
	"gitlab.com/gomidi/midi"
	driver "gitlab.com/gomidi/rtmididrv"
)

// MidiMessage 表示一条 MIDI 消息，包含时间增量（秒）和原始 MIDI 数据字节
type MidiMessage struct {
	DeltaTime float64
	Data      []byte
}

// ConnectEvent 在 MIDI 设备连接成功时发出，携带端口索引和名称
type ConnectEvent struct {
	Index int
	Name  string
}

// MidiReader 负责发现、打开并持续读取目标 MIDI 输入设备
// 它通过 channel 向外发送消息和连接状态事件，支持设备热插拔自动重连
type MidiReader struct {
	deviceName        string        // 目标 MIDI 设备名称（支持子串匹配）
	reconnectInterval time.Duration // 断开后重连间隔
	shouldReconnect   bool          // 是否启用自动重连

	Msgs        chan MidiMessage  // 向外暴露的 MIDI 消息 channel，容量 256
	Connects    chan ConnectEvent // 设备连接成功事件 channel
	Disconnects chan struct{}     // 设备断开事件 channel

	drv midi.Driver // RtMidi 底层驱动实例
	in  midi.In     // 当前打开的 MIDI 输入端口

	msgCh chan midiMsg // 内部消息 channel，Listener 回调写入，readLoop 消费

	// 两级丢弃计数（atomic）：使"队列满导致的消息丢失"可观测，
	// 由 dropMonitor 周期性汇总告警
	droppedInCallback int64
	droppedForward    int64

	mu       sync.Mutex    // 保护 in、drv 和 shouldReconnect 的并发访问
	done     chan struct{} // 关闭信号——通知所有 goroutine 退出
	stopped  bool          // Stop 幂等标志
	loopDone chan struct{} // runLoop 完全退出后关闭，用于 Stop 的关闭顺序控制
}

// midiMsg 是内部使用的轻量级消息结构，避免暴露 Listener 的 deltaMicroseconds 参数
type midiMsg struct {
	data     []byte
	deltaSec float64
}

// NewMidiReader 创建一个新的 MidiReader 实例，初始化所有 channel
// 需随后调用 Start() 才会开始设备扫描和连接。
func NewMidiReader() *MidiReader {
	return &MidiReader{
		Msgs:        make(chan MidiMessage, 256),
		Connects:    make(chan ConnectEvent, 8),
		Disconnects: make(chan struct{}, 4),
		msgCh:       make(chan midiMsg, 256),
		done:        make(chan struct{}),
	}
}

// Start 设置目标设备名和重连参数，然后启动后台主循环。
// autoReconnect=false 时：仍会做一次初始连接尝试，但失败或设备断开后不再重连。
// deviceName 为空字符串时，回退到使用系统首个 MIDI 输入端口。
func (m *MidiReader) Start(deviceName string, reconnectIntervalMs int, autoReconnect bool) {
	m.deviceName = deviceName
	m.reconnectInterval = time.Duration(reconnectIntervalMs) * time.Millisecond
	m.shouldReconnect = autoReconnect
	m.loopDone = make(chan struct{})
	go func() {
		m.runLoop()
		close(m.loopDone)
	}()
	go m.dropMonitor()
}

// dropMonitor 周期性汇总两级队列的丢弃计数。若无此监控，
// 队列满导致的 MIDI 消息丢失将完全不可见。
func (m *MidiReader) dropMonitor() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			if n := atomic.SwapInt64(&m.droppedInCallback, 0); n > 0 {
				golog.Warn(fmt.Sprintf("Dropped %d MIDI message(s) in driver callback (internal queue full)", n))
			}
			if n := atomic.SwapInt64(&m.droppedForward, 0); n > 0 {
				golog.Warn(fmt.Sprintf("Dropped %d MIDI message(s) while forwarding to broadcast (Msgs queue full)", n))
			}
		}
	}
}

// Stop 发出停止信号，关闭 MIDI 端口和驱动，释放所有资源。
// 该方法是幂等的，多次调用不会 panic。
// 关闭顺序：发停止信号 → 关硬件 → 等待主循环（含 readLoop）完全退出 →
// 关闭对外暴露的 channel（Msgs/Connects/Disconnects），使 main 中的
// 消费协程（for range）能正常终止。内部 msgCh 永不关闭——
// 驱动回调可能在关停瞬间仍在投递，关闭它会引发 send-on-closed panic。
func (m *MidiReader) Stop() {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	m.stopped = true
	m.shouldReconnect = false
	loopDone := m.loopDone
	m.mu.Unlock()

	close(m.done)

	m.mu.Lock()
	if m.in != nil && m.in.IsOpen() {
		m.in.Close()
		m.in = nil
	}
	if m.drv != nil {
		m.drv.Close()
		m.drv = nil
	}
	m.mu.Unlock()

	// 等待主循环（含 readLoop）完全退出，之后再关闭对外 channel，
	// 杜绝向已关闭 channel 发送的窗口
	if loopDone != nil {
		select {
		case <-loopDone:
		case <-time.After(5 * time.Second):
			golog.Warn("Timed out waiting for MIDI loop to exit — closing channels anyway")
		}
	}
	close(m.Msgs)
	close(m.Connects)
	close(m.Disconnects)
	golog.Info("MIDI reader stopped")
}

// IsConnected 返回 MIDI 端口当前是否处于打开状态
func (m *MidiReader) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.in != nil && m.in.IsOpen()
}

// runLoop 主循环：连接设备 → readLoop 阻塞读取 → 断开后按配置决定是否重连。
// 首次连接尝试总是执行；其后仅在 shouldReconnect 为 true 时继续重试，
// 只要 done 未关闭就一直循环。
func (m *MidiReader) runLoop() {
	for {
		select {
		case <-m.done:
			return
		default:
		}

		connected := m.tryConnect()
		if connected {
			// readLoop 阻塞直到设备断开（端口关闭或 done 信号）
			m.readLoop()
			// 通知外部：设备已断开
			select {
			case m.Disconnects <- struct{}{}:
			default:
			}
		}

		// 断开或连接失败后：按配置决定是否继续重连
		m.mu.Lock()
		reconnect := m.shouldReconnect
		m.mu.Unlock()
		if !reconnect {
			if !connected {
				golog.Warn("MIDI connection failed and autoReconnect is disabled — giving up")
			} else {
				golog.Info("MIDI device disconnected and autoReconnect is disabled — stopping")
			}
			return
		}

		// 等待重连间隔（同时监听 done 以便快速退出）
		select {
		case <-m.done:
			return
		case <-time.After(m.reconnectInterval):
		}
	}
}

// tryConnect 枚举系统中所有 MIDI 输入端口，匹配目标设备名，打开并设置监听回调
// 返回值表示是否连接成功 该方法持有 mu 锁，内部操作是线程安全的
func (m *MidiReader) tryConnect() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 初始化驱动（仅首次调用）
	if m.drv == nil {
		var err error
		m.drv, err = driver.New()
		if err != nil {
			golog.Warn("Failed to init MIDI driver, retrying... error=" + err.Error())
			return false
		}
	}

	// 枚举所有 MIDI 输入端口
	ins, err := m.drv.Ins()
	if err != nil {
		golog.Warn("Failed to list MIDI ports error=" + err.Error())
		return false
	}

	if len(ins) == 0 {
		golog.Warn("No MIDI devices detected, waiting...")
		return false
	}

	// 根据 deviceName 查找目标设备
	var target midi.In
	if m.deviceName != "" {
		// 指定了设备名：使用子串匹配（大小写敏感）
		for _, in := range ins {
			if strings.Contains(in.String(), m.deviceName) {
				target = in
				break
			}
		}
		if target == nil {
			// 未找到匹配设备，列出所有可用端口方便排查
			golog.Error("not found target midi device: " + m.deviceName)
			golog.Error("Available devices:")
			for _, in := range ins {
				golog.Error("  [" + itoa(in.Number()) + "] " + in.String())
			}
			return false
		}
	} else {
		// 未指定设备名：取系统第一个 MIDI 输入端口
		target = ins[0]
	}

	// 打开 MIDI 端口
	if err := target.Open(); err != nil {
		golog.Warn("Failed to open MIDI port port=" + target.String() + " error=" + err.Error())
		return false
	}

	// 设置消息监听回调——该回调在 MIDI 驱动线程中执行，因此只做数据拷贝和 channel 投递
	m.in = target
	m.in.SetListener(func(data []byte, deltaMicroseconds int64) {
		// 拷贝 data 切片，避免底层缓冲区被复用后数据被覆盖
		msg := make([]byte, len(data))
		copy(msg, data)
		select {
		case m.msgCh <- midiMsg{data: msg, deltaSec: float64(deltaMicroseconds) / 1_000_000.0}:
		default:
			// channel 满时丢弃消息，避免阻塞 MIDI 驱动线程（计数可观测）
			atomic.AddInt64(&m.droppedInCallback, 1)
		}
	})

	// 通知外部：设备已连接
	select {
	case m.Connects <- ConnectEvent{Index: target.Number(), Name: target.String()}:
	default:
	}
	return true
}

// readLoop 从内部 msgCh 消费 MIDI 消息，将它们转发到对外 Msgs channel。
// 同时每 500ms 检查一次端口是否仍处于打开状态，如果端口被拔出则返回让主循环进入重连流程。
func (m *MidiReader) readLoop() {
	// 定时器在循环外创建一次：避免 select 内 time.After 每轮产生新定时器
	// 造成的高频分配与 GC 压力
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case msg := <-m.msgCh:
			// 将内部消息转发到对外的 Msgs channel
			select {
			case m.Msgs <- MidiMessage{DeltaTime: msg.deltaSec, Data: msg.data}:
			default:
				// Msgs channel 满时丢弃，避免背压阻塞整个链条（计数可观测）
				atomic.AddInt64(&m.droppedForward, 1)
			}
		case <-ticker.C:
			// 定期检查端口状态——如果设备物理断开，IsOpen() 会返回 false
			m.mu.Lock()
			open := m.in != nil && m.in.IsOpen()
			m.mu.Unlock()
			if !open {
				return // 端口断开，退出 readLoop 让主循环重连
			}
		}
	}
}

// itoa 是将整数转为字符串的轻量实现，避免引入 strconv 或 fmt 的包
// 仅在 MIDI 端口号和日志输出中使用，输入值范围为 0~65535
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

// 音符名称映射（MIDI 音符编号 mod 12）
var noteNames = [12]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// midiVerbose 将 MIDI 消息解析为人类可读的详细描述。
// 仅处理通道消息（Note On/Off、CC、Program Change、Pitch Bend），
// 系统消息返回空字符串（不记录）。
func midiVerbose(data []byte) string {
	if len(data) < 2 {
		return ""
	}
	status := data[0]
	msgType := status & 0xF0        // 高 4 位 = 消息类型
	channel := int(status&0x0F) + 1 // 低 4 位 = 通道号（显示为 1-16）

	switch msgType {
	case 0x80: // Note Off
		if len(data) < 3 {
			return ""
		}
		note := data[1]
		vel := int(data[2])
		oct := int(note/12) - 1
		name := noteNames[note%12]
		return "CH" + itoa(channel) + " Note Off: " + name + itoa(oct) + " vel=" + itoa(vel)

	case 0x90: // Note On (velocity=0 视为 Note Off)
		if len(data) < 3 {
			return ""
		}
		note := data[1]
		vel := int(data[2])
		oct := int(note/12) - 1
		name := noteNames[note%12]
		if vel == 0 {
			return "CH" + itoa(channel) + " Note Off: " + name + itoa(oct) + " (vel=0)"
		}
		return "CH" + itoa(channel) + " Note On:  " + name + itoa(oct) + " vel=" + itoa(vel)

	case 0xB0: // Control Change
		if len(data) < 3 {
			return ""
		}
cc := int(data[1])
			if cc == 123 || cc == 120 {
			return ""
			}
			val := int(data[2])
		return "CH" + itoa(channel) + " CC#" + itoa(cc) + " = " + itoa(val)

	case 0xC0: // Program Change
		prog := int(data[1])
		return "CH" + itoa(channel) + " Program: " + itoa(prog)

	case 0xE0: // Pitch Bend
		if len(data) < 3 {
			return ""
		}
		lsb := int(data[1])
		msb := int(data[2])
		val := (msb<<7 | lsb) - 8192 // 居中为 0
		return "CH" + itoa(channel) + " Pitch: " + itoa(val)
	}
	return "" // 系统消息不记录
}
