package repository

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// FirewallRuleLeaseRepository persists MineOps ownership and references for remote firewall rules.
type FirewallRuleLeaseRepository interface {
	Upsert(context.Context, *model.FirewallRuleLease) error
	ListByServer(context.Context, model.ID) ([]model.FirewallRuleLease, error)
	ListByRule(context.Context, model.ID, enums.FirewallBackend, uint16) ([]model.FirewallRuleLease, error)
	Delete(context.Context, model.ID, enums.FirewallBackend, uint16) error
}
