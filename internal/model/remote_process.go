package model

import (
	"path"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
)

// RemoteProcessIdentity prevents PID reuse from being mistaken for the managed Minecraft process.
type RemoteProcessIdentity struct {
	ID                 ID                       `json:"id"`
	ServerID           ID                       `json:"serverID"`
	SSHSessionID       ID                       `json:"sshSessionID"`
	PID                int                      `json:"pid"`
	ProcessGroupID     int                      `json:"processGroupID"`
	LinuxStartTicks    uint64                   `json:"linuxStartTicks"`
	CommandFingerprint string                   `json:"commandFingerprint"`
	WorkingDirectory   string                   `json:"workingDirectory"`
	TmuxSession        string                   `json:"tmuxSession,omitempty"`
	ConsoleFIFO        string                   `json:"consoleFIFO,omitempty"`
	ConsoleLog         string                   `json:"consoleLog,omitempty"`
	State              enums.RemoteProcessState `json:"state"`
	StartedAt          time.Time                `json:"startedAt"`
	LastProbedAt       time.Time                `json:"lastProbedAt"`
	ExitedAt           *time.Time               `json:"exitedAt,omitempty"`
	ExitCode           *int                     `json:"exitCode,omitempty"`
	LastOutput         string                   `json:"lastOutput,omitempty"`
	CreatedAt          time.Time                `json:"createdAt"`
	UpdatedAt          time.Time                `json:"updatedAt"`
}

// NewRemoteProcessIdentity creates a validated durable PID identity after a successful remote launch.
func NewRemoteProcessIdentity(clock Clock, identity RemoteProcessIdentity) (*RemoteProcessIdentity, error) {
	if clock == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Remote Process Clock 不能为空")
	}
	now := clock.Now().UTC()
	id, err := NewID(now)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, "生成 Remote Process ID 失败", err)
	}
	identity.ID = id
	identity.State = enums.RemoteProcessRunning
	identity.StartedAt = now
	identity.LastProbedAt = now
	identity.CreatedAt = now
	identity.UpdatedAt = now
	if err := identity.Validate(); err != nil {
		return nil, err
	}
	return &identity, nil
}

// Validate enforces server/session ownership, PID identity, and controlled absolute paths.
func (p RemoteProcessIdentity) Validate() error {
	if !p.ServerID.Valid() || !p.SSHSessionID.Valid() || p.PID < 1 || p.ProcessGroupID < 1 || p.LinuxStartTicks == 0 {
		return apperror.New(apperror.CodeValidationRequired, "Remote Process Server、SSH、PID 和启动时钟不能为空")
	}
	if strings.TrimSpace(p.CommandFingerprint) == "" || !path.IsAbs(p.WorkingDirectory) || !p.State.Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Remote Process 命令特征、路径或状态无效")
	}
	if p.TmuxSession != "" {
		if !validTmuxSessionName(p.TmuxSession) {
			return apperror.New(apperror.CodeValidationInvalidArgument, "Remote Process tmux Session 名称无效")
		}
		return nil
	}
	// 兼容迁移前持久化的 FIFO/日志进程身份；新进程只写入 TmuxSession。
	if !path.IsAbs(p.ConsoleFIFO) || !path.IsAbs(p.ConsoleLog) {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Remote Process 缺少 tmux Session 或旧版 Console 路径")
	}
	return nil
}

func validTmuxSessionName(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

// ApplyProbe updates durable process evidence without changing Server lifecycle by itself.
func (p *RemoteProcessIdentity) ApplyProbe(clock Clock, state enums.RemoteProcessState, lastOutput string, exitCode *int) error {
	if p == nil || clock == nil || !state.Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Remote Process Probe 状态无效")
	}
	now := clock.Now().UTC()
	p.State = state
	p.LastProbedAt = now
	p.LastOutput = lastOutput
	p.ExitCode = exitCode
	p.UpdatedAt = now
	if state == enums.RemoteProcessExited || state == enums.RemoteProcessMismatched {
		p.ExitedAt = &now
	}
	return nil
}
