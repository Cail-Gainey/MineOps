package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

const hostSpecsProbeTimeout = 5 * time.Second

const hostSpecsProbeScript = `cpu_count=$(getconf _NPROCESSORS_ONLN 2>/dev/null || true)
case "$cpu_count" in ''|*[!0-9]*) cpu_count=$(awk '/^processor[[:space:]]*:/{count++} END{print count+0}' /proc/cpuinfo 2>/dev/null || true) ;; esac
memory_kib=$(awk '/^MemTotal:[[:space:]]+/{print $2; exit}' /proc/meminfo 2>/dev/null || true)
home=${HOME:-/}
disk_kib=$(df -Pk "$home" 2>/dev/null | awk 'NR==2 {print $2}' || true)
printf '%s\t%s\t%s\n' "${cpu_count:-0}" "${memory_kib:-0}" "${disk_kib:-0}"`

// CollectHostSpecs 按配置的直连或 Jump Host 路由完成认证,并一次性读取主机容量信息。
func (f *SSHClientFactory) CollectHostSpecs(ctx context.Context, session *model.SSHSession, settings model.SSHSettings) (model.SSHHostSpecs, error) {
	if session == nil {
		return model.SSHHostSpecs{}, apperror.New(apperror.CodeValidationRequired, "SSH Session 不能为空")
	}
	client, err := f.Connect(ctx, session, settings)
	if err != nil {
		return model.SSHHostSpecs{}, err
	}
	defer func() { _ = client.Close() }()
	return client.collectHostSpecs(ctx)
}

func (c *SSHClient) collectHostSpecs(ctx context.Context) (model.SSHHostSpecs, error) {
	result, err := c.RunCommand(ctx, RemoteCommand{
		Executable: "sh", Arguments: []string{"-c", hostSpecsProbeScript},
		Timeout: hostSpecsProbeTimeout, MaximumOutput: 1024,
	})
	if err != nil {
		return model.SSHHostSpecs{}, err
	}
	return parseHostSpecs(result.Stdout)
}

// parseHostSpecs 只读取探测输出的最后一行,避免远端 profile 或 MOTD 写入 stdout 干扰采集。
func parseHostSpecs(output string) (model.SSHHostSpecs, error) {
	fields := strings.Fields(lastNonEmptyLine(output))
	if len(fields) != 3 {
		return model.SSHHostSpecs{}, apperror.New(apperror.CodeProcessExitFailed, "SSH 主机规格探测输出无效")
	}
	cpuCount, cpuErr := strconv.Atoi(fields[0])
	memoryKiB, memoryErr := strconv.ParseInt(fields[1], 10, 64)
	diskKiB, diskErr := strconv.ParseInt(fields[2], 10, 64)
	const maximumKiB = int64(9_007_199_254_740_991)
	if cpuErr != nil || memoryErr != nil || diskErr != nil || cpuCount < 1 || memoryKiB < 1 || diskKiB < 1 || memoryKiB > maximumKiB || diskKiB > maximumKiB {
		return model.SSHHostSpecs{}, apperror.New(apperror.CodeProcessExitFailed, "SSH 主机规格探测结果无效")
	}
	return model.SSHHostSpecs{
		CPUCount: cpuCount, MemoryBytes: memoryKiB * 1024, DiskBytes: diskKiB * 1024,
	}, nil
}

func lastNonEmptyLine(output string) string {
	lines := strings.Split(output, "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		if trimmed := strings.TrimSpace(lines[index]); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// EnsureHostSpecs 仅在 Session 尚无主机容量信息时采集,确保列表不会重复触发 SSH。
func (m *SSHSessionManager) EnsureHostSpecs(ctx context.Context, clients *SSHClientFactory, session *model.SSHSession, settings model.SSHSettings) (model.SSHHostSpecs, error) {
	if session == nil {
		return model.SSHHostSpecs{}, apperror.New(apperror.CodeValidationRequired, "SSH Session 不能为空")
	}
	if session.HostSpecs.Collected() {
		return session.HostSpecs, nil
	}
	return m.RefreshHostSpecs(ctx, clients, session, settings)
}

// RefreshHostSpecs 经 SSH 探测主机一次,并把结果写回 Session 行。
func (m *SSHSessionManager) RefreshHostSpecs(ctx context.Context, clients *SSHClientFactory, session *model.SSHSession, settings model.SSHSettings) (model.SSHHostSpecs, error) {
	if clients == nil || session == nil {
		return model.SSHHostSpecs{}, apperror.New(apperror.CodeValidationRequired, "SSH 主机规格采集依赖不能为空")
	}
	specs, err := clients.CollectHostSpecs(ctx, session, settings)
	if err != nil {
		return model.SSHHostSpecs{}, err
	}
	collectedAt := m.clock.Now().UTC()
	specs.CollectedAt = &collectedAt
	if err := m.store.SSHSessions().UpdateHostSpecs(ctx, session.ID, specs); err != nil {
		return model.SSHHostSpecs{}, err
	}
	session.HostSpecs = specs
	return specs, nil
}
