package main

import (
	"net"
	"strings"

	golog "github.com/donnie4w/go-logger/logger"
)

// isAllowed 检查指定 IP 是否匹配白名单 allowlist。
// 白名单为逗号分隔的字符串，支持三种格式：
//  1. 精确 IP：          "192.168.1.100"
//  2. IP 范围（起-止）：  "192.168.1.1-192.168.2.254"
//  3. CIDR 子网：        "172.16.0.0/16"
//
// allowlist 为空字符串时，表示允许所有 IP（白名单未启用）。
// IPv4-in-IPv6 格式（如 ::ffff:192.168.1.1）会被自动转换为 IPv4 进行比较。
func isAllowed(ip, allowlist string) bool {
	entries := parseAllowlist(allowlist)
	if len(entries) == 0 {
		return true // 空列表 = 允许所有
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false // IP 格式无效，拒绝
	}
	// 如果是 IPv4-in-IPv6 格式 (::ffff:192.168.1.1)，转为纯 IPv4
	if v4 := parsed.To4(); v4 != nil {
		parsed = v4
	}

	for _, entry := range entries {
		if matchIP(parsed, entry) {
			return true
		}
	}
	return false
}

// parseAllowlist 将逗号分隔的白名单字符串解析为条目切片。
// 自动去除空白字符和空条目。返回 nil 表示未配置白名单。
func parseAllowlist(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// matchIP 判断指定 IP 是否匹配单条白名单规则。
// 根据 entry 的格式自动选择匹配方式（CIDR > 范围 > 精确）。
func matchIP(ip net.IP, entry string) bool {
	// CIDR 格式：172.16.0.0/16
	if strings.Contains(entry, "/") {
		_, cidr, err := net.ParseCIDR(entry)
		if err != nil {
			return false
		}
		return cidr.Contains(ip)
	}

	// IP 范围格式：192.168.1.1-192.168.2.1
	if strings.Contains(entry, "-") {
		parts := strings.SplitN(entry, "-", 2)
		start := net.ParseIP(strings.TrimSpace(parts[0]))
		end := net.ParseIP(strings.TrimSpace(parts[1]))
		if start == nil || end == nil {
			return false
		}
		start = start.To4()
		end = end.To4()
		ipv4 := ip.To4()
		if start == nil || end == nil || ipv4 == nil {
			return false // 范围匹配仅支持 IPv4
		}
		// 转为整数比较，简单高效
		return ipToInt(ipv4) >= ipToInt(start) && ipToInt(ipv4) <= ipToInt(end)
	}

	// 精确 IP 匹配
	return ip.Equal(net.ParseIP(entry))
}

// ipToInt 将 IPv4 地址转换为 32 位无符号整数，用于范围比较。
// 调用方必须保证 ip 可转为 IPv4（matchIP 已先判空）；
// 此处再做一次防御性检查，避免未来新调用方传入 IPv6/nil 时 panic。
func ipToInt(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// validateAllowlist 在启动期逐条校验白名单配置，把拼写错误、非法条目、
// 写反的范围、不支持的 IPv6 范围等问题全部以日志告警暴露出来，
// 避免"配置了但静默不生效"（合法客户端被拒）或"误配全放行"而无人察觉。
func validateAllowlist(name, raw string) {
	entries := parseAllowlist(raw)
	for _, entry := range entries {
		switch {
		case strings.Contains(entry, "/"):
			if _, _, err := net.ParseCIDR(entry); err != nil {
				golog.Error(name + " allowlist entry is not a valid CIDR (ignored at runtime): " + entry)
			}
		case strings.Contains(entry, "-"):
			parts := strings.SplitN(entry, "-", 2)
			start := net.ParseIP(strings.TrimSpace(parts[0]))
			end := net.ParseIP(strings.TrimSpace(parts[1]))
			if start == nil || end == nil {
				golog.Error(name + " allowlist range has invalid IP(s) (ignored at runtime): " + entry)
			} else if s4, e4 := start.To4(), end.To4(); s4 == nil || e4 == nil {
				golog.Warn(name + " allowlist range only supports IPv4 (ignored at runtime): " + entry)
			} else if ipToInt(s4) > ipToInt(e4) {
				golog.Error(name + " allowlist range is inverted and will never match: " + entry)
			}
		default:
			if net.ParseIP(entry) == nil {
				golog.Error(name + " allowlist entry is not a valid IP (ignored at runtime): " + entry)
			}
		}
	}
}
