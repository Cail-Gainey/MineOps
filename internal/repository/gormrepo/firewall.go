package gormrepo

import (
	"context"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"gorm.io/gorm"
)

// FirewallRuleLeaseRecord is the encrypted SQLite representation of one firewall rule reference.
type FirewallRuleLeaseRecord struct {
	ServerID       string `gorm:"primaryKey;size:36"`
	Backend        string `gorm:"primaryKey;index:idx_firewall_rule,priority:2;size:24"`
	Port           uint16 `gorm:"primaryKey;index:idx_firewall_rule,priority:3"`
	SSHSessionID   string `gorm:"index:idx_firewall_rule,priority:1;size:36"`
	Managed        bool
	PendingRemoval bool `gorm:"index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time `gorm:"index"`
}

type firewallRuleLeaseRepository struct{ database *gorm.DB }

func (r *firewallRuleLeaseRepository) Upsert(ctx context.Context, lease *model.FirewallRuleLease) error {
	if lease == nil {
		return apperror.New(apperror.CodeValidationRequired, "防火墙规则租约不能为空")
	}
	if err := lease.Validate(); err != nil {
		return err
	}
	record := firewallLeaseToRecord(*lease)
	if err := r.database.WithContext(ctx).Save(&record).Error; err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "保存防火墙规则租约失败", err)
	}
	return nil
}

func (r *firewallRuleLeaseRepository) ListByServer(ctx context.Context, serverID model.ID) ([]model.FirewallRuleLease, error) {
	var records []FirewallRuleLeaseRecord
	if err := r.database.WithContext(ctx).Where("server_id = ?", serverID.String()).Order("updated_at desc").Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Server 防火墙规则租约失败", err)
	}
	return firewallLeaseRecords(records), nil
}

func (r *firewallRuleLeaseRepository) ListByRule(ctx context.Context, sshSessionID model.ID, backend enums.FirewallBackend, rulePort uint16) ([]model.FirewallRuleLease, error) {
	var records []FirewallRuleLeaseRecord
	if err := r.database.WithContext(ctx).Where("ssh_session_id = ? AND backend = ? AND port = ?", sshSessionID.String(), backend.String(), rulePort).Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询防火墙规则引用失败", err)
	}
	return firewallLeaseRecords(records), nil
}

func (r *firewallRuleLeaseRepository) Delete(ctx context.Context, serverID model.ID, backend enums.FirewallBackend, rulePort uint16) error {
	result := r.database.WithContext(ctx).Delete(&FirewallRuleLeaseRecord{}, "server_id = ? AND backend = ? AND port = ?", serverID.String(), backend.String(), rulePort)
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "删除防火墙规则租约失败", result.Error)
	}
	return nil
}

func firewallLeaseToRecord(lease model.FirewallRuleLease) FirewallRuleLeaseRecord {
	return FirewallRuleLeaseRecord{
		ServerID: lease.ServerID.String(), SSHSessionID: lease.SSHSessionID.String(), Backend: lease.Backend.String(), Port: lease.Port,
		Managed: lease.Managed, PendingRemoval: lease.PendingRemoval, CreatedAt: lease.CreatedAt, UpdatedAt: lease.UpdatedAt,
	}
}

func firewallLeaseRecords(records []FirewallRuleLeaseRecord) []model.FirewallRuleLease {
	result := make([]model.FirewallRuleLease, len(records))
	for index, record := range records {
		result[index] = model.FirewallRuleLease{
			ServerID: model.ID(record.ServerID), SSHSessionID: model.ID(record.SSHSessionID), Backend: enums.FirewallBackend(record.Backend), Port: record.Port,
			Managed: record.Managed, PendingRemoval: record.PendingRemoval, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		}
	}
	return result
}
