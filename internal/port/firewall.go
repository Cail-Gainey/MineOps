package port

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// FirewallStatus describes detected backend, activation, privilege, and the target TCP port.
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

// FirewallDetector probes UFW/firewalld/none and current active state.
type FirewallDetector interface {
	Detect(context.Context, model.ID) (FirewallStatus, error)
}

// FirewallController applies idempotent TCP allow rules without removing shared rules.
type FirewallController interface {
	EnsureAllowed(context.Context, FirewallStatus) error
}
