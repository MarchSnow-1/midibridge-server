<div align="center">

# MIDIBridge Server

Stream USB MIDI signals from keyboards and synthesizers to other computers over the network in real time.

> Repository: `github.com/MarchSnow-1/midibridge-server`

[**English**](README.md) | [**简体中文**](README_zh-CN.md)

</div>

## Quick Start (Release Binary)

Download the binary for your platform from [Releases](https://github.com/MarchSnow-1/midibridge-server/releases), extract and run:

```bash
./midibridge-server
```

On first run, `data/config.json` is auto-generated with default password **`midiBridge123`**. Change it immediately (see below).

## Build from Source

### Requirements

| Dependency | Notes |
|------------|-------|
| Go | ≥ 1.22 |
| GCC | Required for CGO / RtMidi |

### Build

Windows

```bash
# Clone the repo
git clone https://github.com/MarchSnow-1/midibridge-server.git
cd midibridge-server

# Fetch dependencies
go mod tidy

# Build
# Windows
go build -o dist/midibridge-server.exe ./src/

# Run
./dist/midibridge-server.exe
```

Linux / macOS

```bash
# Clone the repo
git clone https://github.com/MarchSnow-1/midibridge-server.git
cd midibridge-server

# Fetch dependencies
go mod tidy

# Build
go build -o dist/midibridge-server ./src/

# Run
./dist/midibridge-server
```

## Change Password

```bash
curl -X POST http://<server-ip>:9002/admin/change-password \
  -H "Content-Type: application/json" \
  -d '{"oldPassword":"midiBridge123","newPassword":"your_new_password"}'
```

All connected clients are kicked after a password change and must reconnect with the new password.

For a forgotten password, back up and delete `data/config.json`, then restart the server. The password resets to `midiBridge123`.

## Configuration

File: `data/config.json`. Auto-generated on first run. Restart required for changes to take effect.

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

### Common Settings

**Target a specific MIDI device** (when multiple are connected):

```json
"midi": { "deviceName": "Digital Piano" }
```

Leave empty to auto-connect to the first available device.

**IP allowlist** (separate for each port):

Single IPs:
```json
"admin": { "allowedIPs": "192.168.1.200" },
"ws":   { "allowedIPs": "172.16.0.1,172.16.0.2" }
```

IP ranges / CIDR:
```json
"admin": { "allowedIPs": "192.168.1.1-192.168.2.1" },
"ws":   { "allowedIPs": "192.168.1.0/24" }
```

Supported formats:
- `192.168.1.1` — single IP
- `192.168.1.1-192.168.1.100` — IP range
- `172.16.0.0/16` — CIDR

Multiple entries separated by commas. Leave empty to allow all.

**Rate limiting** (prevents brute-force on the password endpoint):

```json
"admin": {
  "rateLimitWindowMs": 60000,
  "rateLimitMaxRequests": 5
}
```

Max 5 requests per IP per minute. Returns 429 when exceeded.

**File logging** (console only by default):

```json
"logging": { "file": true }
```

Logs written to `logs/server.log`, 10MB max per file, keeps the last 3 files.

**Custom ports**:

```json
"ws": { "port": 12345 },
"admin": { "port": 23456 }
```

## Ports

| Default | Protocol | Purpose |
|---------|----------|---------|
| 9001 | WebSocket | Client connections & MIDI broadcast |
| 9002 | HTTP | Admin API (status & password change) |
