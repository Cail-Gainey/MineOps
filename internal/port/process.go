package port

import (
	"context"
	"io"

	"github.com/Cail-Gainey/MineOps/internal/model"
)

// ProcessLaunchSpec 承载结构化的可执行参数与受控的远端路径。
type ProcessLaunchSpec struct {
	ServerID         model.ID
	SSHSessionID     model.ID
	Executable       string
	Arguments        []string
	Environment      map[string]string
	WorkingDirectory string
	ReadyPatterns    []string
}

// ProcessProbeResult 区分运行中、已退出、PID 复用不匹配与传输失败等证据。
type ProcessProbeResult struct {
	Identity model.RemoteProcessIdentity
	Ready    bool
	Output   string
}

// InteractiveProcess 是独立于通用 SSH 终端生命周期的控制台通道。
type InteractiveProcess interface {
	Identity() model.RemoteProcessIdentity
	Input(context.Context, []byte) error
	Attach(context.Context, int64, io.Writer) (int64, error)
	Detach() error
	Close() error
}

// ProcessController 启动受管远端进程,并优雅或强制停止它们。
type ProcessController interface {
	Start(context.Context, ProcessLaunchSpec) (InteractiveProcess, error)
	Stop(context.Context, model.RemoteProcessIdentity, bool) error
}

// ProcessProbe 校验 PID、Linux 启动 tick、命令指纹与工作目录。
type ProcessProbe interface {
	Probe(context.Context, model.RemoteProcessIdentity) (ProcessProbeResult, error)
}

// ProcessIdentityStore 持久化用于重启恢复与 PID 复用防护的最新身份。
type ProcessIdentityStore interface {
	Save(context.Context, *model.RemoteProcessIdentity) error
	GetByServer(context.Context, model.ID) (*model.RemoteProcessIdentity, error)
	DeleteByServer(context.Context, model.ID) error
}
