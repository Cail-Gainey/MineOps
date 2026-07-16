package services

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"github.com/Cail-Gainey/MineOps/internal/service"
	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/crypto/ssh"
)

func init() {
	application.RegisterEvent[TerminalEvent](constants.TerminalEventName)
}

// TerminalEvent is the versioned Opened/Data/Closed/Error event contract for one PTY.
type TerminalEvent struct {
	Version   int                    `json:"version"`
	Type      string                 `json:"type"`
	SessionID string                 `json:"sessionID"`
	Data      string                 `json:"data,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Error     *apperror.DTO          `json:"error,omitempty"`
	Session   *model.TerminalSession `json:"session,omitempty"`
}

// TerminalSessionResult contains one PTY session or a stable error.
type TerminalSessionResult struct {
	Session *model.TerminalSession `json:"session,omitempty"`
	Error   *apperror.DTO          `json:"error,omitempty"`
}

type activeTerminal struct {
	model        model.TerminalSession
	client       *service.SSHClient
	session      *ssh.Session
	stdin        io.WriteCloser
	cancel       context.CancelFunc
	output       chan []byte
	wait         sync.WaitGroup
	closeOnce    sync.Once
	inputMu      sync.Mutex
	droppedBytes atomic.Int64
}

// TerminalService owns authenticated SSH PTYs, bounded output queues, and Wails events.
type TerminalService struct {
	clients  *service.SSHClientFactory
	store    repository.Store
	settings *appsettings.Manager
	logger   *applog.Logger

	mu       sync.Mutex
	rootCtx  context.Context
	app      *application.App
	sessions map[model.ID]*activeTerminal
}

// NewTerminalService creates the desktop SSH PTY facade.
func NewTerminalService(clients *service.SSHClientFactory, store repository.Store, settings *appsettings.Manager, logger *applog.Logger) *TerminalService {
	return &TerminalService{
		clients: clients, store: store, settings: settings, logger: logger,
		sessions: make(map[model.ID]*activeTerminal),
	}
}

// ServiceStartup captures the root application context and event emitter.
func (s *TerminalService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	s.rootCtx = ctx
	s.app = application.Get()
	s.mu.Unlock()
	return nil
}

// ServiceShutdown closes all PTYs and waits for their goroutines.
func (s *TerminalService) ServiceShutdown() error {
	return s.closeAll()
}

// Open authenticates an SSH Session, requests xterm-256color PTY, and starts bounded output streaming.
func (s *TerminalService) Open(ctx context.Context, sshSessionID string, columns, rows int) (result TerminalSessionResult) {
	defer s.recoverSession(ctx, "TerminalService.Open", &result)
	if columns < 1 || rows < 1 || columns > 1000 || rows > 500 {
		dto := apperror.ToDTO(apperror.New(apperror.CodeValidationInvalidArgument, "Terminal 尺寸无效"))
		return TerminalSessionResult{Error: &dto}
	}
	requestedSSHSessionID := strings.TrimSpace(sshSessionID)
	sshSession, err := s.store.SSHSessions().Get(ctx, model.ID(requestedSSHSessionID))
	if err != nil {
		var applicationError *apperror.Error
		if errors.As(err, &applicationError) && applicationError.Code == apperror.CodeIONotFound {
			_ = applicationError.WithDetails(map[string]any{
				"requestedSSHSessionID":       requestedSSHSessionID,
				"requestedSSHSessionIDLength": len(requestedSSHSessionID),
				"requestedSSHSessionIDValid":  model.ID(requestedSSHSessionID).Valid(),
			})
		}
		dto := apperror.ToDTO(err)
		return TerminalSessionResult{Error: &dto}
	}
	sshSettings := s.settings.Snapshot().SSH
	client, err := s.clients.Connect(ctx, sshSession, sshSettings)
	if err != nil {
		dto := apperror.ToDTO(err)
		return TerminalSessionResult{Error: &dto}
	}
	pty, err := client.Raw().NewSession()
	if err != nil {
		_ = client.Close()
		dto := apperror.ToDTO(apperror.Wrap(apperror.CodeSSHConnectionFailed, "创建 SSH PTY Session 失败", err))
		return TerminalSessionResult{Error: &dto}
	}
	stdin, err := pty.StdinPipe()
	if err != nil {
		_ = pty.Close()
		_ = client.Close()
		dto := apperror.ToDTO(apperror.Wrap(apperror.CodeSSHConnectionFailed, "创建 PTY 输入流失败", err))
		return TerminalSessionResult{Error: &dto}
	}
	stdout, err := pty.StdoutPipe()
	if err != nil {
		_ = pty.Close()
		_ = client.Close()
		dto := apperror.ToDTO(apperror.Wrap(apperror.CodeSSHConnectionFailed, "创建 PTY 输出流失败", err))
		return TerminalSessionResult{Error: &dto}
	}
	stderr, err := pty.StderrPipe()
	if err != nil {
		_ = pty.Close()
		_ = client.Close()
		dto := apperror.ToDTO(apperror.Wrap(apperror.CodeSSHConnectionFailed, "创建 PTY 错误流失败", err))
		return TerminalSessionResult{Error: &dto}
	}
	if err := pty.RequestPty(sshSettings.PTYTerminalType, rows, columns, ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}); err != nil {
		_ = pty.Close()
		_ = client.Close()
		dto := apperror.ToDTO(apperror.Wrap(apperror.CodeSSHConnectionFailed, "请求远程 PTY 失败", err))
		return TerminalSessionResult{Error: &dto}
	}
	if err := pty.Shell(); err != nil {
		_ = pty.Close()
		_ = client.Close()
		dto := apperror.ToDTO(apperror.Wrap(apperror.CodeSSHConnectionFailed, "启动远程 Shell 失败", err))
		return TerminalSessionResult{Error: &dto}
	}
	now := time.Now().UTC()
	id, err := model.NewID(now)
	if err != nil {
		_ = pty.Close()
		_ = client.Close()
		dto := apperror.ToDTO(apperror.Wrap(apperror.CodeInternal, "生成 Terminal Session ID 失败", err))
		return TerminalSessionResult{Error: &dto}
	}
	s.mu.Lock()
	rootCtx := s.rootCtx
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	sessionCtx, cancel := context.WithCancel(rootCtx)
	active := &activeTerminal{
		model:  model.TerminalSession{ID: id, SSHSessionID: sshSession.ID, State: model.TerminalOpen, Columns: columns, Rows: rows, OpenedAt: now},
		client: client, session: pty, stdin: stdin, cancel: cancel, output: make(chan []byte, 256),
	}
	s.sessions[id] = active
	s.mu.Unlock()
	active.wait.Add(4)
	go s.readOutput(sessionCtx, active, stdout)
	go s.readOutput(sessionCtx, active, stderr)
	go s.emitOutput(sessionCtx, active)
	go s.waitRemote(active)
	s.emit(TerminalEvent{Version: constants.DesktopProtocolVersion, Type: "opened", SessionID: id.String(), Session: &active.model})
	copy := active.model
	return TerminalSessionResult{Session: &copy}
}

// Input writes ordered UTF-8 terminal input to one PTY.
func (s *TerminalService) Input(ctx context.Context, sessionID, data string) (result ActionResult) {
	active, err := s.get(sessionID)
	if err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	active.inputMu.Lock()
	_, writeErr := io.WriteString(active.stdin, data)
	active.inputMu.Unlock()
	if writeErr != nil {
		dto := apperror.ToDTO(apperror.Wrap(apperror.CodeSSHConnectionFailed, "写入 Terminal 输入失败", writeErr))
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// Resize applies a remote window-change request and updates session dimensions.
func (s *TerminalService) Resize(ctx context.Context, sessionID string, columns, rows int) (result ActionResult) {
	if columns < 1 || rows < 1 || columns > 1000 || rows > 500 {
		dto := apperror.ToDTO(apperror.New(apperror.CodeValidationInvalidArgument, "Terminal 尺寸无效"))
		return ActionResult{Error: &dto}
	}
	active, err := s.get(sessionID)
	if err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	if err := active.session.WindowChange(rows, columns); err != nil {
		dto := apperror.ToDTO(apperror.Wrap(apperror.CodeSSHConnectionFailed, "调整远程 PTY 尺寸失败", err))
		return ActionResult{Error: &dto}
	}
	active.model.Columns = columns
	active.model.Rows = rows
	return ActionResult{}
}

// Close terminates one PTY and emits its final event.
func (s *TerminalService) Close(ctx context.Context, sessionID string) (result ActionResult) {
	active, err := s.get(sessionID)
	if err != nil {
		if apperror.ToDTO(err).Code == apperror.CodeIONotFound.String() {
			return ActionResult{}
		}
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	s.close(active, nil)
	return ActionResult{}
}

// CloseAll terminates every active PTY owned by the desktop process.
func (s *TerminalService) CloseAll(ctx context.Context) (result ActionResult) {
	if err := s.closeAll(); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

func (s *TerminalService) readOutput(ctx context.Context, active *activeTerminal, reader io.Reader) {
	defer active.wait.Done()
	buffer := make([]byte, 16*1024)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			chunk := append([]byte(nil), buffer[:count]...)
			select {
			case active.output <- chunk:
			default:
				active.droppedBytes.Add(int64(len(chunk)))
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				s.close(active, apperror.Wrap(apperror.CodeSSHConnectionFailed, "读取 Terminal 输出失败", err))
			}
			return
		}
	}
}

func (s *TerminalService) emitOutput(ctx context.Context, active *activeTerminal) {
	defer active.wait.Done()
	ticker := time.NewTicker(16 * time.Millisecond)
	defer ticker.Stop()
	batch := make([]byte, 0, 64*1024)
	for {
		select {
		case <-ctx.Done():
			if len(batch) > 0 {
				s.emitData(active.model.ID, batch)
			}
			return
		case chunk := <-active.output:
			batch = append(batch, chunk...)
			if len(batch) >= 64*1024 {
				s.emitData(active.model.ID, batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if dropped := active.droppedBytes.Swap(0); dropped > 0 {
				s.emit(TerminalEvent{
					Version: constants.DesktopProtocolVersion, Type: "dropped", SessionID: active.model.ID.String(),
					Message: "Terminal 输出队列已丢弃 " + strconv.FormatInt(dropped, 10) + " 字节",
				})
			}
			if len(batch) > 0 {
				s.emitData(active.model.ID, batch)
				batch = batch[:0]
			}
		}
	}
}

func (s *TerminalService) waitRemote(active *activeTerminal) {
	defer active.wait.Done()
	err := active.session.Wait()
	if err != nil && !errors.Is(err, io.EOF) {
		s.close(active, apperror.Wrap(apperror.CodeSSHConnectionFailed, "远程 Terminal 已异常退出", err))
		return
	}
	s.close(active, nil)
}

func (s *TerminalService) emitData(sessionID model.ID, data []byte) {
	s.emit(TerminalEvent{
		Version: constants.DesktopProtocolVersion, Type: "data", SessionID: sessionID.String(),
		Data: base64.StdEncoding.EncodeToString(data),
	})
}

func (s *TerminalService) get(sessionID string) (*activeTerminal, error) {
	s.mu.Lock()
	active := s.sessions[model.ID(sessionID)]
	s.mu.Unlock()
	if active == nil {
		return nil, apperror.New(apperror.CodeIONotFound, "Terminal Session 不存在")
	}
	return active, nil
}

func (s *TerminalService) close(active *activeTerminal, failure error) {
	active.closeOnce.Do(func() {
		active.cancel()
		_ = active.stdin.Close()
		_ = active.session.Close()
		_ = active.client.Close()
		now := time.Now().UTC()
		active.model.ClosedAt = &now
		active.model.State = model.TerminalClosed
		if failure != nil {
			active.model.State = model.TerminalFailed
		}
		s.mu.Lock()
		delete(s.sessions, active.model.ID)
		s.mu.Unlock()
		event := TerminalEvent{Version: constants.DesktopProtocolVersion, Type: "closed", SessionID: active.model.ID.String(), Session: &active.model}
		if failure != nil {
			dto := apperror.ToDTO(failure)
			event.Type = "error"
			event.Error = &dto
		}
		s.emit(event)
	})
}

func (s *TerminalService) closeAll() error {
	s.mu.Lock()
	activeSessions := make([]*activeTerminal, 0, len(s.sessions))
	for _, active := range s.sessions {
		activeSessions = append(activeSessions, active)
	}
	s.mu.Unlock()
	for _, active := range activeSessions {
		s.close(active, nil)
	}
	for _, active := range activeSessions {
		active.wait.Wait()
	}
	return nil
}

func (s *TerminalService) emit(event TerminalEvent) {
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	if app != nil {
		app.Event.Emit(constants.TerminalEventName, event)
	}
}

func (s *TerminalService) recoverSession(ctx context.Context, boundary string, result *TerminalSessionResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}
