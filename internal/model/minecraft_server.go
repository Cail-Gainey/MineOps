package model

import (
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
)

var unsafeServerDirectoryCharacter = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// LaunchProfile contains structured Java process arguments without a concatenated shell command.
type LaunchProfile struct {
	XmsMiB           int      `json:"xmsMiB"`
	XmxMiB           int      `json:"xmxMiB"`
	JVMArguments     []string `json:"jvmArguments"`
	JarPath          string   `json:"jarPath"`
	WorkingDirectory string   `json:"workingDirectory"`
	ServerArguments  []string `json:"serverArguments"`
}

// MinecraftServer is one remotely managed server bound to exactly one SSH Session.
type MinecraftServer struct {
	ID             ID                        `json:"id"`
	SSHSessionID   ID                        `json:"sshSessionID"`
	JavaRuntimeID  *ID                       `json:"javaRuntimeID,omitempty"`
	Name           string                    `json:"name"`
	DirectoryName  string                    `json:"directoryName"`
	Type           enums.MinecraftServerType `json:"type"`
	Version        string                    `json:"version"`
	RemotePath     string                    `json:"remotePath"`
	Group          string                    `json:"group"`
	Tags           []string                  `json:"tags"`
	Favourite      bool                      `json:"favourite"`
	State          enums.LifecycleState      `json:"state"`
	LaunchProfile  LaunchProfile             `json:"launchProfile"`
	FirewallPolicy enums.FirewallPolicy      `json:"firewallPolicy"`
	EULAAccepted   bool                      `json:"eulaAccepted"`
	CreatedAt      time.Time                 `json:"createdAt"`
	UpdatedAt      time.Time                 `json:"updatedAt"`
	DeletedAt      *time.Time                `json:"deletedAt,omitempty"`
}

// NewMinecraftServer creates a validated remote server draft or durable record.
func NewMinecraftServer(clock Clock, server MinecraftServer) (*MinecraftServer, error) {
	if clock == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Clock 不能为空")
	}
	now := clock.Now().UTC()
	id, err := NewID(now)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, "生成 Minecraft Server ID 失败", err)
	}
	server.ID = id
	server.CreatedAt = now
	server.UpdatedAt = now
	if server.DirectoryName == "" {
		server.DirectoryName = SafeServerDirectoryName(server.Name)
	}
	if server.State == "" {
		server.State = enums.LifecycleCreating
	}
	if server.FirewallPolicy == "" {
		server.FirewallPolicy = enums.FirewallPrompt
	}
	if err := server.Validate(); err != nil {
		return nil, err
	}
	return &server, nil
}

// Validate enforces SSH binding, path, distribution, launch, and lifecycle invariants.
func (s MinecraftServer) Validate() error {
	if !s.SSHSessionID.Valid() || strings.TrimSpace(s.Name) == "" || strings.TrimSpace(s.Version) == "" {
		return apperror.New(apperror.CodeValidationRequired, "Minecraft Server 必须绑定 SSH Session、名称和版本")
	}
	if !s.Type.Valid() || !s.State.Valid() || !s.FirewallPolicy.Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Minecraft Server 类型、状态或防火墙策略无效")
	}
	if s.JavaRuntimeID != nil && !s.JavaRuntimeID.Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Java Runtime ID 无效")
	}
	if !path.IsAbs(s.RemotePath) || s.RemotePath == "/" || strings.ContainsAny(s.RemotePath, "\r\n\x00") || s.DirectoryName != SafeServerDirectoryName(s.DirectoryName) {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Minecraft Server 远程路径或目录名无效")
	}
	if path.Clean(s.LaunchProfile.WorkingDirectory) != path.Clean(s.RemotePath) {
		return apperror.New(apperror.CodeValidationConflict, "LaunchProfile 工作目录必须与 Minecraft Server 远程目录一致")
	}
	return s.LaunchProfile.Validate()
}

