<div align="center">

# MIDIBridge Server

把 MIDI 设备的信号通过网络实时传给其他设备

> 本仓库：`github.com/MarchSnow-1/midibridge-server`

[**English**](README.md) | [**简体中文**](README_zh-CN.md)

</div>

## 快速开始（使用 Release 二进制）

从 [Releases](https://github.com/MarchSnow-1/midibridge-server/releases) 下载对应平台的二进制，解压后直接运行：

```bash
./midibridge-server
```

首次运行会自动生成 `data/config.json`，**默认密码 `midiBridge123`**，务必马上修改（见下方改密码）

## 从源码构建

### 环境要求

| 依赖 | 说明 |
|------|------|
| Go | ≥ 1.22 |
| GCC | CGO 编译 RtMidi 所需 |

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

## 修改密码

```bash
curl -X POST http://<服务端IP>:9002/admin/change-password \
  -H "Content-Type: application/json" \
  -d '{"oldPassword":"midiBridge123","newPassword":"你的新密码"}'
```

改密码后所有已连接的客户端会被踢出，需用新密码重连

忘记密码可备份后删除 `data/config.json` , 然后重启服务端

密码将重置为初始密码 `midiBridge123`

## 配置文件

文件位置：`data/config.json`，第一次启动自动生成, 变更需重启生效

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
    "passwordHash": "$2b$10$...",
    "updatedAt": "2026-05-14T00:00:00Z"
  },
  "midi": {
    "deviceName": "",
    "autoReconnect": true,
    "reconnectIntervalMs": 3000
  },
  "logging": {
    "file": false
  }
}
```

### 常用配置项

**指定 MIDI 设备**（有多个设备时）：

```json
"midi": { "deviceName": "Digital Piano" }
```
若留空则自动连接第一个设备

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

逗号分隔多个, 留空为不限制IP

**改密限速**（防暴力枚举）:

```json
"admin": {
  "rateLimitWindowMs": 60000,
  "rateLimitMaxRequests": 5
}
```

每分钟每 IP 最多 5 次，超限返回 429

**文件日志**（默认只输出到控制台）：

```json
"logging": { "file": true }
```

日志写入 `logs/server.log`，单文件 10MB，保留最近 3 个。

**改端口**：

```json
"ws": { "port": 12345 },
"admin": { "port": 23456 }
```

## 端口说明

| 默认端口 | 协议 | 用途 |
|------|------|------|
| 9001 | WebSocket | 客户端连接，MIDI 数据广播 |
| 9002 | HTTP | 管理接口（状态查询、改密码） |