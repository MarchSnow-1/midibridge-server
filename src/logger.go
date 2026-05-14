package main

import (
	"os"
	"path/filepath"
	"time"

	golog "github.com/donnie4w/go-logger/logger"
)

// initLogger 初始化日志系统。默认启用控制台彩色输出，日志级别为 INFO
// 日志格式为 "[2026-01-15 14:30:00] info: message"
// 若 fileEnabled 为 true，同时将日志写入 logs/server.log（10MB 切割，保留 3 份备份）
func initLogger(fileEnabled bool) {
	opt := &golog.Option{
		Level:   golog.LEVEL_INFO,
		Console: true,
		AttrFormat: &golog.AttrFormat{
			// 自定义时间格式：年-月-日 时:分:秒
			SetTimeFmt: func() (string, string, string) {
				return time.Now().Format("2006-01-02 15:04:05"), "", ""
			},
			// 自定义日志级别显示文本，统一使用小写英文
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
				}
				return "unknown"
			},
		},
		// 最终日志格式：[{time}] {level}: {message}
		// 示例：[2026-01-15 14:30:00] info: MIDI Bridge server starting...
		Formatter: "[{time}] {level}: {message}\n",
	}

	// 如果配置中启用了文件日志，设置文件输出
	if fileEnabled {
		os.MkdirAll(filepath.Join(".", "logs"), 0755)
		opt.FileOption = &golog.FileSizeMode{
			Filename:  filepath.Join(".", "logs", "server.log"),
			Maxsize:   10 * 1024 * 1024, // 10MB
			Maxbuckup: 3,                // 最多保留 3 个备份文件
		}
	}

	golog.SetOption(opt)
}

// enableFileLogging 在运行时动态启用文件日志输出
// 在 main() 中，仅在配置文件的 logging.file 为 true 时才调用此函数
// 它重新应用完整的日志选项并添加 FileSizeMode 输出
func enableFileLogging() {
	os.MkdirAll(filepath.Join(".", "logs"), 0755)
	golog.SetOption(&golog.Option{
		Level:   golog.LEVEL_INFO,
		Console: true,
		AttrFormat: &golog.AttrFormat{
			SetTimeFmt: func() (string, string, string) {
				return time.Now().Format("2006-01-02 15:04:05"), "", ""
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
				}
				return "unknown"
			},
		},
		Formatter: "[{time}] {level}: {message}\n",
		FileOption: &golog.FileSizeMode{
			Filename:  filepath.Join(".", "logs", "server.log"),
			Maxsize:   10 * 1024 * 1024, // 10MB
			Maxbuckup: 3,                // server.log, server.log.1, server.log.2, server.log.3
		},
	})
	golog.Info("File logging enabled")
}
