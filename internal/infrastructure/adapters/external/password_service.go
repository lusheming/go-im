package external

import (
	"golang.org/x/crypto/bcrypt"

	"go-im/internal/application/ports"
)

// PasswordServiceAdapter 密码服务适配器
type PasswordServiceAdapter struct{}

// NewPasswordServiceAdapter 创建密码服务适配器
func NewPasswordServiceAdapter() ports.PasswordService {
	return &PasswordServiceAdapter{}
}

// HashPassword 加密密码
func (p *PasswordServiceAdapter) HashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}

// VerifyPassword 验证密码
func (p *PasswordServiceAdapter) VerifyPassword(hashedPassword, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}
