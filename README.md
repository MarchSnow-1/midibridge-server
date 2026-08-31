<div align="center">

# MIDIBridge Server

Stream USB MIDI signals from keyboards and synthesizers to other computers over the network in real time.

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

## Quick Start (Release Binary)

Download the binary for your platform from [Releases](https://github.com/MarchSnow-1/midibridge-server/releases), extract and run:

```bash
./midibridge-server
```

On first run, `data/config.json` is auto-generated and a **random initial password** is printed to the console (shown only once). Change it immediately (see below).

## Security Notes

> **Read this before exposing the server to any shared network.**

- **Plaintext transport by default.** WebSocket (9001) and the admin API (9002) are unencrypted. Passwords and MIDI data travel in cleartext — an attacker on the same network can sniff them. Configure TLS (see below) or restrict access to a trusted LAN.
- **Listens on all interfaces by default.** Use the `network.bind` setting to restrict the listening address (e.g. `127.0.0.1`).
- **Random initial password.** On first run, a cryptographically random password is generated and printed once to the console. It is not stored in plaintext. Change it immediately via the admin API.
- **IP allowlists are empty by default** (any IP can connect). Set `ws.allowedIPs` and `admin.allowedIPs` for defense in depth.
- **Rate limiting** applies to the password-change endpoint (5 req/min per IP by default). WebSocket authentication also has per-IP failure counting with temporary bans.

### Enabling TLS

Provide a certificate and key (PEM) to upgrade both ports to encrypted connections (wss / https):

```json
"tls": { "cert": "certs/server.pem", "key": "certs/server.key" }
```

For self-signed certificates, generate one with:

```bash
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.pem -days 365 -nodes
```

The certificate must match the hostname or IP that clients connect to. Clients configure trust via their own `tls.caCert` setting.

## Build from Source

### Requirements

| Dependency | Notes |
|------------|-------|
| Go | ≥ 1.26 |
| GCC | Required for CGO / RtMidi |
| libasound2-dev | Linux only (ALSA headers for RtMidi) |

### Build

Windows

```bash
# Clone the repo
git clone https://github.com/MarchSnow-1/midibridge-server.git
cd midibridge-server

# Fetch dependencies
go mod tidy

# Build
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

### Makefile

A Makefile is provided (`make build`, `make version`, `make clean`). Note: it produces a Windows-style `.exe` name on all platforms and uses POSIX commands — on Windows, run it from Git Bash / MSYS2. Manual `go build` (above) works everywhere.

## Change Password

```bash
curl -X POST http://<server-ip>:9002/admin/change-password \
  -H "Content-Type: application/json" \
  -d '{"oldPassword":"<initial-password>","newPassword":"your_new_password"}'
```

All connected clients are kicked after a password change and must reconnect with the new password.

For a forgotten password, back up and delete `data/config.json`, then restart the server. A new random initial password will be generated and printed to the console.

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

### Common Settings

**Target a specific MIDI device** (when multiple are connected):

```json
"midi": { "deviceName": "Digital Piano" }
```

Leave empty to auto-connect to the first available device. Matching is case-sensitive substring (`strings.Contains`) — a partial name works.

**Auto-reconnect** (reconnect after device disconnection):

```json
"midi": { "autoReconnect": true, "reconnectIntervalMs": 3000 }
```

When `false`, the server gives up after the first connection attempt fails or the device disconnects.

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

Multiple entries separated by commas. Leave empty to allow all (a warning is logged at startup when left empty).

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

**MIDI verbose logging** (debugging — logs every MIDI event, very high volume):

```json
"logging": { "midiVerbose": true }
```

**Custom ports**:
```json
"ws": { "port": 12345 },
"admin": { "port": 23456 }
```

**Bind address** (restrict which network interface to listen on):

```json
"network": { "bind": "127.0.0.1" }
```

Leave empty to listen on all interfaces (`0.0.0.0`).

**TLS** (encrypt both ports — see [Security Notes](#security-notes)):

```json
"tls": { "cert": "certs/server.pem", "key": "certs/server.key" }
```

## WebSocket Protocol

Clients connect to `ws://<server-ip>:9001/` (or `wss://` when TLS is enabled).

### Authentication

Send within **5 seconds** of connecting:

```json
{ "type": "auth", "password": "..." }
```

Responses:
- `{"type": "auth_ok"}` — authenticated, MIDI broadcast begins
- `{"type": "auth_fail", "reason": "..."}` — wrong password; connection is closed

Failure to authenticate within 5 seconds results in a kick with reason `auth_timeout`.

### Heartbeat

Send `{"type": "ping"}` at any time; the server responds with `{"type": "pong"}`. The server also sends protocol-level WebSocket pings every 30 seconds; clients that fail to respond within 60 seconds are disconnected.

### MIDI Messages

After authentication, the server broadcasts each MIDI event:

```json
{
  "type": "midi",
  "data": {
    "t": 0.023,
    "m": "OTA="
  }
}
```

- `t` — time delta since the previous message, in **seconds** (float)
- `m` — raw MIDI bytes, Base64-encoded

### Kick Notifications

The server may disconnect a client with a kick message:

```json
{ "type": "kicked", "reason": "..." }
```

Reasons:
- `auth_timeout` — authentication not completed within 5 seconds
- `server_shutdown` — server is stopping
- `password_changed` — the password was changed; reconnect with the new one

## Admin API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/admin/status` | GET | Runtime status (client count, MIDI connection, password update time) |
| `/admin/change-password` | POST | Change password (`{"oldPassword":"...","newPassword":"..."}`) |

Both endpoints are protected by IP allowlist and rate limiting (change-password only).

## Ports

| Default | Protocol | Purpose |
|---------|----------|---------|
| 9001 | WebSocket | Client connections & MIDI broadcast |
| 9002 | HTTP | Admin API (status & password change) |
