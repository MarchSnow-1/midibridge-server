package main

import (
	"os"
	"path/filepath"
	"time"

	golog "github.com/donnie4w/go-logger/logger"
)

// buildLogOption 构造 go-logger 的选项。
// fileEnabled=true 时附加文件日志参数（logs/server.log，10MB 轮转，保留 3 份备份）。
func buildLogOption(fileEnabled bool) *golog.Option {
	opt := &golog.Option{
		Level:   golog.LEVEL_INFO,
		Console: true,
		AttrFormat: &golog.AttrFormat{
			SetTimeFmt: func() (string, string, string) {
				return time.Now().Format("2006-01-02 15:04:05.000"), "", ""
			},
			SetLevelFmt: func(level golog.LEVELTYPE) string {
				switch level {
				case golog.LEVEL_INFO:
					return "info"
				case golog.LEVEL_WARN:
					return "warn"
				case golog.LEVEL_ERROR:
					return "error"
				case golog.LEVEL_FATAL:
					return "fatal"
				case golog.LEVEL_DEBUG:
					return "debug"
				}
				return "unknown"
			},
		},
		Formatter: "[{time}] {level}: {message}\n",
	}
	if fileEnabled {
		opt.FileOption = &golog.FileSizeMode{
			Filename:  filepath.Join(".", "logs", "server.log"),
			Maxsize:   10 * 1024 * 1024,
			Maxbuckup: 3,
		}
	}
	return opt
}

// initLogger 初始化日志（启动早期仅启用控制台输出）。
func initLogger(fileEnabled bool) {
	golog.SetOption(buildLogOption(fileEnabled))
}

// canWriteToLogsDir 探测 logs 目录能否创建且可写。
// 必须事前探测：go-logger v0.28.0 在打开日志文件失败后会将内部错误位置位
// 且全库无复位路径，此后包括控制台在内的所有日志都会被永久静默丢弃
// （运行期轮转重开失败同样触发）。绝不能把不可写的路径交给它。
func canWriteToLogsDir() bool {
	dir := filepath.Join(".", "logs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false
	}
	probe := filepath.Join(dir, ".write_probe")
	f, err := os.Create(probe)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(probe)
	return true
}

// enableFileLogging 启用文件日志。若 logs 目录不可写，回退为纯控制台日志并告警，
// 避免触发 go-logger 的"一次失败、全部日志永久静默"陷阱。
func enableFileLogging() {
	if !canWriteToLogsDir() {
		golog.Warn("Logs directory is not writable — file logging disabled, keeping console-only logging")
		return
	}
	golog.SetOption(buildLogOption(true))
	golog.Info("File logging enabled")
}
