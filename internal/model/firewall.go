package model

import (
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
)

// FirewallRuleLease records one Server reference to a remote firewall rule managed by MineOps.
type FirewallRuleLease struct {
	ServerID       ID                    `json:"serverID"`
	SSHSessionID   ID                    `json:"sshSessionID"`
	Backend        enums.FirewallBackend `json:"backend"`
	Port           uint16                `json:"port"`
	Managed        bool                  `json:"managed"`
	PendingRemoval bool                  `json:"pendingRemoval"`
	CreatedAt      time.Time             `json:"createdAt"`
	UpdatedAt      time.Time             `json:"updatedAt"`
}

// Validate checks firewall lease identity, backend, port, and timestamps.
func (l FirewallRuleLease) Validate() error {
	if !l.ServerID.Valid() || !l.SSHSessionID.Valid() || !l.Backend.Valid() || l.Backend == enums.FirewallBackendNone || l.Port == 0 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "防火墙规则租约无效")
	}
	if l.CreatedAt.IsZero() || l.UpdatedAt.IsZero() {
		return apperror.New(apperror.CodeValidationRequired, "防火墙规则租约时间不能为空")
	}
	return nil
}

// DefaultMinecraftPort returns the initial listening port used before generated configuration exists.
func DefaultMinecraftPort(serverType enums.MinecraftServerType) uint16 {
	switch serverType {
	case enums.ServerVelocity, enums.ServerWaterfall, enums.ServerBungee:
		return 25577
	default:
		return 25565
	}
}
