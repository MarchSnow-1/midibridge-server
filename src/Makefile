# MIDI Bridge Server — Makefile
#   用法（在 src/ 目录下运行）:
#     make build       默认构建 (输出到仓库根 dist/)
#     make version     打印当前版本号
#     make clean       清理构建产物

# 从 Git 标签自动推导版本号，格式: v1.0.0-3-gabc1234 或 v1.0.0-dirty
VERSION ?= $(shell git describe --tags --always --long --dirty 2>/dev/null || echo "dev")

# 静态链接策略按目标系统区分，与 CI 的构建参数保持一致
# Windows 的 mingw 运行库不是系统组件，动态链接会让产物在干净机器上直接报缺少 DLL，因此整包静态链接
# Linux 只静态链接 libgcc 与 libstdc++，libc 与 libasound 保持动态，这是链接系统音频栈的前提
# macOS 的 libc++ 与各 Framework 由系统提供，不需要额外参数
#
# The static linking policy differs per target and mirrors the CI build flags
# The mingw runtime is not an OS component on Windows, so a dynamic link makes the binary fail with a missing DLL on a clean machine and it must be linked in as a whole
# Linux links only libgcc and libstdc++ statically and keeps libc and libasound dynamic, which is what linking against the system audio stack requires
# macOS ships libc++ and the frameworks with the OS, so it needs no extra flag
ifeq ($(OS),Windows_NT)
	EXTLDFLAGS := -extldflags '-static'
else
	UNAME_S := $(shell uname -s 2>/dev/null)
ifneq ($(UNAME_S),Darwin)
	EXTLDFLAGS := -extldflags '-static-libgcc -static-libstdc++'
endif
endif

LDFLAGS := $(strip -X main.version=$(VERSION) $(EXTLDFLAGS))

.PHONY: build version clean

build:
	@echo [build] version: $(VERSION)
	@mkdir -p ../dist
	CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o ../dist/midibridge-server.exe .
	@echo [done] ../dist/midibridge-server.exe

version:
	@echo $(VERSION)

clean:
	@rm -rf ../dist/
	@echo [clean] ../dist/ removed
