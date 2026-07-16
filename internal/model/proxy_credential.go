package model

import (
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
)

// ProxyCredential stores outbound proxy authentication only inside SQLCipher-encrypted SQLite.
type ProxyCredential struct {
	ID        ID        `json:"id"`
	Username  string    `json:"username"`
	Password  []byte    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// NewProxyCredential creates a validated encrypted-database credential record.
func NewProxyCredential(clock Clock, username string, password []byte) (*ProxyCredential, error) {
	if clock == nil || strings.TrimSpace(username) == "" || len(password) == 0 {
		return nil, apperror.New(apperror.CodeValidationRequired, "代理用户名和密码不能为空")
	}
	now := clock.Now().UTC()
	id, err := NewID(now)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, "生成 Proxy Credential ID 失败", err)
	}
	return &ProxyCredential{ID: id, Username: strings.TrimSpace(username), Password: append([]byte(nil), password...), CreatedAt: now, UpdatedAt: now}, nil
}

// Clear overwrites in-memory secret bytes after use.
func (c *ProxyCredential) Clear() {
	if c == nil {
		return
	}
	for index := range c.Password {
		c.Password[index] = 0
	}
	c.Password = nil
}
