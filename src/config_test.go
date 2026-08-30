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
