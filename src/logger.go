package main

import (
	"os"
	"path/filepath"
	"time"

	golog "github.com/donnie4w/go-logger/logger"
)

func initLogger(fileEnabled bool) {
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
				}
				return "unknown"
			},
		},
		Formatter: "[{time}] {level}: {message}\n",
	}

	if fileEnabled {
		os.MkdirAll(filepath.Join(".", "logs"), 0755)
		opt.FileOption = &golog.FileSizeMode{
			Filename:  filepath.Join(".", "logs", "server.log"),
			Maxsize:   10 * 1024 * 1024,
			Maxbuckup: 3,
		}
	}

	golog.SetOption(opt)
}

func enableFileLogging() {
	os.MkdirAll(filepath.Join(".", "logs"), 0755)
	golog.SetOption(&golog.Option{
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
				}
				return "unknown"
			},
		},
		Formatter: "[{time}] {level}: {message}\n",
		FileOption: &golog.FileSizeMode{
			Filename:  filepath.Join(".", "logs", "server.log"),
			Maxsize:   10 * 1024 * 1024,
			Maxbuckup: 3,
		},
	})
	golog.Info("File logging enabled")
}
