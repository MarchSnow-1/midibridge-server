<div align="center">

# MIDIBridge Server

把 MIDI 设备的信号通过网络实时传给其他设备

<!-- Badges -->

[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-blue?style=for-the-badge)](https://github.com/MarchSnow-1/midibridge-server)
[![Golang](https://img.shields.io/badge/Golang-1.26%2B-green?style=for-the-badge)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-orange?style=for-the-badge)](LICENSE)
<br>
[![GitHub Release](https://img.shields.io/github/v/release/MarchSnow-1/midibridge-server?style=for-the-badge)](https://github.com/MarchSnow-1/midibridge-server/releases)
[![GitHub Repo stars](https://img.shields.io/github/stars/MarchSnow-1/midibridge-server?style=for-the-badge)](https://github.com/MarchSnow-1/midibridge-server)
[![GitHub Last Commit](https://img.shields.io/github/last-commit/MarchSnow-1/midibridge-server?style=for-the-badge)](https://github.com/MarchSnow-1/midibridge-server)
[![Total Download](https://img.shields.io/github/downloads/MarchSnow-1/midibridge-server/total?style=for-the-badge)](https://github.com/MarchSnow-1/midibridge-server/releases)

[**English**](README.md) | [**简体中文**](README_zh-CN.md)

</div>

## 快速开始（使用 Release 二进制）

从 [Releases](https://github.com/MarchSnow-1/midibridge-server/releases) 下载对应平台的二进制，解压后直接运行：

```bash
./midibridge-server
```

首次运行会自动生成 `data/config.json`，并在**控制台打印一个随机初始密码（仅显示一次）**，务必立即修改（见下方改密码）

## 安全注意事项

> **将服务端暴露到任何共享网络之前，请先阅读本节。**

- **默认明文传输** WebSocket（9001）与管理接口（9002）默认不加密，密码和 MIDI 数据以明文形式在网络上传输；同网络的攻击者可以直接嗅探，请配置 TLS（见下方）或将使用范围限制在可信局域网
- **默认监听所有网卡** 使用 `network.bind` 配置项可限制监听地址（如 `127.0.0.1`）
- **随机初始密码** 首次运行时使用加密安全随机数生成初始密码，仅在控制台打印一次，不以明文存储，请尽快通过管理接口修改初始密码
- **IP 白名单默认为空**（任何 IP 都可连接）建议设置 `ws.allowedIPs` 与 `admin.allowedIPs` 进行纵深防御
- **限速保护** 改密接口默认每 IP 每分钟 5 次；WebSocket 认证另有按 IP 的失败计数与临时封禁

### 启用 TLS

配置证书与私钥（PEM 格式）后，两个端口均升级为加密连接（wss / https）：

```json
"tls": { "cert": "certs/server.pem", "key": "certs/server.key" }
```

自签名证书的生成方式：

```bash
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.pem -days 365 -nodes
```

证书必须与客户端连接时使用的主机名或 IP 匹配。客户端通过其 `tls.caCert` 配置信任。

## 从源码构建

### 环境要求

| 依赖 | 说明 |
|------|------|
| Go | ≥ 1.26 |
| GCC | CGO 编译 RtMidi 所需 |
| libasound2-dev | 仅 Linux（ALSA 头文件） |

### 构建

Windows

```bash
# 获取源代码
git clone https://github.com/MarchSnow-1/midibridge-server.git
cd midibridge-server

# 拉取依赖
go mod tidy

# 编译
go build -o dist/midibridge-server.exe ./src/

# 运行
./dist/midibridge-server.exe
```

Linux / macOS

```bash
# 获取源代码
git clone https://github.com/MarchSnow-1/midibridge-server.git
cd midibridge-server

# 拉取依赖
go mod tidy

# 编译
go build -o dist/midibridge-server ./src/

# 运行
./dist/midibridge-server
```

### Makefile

仓库提供 Makefile（`make build`、`make version`、`make clean`）。注意：产物名硬编码为 `.exe` 且使用 POSIX 命令——Windows 上请在 Git Bash / MSYS2 中运行。手动 `go build`（上方）在所有平台可用。

## 修改密码

```bash
curl -X POST http://<服务端IP>:9002/admin/change-password \
  -H "Content-Type: application/json" \
  -d '{"oldPassword":"<初始密码>","newPassword":"你的新密码"}'
```

改密码后所有已连接的客户端会被踢出，需用新密码重连。

忘记密码可备份后删除 `data/config.json`，然后重启服务端——将生成新的随机初始密码并打印到控制台。

## 配置文件

文件位置：`data/config.json`，第一次启动自动生成，变更需重启生效。

```json
{
  "ws": {
    "port": 9001,
    "allowedIPs": ""
  },
  "admin": {
    "port": 9002,
    "allowedIPs": "",
    "rateLimitWindowMs": 60000,
    "rateLimitMaxRequests": 5
  },
  "auth": {
    "passwordHash": "$2a$10$...",
    "updatedAt": "2026-05-14T00:00:00Z"
  },
  "midi": {
    "deviceName": "",
    "autoReconnect": true,
    "reconnectIntervalMs": 3000
  },
  "logging": {
    "file": false,
    "midiVerbose": false
  },
  "network": {
    "bind": ""
  },
  "tls": {
    "cert": "",
    "key": ""
  }
}
```

### 常用配置项

**指定 MIDI 设备**（有多个设备时）：

```json
"midi": { "deviceName": "Digital Piano" }
```

若留空则自动连接第一个设备。匹配方式为大小写敏感的子串匹配（部分名称即可）。

**自动重连**（设备断开后是否重连）：

```json
"midi": { "autoReconnect": true, "reconnectIntervalMs": 3000 }
```

设为 `false` 时，首次连接失败或设备断开后不再重试。

**限制 IP 访问**（白名单，两个端口各自独立）：

```json
"admin": { "allowedIPs": "192.168.1.200" },
"ws":   { "allowedIPs": "172.16.0.1,172.16.0.2" }
```

```json
"admin": { "allowedIPs": "192.168.1.1-192.168.2.1" },
"ws":   { "allowedIPs": "192.168.1.0/24" }
```

支持格式:
- `192.168.1.1`（单 IP）
- `192.168.1.1-192.168.1.100`（范围）
- `172.16.0.0/16`（CIDR）

逗号分隔多个，留空为不限制 IP（启动时会打印警告）。

**改密限速**（防暴力枚举）：

```json
"admin": {
  "rateLimitWindowMs": 60000,
  "rateLimitMaxRequests": 5
}
```

每分钟每 IP 最多 5 次，超限返回 429。

**文件日志**（默认只输出到控制台）：

```json
"logging": { "file": true }
```

日志写入 `logs/server.log`，单文件 10MB，保留最近 3 个。

**MIDI 详细日志**（调试用——逐条记录 MIDI 事件，量极大慎开）：

```json
"logging": { "midiVerbose": true }
```

**改端口**：

```json
"ws": { "port": 12345 },
"admin": { "port": 23456 }
```

**绑定地址**（限制监听哪个网卡）：

```json
"network": { "bind": "127.0.0.1" }
```

留空监听所有网卡（`0.0.0.0`）。

**TLS**（加密两个端口——见[安全注意事项](#安全注意事项)）：

```json
"tls": { "cert": "certs/server.pem", "key": "certs/server.key" }
```

## WebSocket 协议

客户端连接 `ws://<服务端IP>:9001/`（启用 TLS 后为 `wss://`）。

### 认证

连接后 **5 秒内**发送：

```json
{ "type": "auth", "password": "..." }
```

响应：
- `{"type": "auth_ok"}` —— 认证成功，开始接收 MIDI 广播
- `{"type": "auth_fail", "reason": "..."}` —— 密码错误，连接被关闭

5 秒内未完成认证将被以 `auth_timeout` 原因踢出。

### 心跳

随时发送 `{"type": "ping"}`，服务端回 `{"type": "pong"}`。服务端同时每 30 秒发送协议层 WebSocket ping；客户端 60 秒内未响应将被断开。

### MIDI 消息

认证成功后，服务端逐条广播 MIDI 事件：

```json
{
  "type": "midi",
  "data": {
    "t": 0.023,
    "m": "OTA="
  }
}
```

- `t` —— 距上一条消息的时间增量，单位**秒**（浮点数）
- `m` —— 原始 MIDI 字节，Base64 编码

### 踢出通知

服务端可能以下列消息断开客户端：

```json
{ "type": "kicked", "reason": "..." }
```

原因：
- `auth_timeout` —— 5 秒内未完成认证
- `server_shutdown` —— 服务端正在关闭
- `password_changed` —— 密码已修改，请用新密码重连

## 管理接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `/admin/status` | GET | 运行状态（客户端数、MIDI 连接状态、密码更新时间） |
| `/admin/change-password` | POST | 修改密码（`{"oldPassword":"...","newPassword":"..."}`） |

两个接口均受 IP 白名单保护，改密接口另有速率限制。

## 端口说明

| 默认端口 | 协议 | 用途 |
|------|------|------|
| 9001 | WebSocket | 客户端连接，MIDI 数据广播 |
| 9002 | HTTP | 管理接口（状态查询、改密码） |
