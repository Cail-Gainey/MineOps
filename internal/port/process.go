package port

import (
	"context"
	"io"

	"github.com/Cail-Gainey/MineOps/internal/model"
)

// ProcessLaunchSpec contains structured executable arguments and controlled remote paths.
type ProcessLaunchSpec struct {
	ServerID         model.ID
	SSHSessionID     model.ID
	Executable       string
	Arguments        []string
	Environment      map[string]string
	WorkingDirectory string
	ReadyPatterns    []string
}

// ProcessProbeResult distinguishes running, exited, mismatched PID reuse, and transport failure evidence.
type ProcessProbeResult struct {
	Identity model.RemoteProcessIdentity
	Ready    bool
	Output   string
}

// InteractiveProcess is a console channel independent from the general SSH Terminal lifecycle.
type InteractiveProcess interface {
	Identity() model.RemoteProcessIdentity
	Input(context.Context, []byte) error
	Attach(context.Context, int64, io.Writer) (int64, error)
	Detach() error
	Close() error
}

// ProcessController starts and gracefully or forcibly stops managed remote processes.
type ProcessController interface {
	Start(context.Context, ProcessLaunchSpec) (InteractiveProcess, error)
	Stop(context.Context, model.RemoteProcessIdentity, bool) error
}

// ProcessProbe verifies PID, Linux start ticks, command fingerprint, and working directory.
type ProcessProbe interface {
	Probe(context.Context, model.RemoteProcessIdentity) (ProcessProbeResult, error)
}

// ProcessIdentityStore persists the latest identity used for restart recovery and PID reuse protection.
type ProcessIdentityStore interface {
	Save(context.Context, *model.RemoteProcessIdentity) error
	GetByServer(context.Context, model.ID) (*model.RemoteProcessIdentity, error)
	DeleteByServer(context.Context, model.ID) error
}
