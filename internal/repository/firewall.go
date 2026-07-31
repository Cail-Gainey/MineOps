package repository

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// FirewallRuleLeaseRepository 持久化 MineOps 对远端防火墙规则的归属与引用。
type FirewallRuleLeaseRepository interface {
	Upsert(context.Context, *model.FirewallRuleLease) error
	ListByServer(context.Context, model.ID) ([]model.FirewallRuleLease, error)
	ListByRule(context.Context, model.ID, enums.FirewallBackend, uint16) ([]model.FirewallRuleLease, error)
	Delete(context.Context, model.ID, enums.FirewallBackend, uint16) error
}