// Validate checks memory ordering, working directory, Jar, and structured argument safety.
func (p LaunchProfile) Validate() error {
	if p.XmsMiB < 64 || p.XmxMiB < p.XmsMiB || p.XmxMiB > 1024*1024 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "LaunchProfile 内存范围无效")
	}
	jarPath := path.Clean(strings.TrimSpace(p.JarPath))
	if !path.IsAbs(p.WorkingDirectory) || strings.ContainsAny(p.WorkingDirectory, "\r\n\x00") || jarPath == "." || jarPath == ".." || path.IsAbs(jarPath) || strings.HasPrefix(jarPath, "../") || strings.ContainsAny(jarPath, "\r\n\x00") {
		return apperror.New(apperror.CodeValidationInvalidArgument, "LaunchProfile 工作目录必须为绝对路径且 Jar 必须为相对路径")
	}
	for _, argument := range append(append([]string(nil), p.JVMArguments...), p.ServerArguments...) {
		if strings.ContainsRune(argument, '\x00') || strings.ContainsAny(argument, "\r\n") {
			return apperror.New(apperror.CodeValidationInvalidArgument, "LaunchProfile 参数包含非法控制字符")
		}
	}
	return nil
}

// SafeServerDirectoryName converts a user-facing server name into a conservative path segment.
func SafeServerDirectoryName(name string) string {
	value := unsafeServerDirectoryCharacter.ReplaceAllString(strings.TrimSpace(name), "-")
	value = strings.Trim(value, ".-_")
	if len(value) > 80 {
		value = value[:80]
	}
	if value == "" {
		return "server"
	}
	return value
}

// CanSoftDelete reports whether the lifecycle permits metadata-only deletion.
func (s MinecraftServer) CanSoftDelete() bool {
	return s.DeletedAt == nil && s.State != enums.LifecycleRunning && s.State != enums.LifecycleStarting && s.State != enums.LifecycleStopping
}

// CanTransition reports whether the Server lifecycle permits the requested state change.
func (s MinecraftServer) CanTransition(next enums.LifecycleState) bool {
	if !next.Valid() || s.State == next {
		return false
	}
	switch s.State {
	case enums.LifecycleCreating:
		return next == enums.LifecycleInstalling || next == enums.LifecycleFailed || next == enums.LifecycleDeleted
	case enums.LifecycleInstalling:
		return next == enums.LifecycleStopped || next == enums.LifecycleFailed
	case enums.LifecycleReady, enums.LifecycleStopped:
		return next == enums.LifecycleStarting || next == enums.LifecycleUpdating || next == enums.LifecycleBackingUp || next == enums.LifecycleDeleted || next == enums.LifecycleFailed
	case enums.LifecycleStarting:
		return next == enums.LifecycleRunning || next == enums.LifecycleStopping || next == enums.LifecycleFailed
	case enums.LifecycleRunning:
		return next == enums.LifecycleStopping || next == enums.LifecycleUpdating || next == enums.LifecycleBackingUp || next == enums.LifecycleFailed
	case enums.LifecycleStopping:
		return next == enums.LifecycleStopped || next == enums.LifecycleFailed
	case enums.LifecycleUpdating, enums.LifecycleBackingUp:
		return next == enums.LifecycleStopped || next == enums.LifecycleRunning || next == enums.LifecycleFailed
	case enums.LifecycleFailed:
		return next == enums.LifecycleStarting || next == enums.LifecycleStopping || next == enums.LifecycleStopped || next == enums.LifecycleDeleted
	case enums.LifecycleDeleted:
		return next == enums.LifecycleStopped
	default:
		return false
	}
}

// Transition applies a validated lifecycle change with a UTC update timestamp.
func (s *MinecraftServer) Transition(clock Clock, next enums.LifecycleState) error {
	if s == nil || clock == nil || !s.CanTransition(next) {
		return apperror.New(apperror.CodeValidationConflict, "Minecraft Server 生命周期转换无效").WithDetails(map[string]any{
			"current": s.State, "next": next,
		})
	}
	s.State = next
	s.UpdatedAt = clock.Now().UTC()
	return nil
}
