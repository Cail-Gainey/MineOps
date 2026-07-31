package port

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// FirewallStatus 描述探测到的后端、启用状态、权限与目标 TCP 端口。
type FirewallStatus struct {
	ServerID       model.ID              `json:"serverID"`
	SSHSessionID   model.ID              `json:"sshSessionID"`
	Backend        enums.FirewallBackend `json:"backend"`
	Active         bool                  `json:"active"`
	Port           uint16                `json:"port"`
	Privileged     bool                  `json:"privileged"`
	AlreadyAllowed bool                  `json:"alreadyAllowed"`
	Intent         string                `json:"intent"`
}

// FirewallDetector 探测 UFW、firewalld 或无防火墙,以及当前启用状态。
type FirewallDetector interface {
	Detect(context.Context, model.ID) (FirewallStatus, error)
}

// FirewallController 幂等应用 TCP 放行规则,不删除共享规则。
type FirewallController interface {
	EnsureAllowed(context.Context, FirewallStatus) error
}
