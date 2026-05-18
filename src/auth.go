package main

import (
golog "github.com/donnie4w/go-logger/logger"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// 密码操作相关错误定义，供 httpadmin.go 区分不同的失败原因。
var (
	errOldPasswordIncorrect = errors.New("Old password is incorrect")
	errPasswordTooShort     = errors.New("New password must be at least 6 characters")
)

// verifyPassword 使用 bcrypt 比对明文密码与存储的哈希值。
// 返回 true 表示密码匹配。若哈希为空（配置未初始化），会记录错误日志并返回 false。
func verifyPassword(hash, plainPassword string) bool {
	if hash == "" {
		golog.Error("Password hash not found — config may be uninitialized")
		return false
	}
	// bcrypt.CompareHashAndPassword 在哈希格式无效或密码不匹配时返回错误
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plainPassword)) == nil
}

// changePassword 验证旧密码后更新为新密码并持久化到配置文件。
// 安全规则：
//   - 旧密码必须正确
//   - 新密码至少 6 个字符
//   - bcrypt cost factor = 10（在合理安全性和性能之间取得平衡）
func changePassword(cfg *Config, oldPassword, newPassword string) error {
	// 第一步：验证旧密码
	if !verifyPassword(cfg.Auth.PasswordHash, oldPassword) {
		return errOldPasswordIncorrect
	}

	// 第二步：新密码长度检查
	if len(newPassword) < 6 {
		return errPasswordTooShort
	}

	// 第三步：生成 bcrypt 哈希
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 10)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	// 第四步：持久化新哈希到配置文件
	if err := cfg.SetPasswordHash(string(hash)); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	golog.Info("Password changed successfully")
	return nil
}
