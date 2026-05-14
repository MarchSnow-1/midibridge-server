# MIDI Bridge Server — Makefile
#   用法:
#     make build       默认构建 (输出到 dist/)
#     make version     打印当前版本号
#     make clean       清理构建产物

# 从 Git 标签自动推导版本号，格式: v1.0.0-3-gabc1234 或 v1.0.0-dirty
VERSION ?= $(shell git describe --tags --always --long --dirty 2>/dev/null || echo "dev")

LDFLAGS := -X main.version=$(VERSION)

.PHONY: build version clean

build:
	@echo [build] version: $(VERSION)
	@mkdir -p dist
	CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o dist/midibridge-server.exe ./src/
	@echo [done] dist/midibridge-server.exe

version:
	@echo $(VERSION)

clean:
	@rm -rf dist/
	@echo [clean] dist/ removed
