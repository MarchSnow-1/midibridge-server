package main

import (
	"strings"
	"testing"
)

// TestGenerateInitialPassword 验证随机初始密码的长度、字符集与唯一性。
func TestGenerateInitialPassword(t *testing.T) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	seen := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		p, err := generateInitialPassword()
		if err != nil {
			t.Fatalf("generateInitialPassword returned error: %v", err)
		}
		if len(p) != initialPasswordLength {
			t.Fatalf("password length = %d, want %d", len(p), initialPasswordLength)
		}
		for _, r := range p {
			if !strings.ContainsRune(alphabet, r) {
				t.Fatalf("password contains unexpected character %q", r)
			}
		}
		if _, dup := seen[p]; dup {
			t.Fatalf("duplicate password generated: %s", p)
		}
		seen[p] = struct{}{}
	}
}

// TestConfigValidate 验证非法配置值回退为默认值。
func TestConfigValidate(t *testing.T) {
	c := defaultConfig()
	c.WS.Port = 0
	c.Admin.Port = -1
	c.Admin.RateLimitWindowMs = 0
	c.Admin.RateLimitMaxReqs = -5
	c.MIDI.ReconnectIntervalMs = 0

	c.validate()

	if c.WS.Port != 9001 {
		t.Errorf("WS.Port = %d, want 9001", c.WS.Port)
	}
	if c.Admin.Port != 9002 {
		t.Errorf("Admin.Port = %d, want 9002", c.Admin.Port)
	}
	if c.Admin.RateLimitWindowMs != 60000 {
		t.Errorf("RateLimitWindowMs = %d, want 60000", c.Admin.RateLimitWindowMs)
	}
	if c.Admin.RateLimitMaxReqs != 5 {
		t.Errorf("RateLimitMaxReqs = %d, want 5", c.Admin.RateLimitMaxReqs)
	}
	if c.MIDI.ReconnectIntervalMs != 3000 {
		t.Errorf("ReconnectIntervalMs = %d, want 3000", c.MIDI.ReconnectIntervalMs)
	}
}
