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
	"github.com/Cail-Gainey/MineOps/internal/global/constants"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/port"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"github.com/Cail-Gainey/MineOps/internal/service"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	application.RegisterEvent[ConsoleEvent](constants.ConsoleEventName)
}

// ConsoleEvent 是 Server 控制台的带版本事件契约:Opened/Data/Dropped/Closed/Error。
type ConsoleEvent struct {
	Version   int                   `json:"version"`
	Type      string                `json:"type"`
	SessionID string                `json:"sessionID"`
	Data      string                `json:"data,omitempty"`
	Offset    int64                 `json:"offset"`
	Message   string                `json:"message,omitempty"`
	Error     *apperror.DTO         `json:"error,omitempty"`
	Session   *model.ConsoleSession `json:"session,omitempty"`
}

// ConsoleSessionResult 承载一次控制台附着或稳定错误。
type ConsoleSessionResult struct {
	Session *model.ConsoleSession `json:"session,omitempty"`
	Error   *apperror.DTO         `json:"error,omitempty"`
}

type activeConsole struct {
	model      model.ConsoleSession
	process    port.InteractiveProcess
	cancel     context.CancelFunc
	output     chan []byte
	wait       sync.WaitGroup
	closeOnce  sync.Once
	inputMu    sync.Mutex
	dropped    atomic.Int64
	lastOffset atomic.Int64
}

// ConsoleService 独立持有 Server 控制台附着,但不持有远端进程的生命周期。
type ConsoleService struct {
	processes *service.RemoteProcessController
	store     repository.Store
	logger    *applog.Logger

	mu       sync.Mutex
	rootCtx  context.Context
	app      *application.App
	sessions map[model.ID]*activeConsole
}

// NewConsoleService 创建只读的 tmux 控制台门面。
func NewConsoleService(processes *service.RemoteProcessController, store repository.Store, logger *applog.Logger) *ConsoleService {
	return &ConsoleService{processes: processes, store: store, logger: logger, sessions: make(map[model.ID]*activeConsole)}
}

// ServiceStartup 捕获应用上下文与 Wails 事件发射器。
func (s *ConsoleService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	s.rootCtx, s.app = ctx, application.Get()
	s.mu.Unlock()
	return nil
}

// ServiceShutdown 断开全部控制台视图,不停止远端 Server。
func (s *ConsoleService) ServiceShutdown() error {
	s.mu.Lock()
	active := make([]*activeConsole, 0, len(s.sessions))
	for _, session := range s.sessions {
		active = append(active, session)
	}
	s.mu.Unlock()
	for _, session := range active {
		s.close(session, nil)
	}
	for _, session := range active {
		session.wait.Wait()
	}
	return nil
}

// Open 把桌面视图附着到一个由 MineOps 管理的运行中 tmux 会话。
func (s *ConsoleService) Open(ctx context.Context, serverID string, offset int64) (result ConsoleSessionResult) {
	defer s.recoverSession(ctx, &result)
	if offset < 0 {
		dto := apperror.ToDTO(apperror.New(apperror.CodeValidationInvalidArgument, "Console Offset 无效"))
		return ConsoleSessionResult{Error: &dto}
	}
	if _, err := s.store.MinecraftServers().Get(ctx, model.ID(serverID), false); err != nil {
		dto := apperror.ToDTO(err)
		return ConsoleSessionResult{Error: &dto}
	}
	identity, err := s.store.ProcessIdentities().GetByServer(ctx, model.ID(serverID))
	if err != nil {
		if apperror.ToDTO(err).Code == apperror.CodeIONotFound.String() {
			err = apperror.New(apperror.CodeValidationConflict, "服务器尚未创建可附加的 tmux Console")
		}
		dto := apperror.ToDTO(err)
		return ConsoleSessionResult{Error: &dto}
	}
	probe, err := s.processes.Probe(ctx, *identity)
	if err != nil {
		dto := apperror.ToDTO(err)
		return ConsoleSessionResult{Error: &dto}
	}
	if probe.Identity.State != "running" || probe.Identity.TmuxSession == "" {
		dto := apperror.ToDTO(apperror.New(apperror.CodeValidationConflict, "服务器 tmux Console 当前不可附加"))
		return ConsoleSessionResult{Error: &dto}
	}
	process := s.processes.Attach(probe.Identity)
	now := time.Now().UTC()
	id, err := model.NewID(now)
	if err != nil {
		dto := apperror.ToDTO(err)
		return ConsoleSessionResult{Error: &dto}
	}
	s.mu.Lock()
	rootCtx := s.rootCtx
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	consoleCtx, cancel := context.WithCancel(rootCtx)
	active := &activeConsole{
		model: model.ConsoleSession{
			ID: id, ServerID: model.ID(serverID), ProcessIdentityID: probe.Identity.ID,
			Offset: 0, State: "open", ReadOnly: false, OpenedAt: now,
		},
		process: process, cancel: cancel, output: make(chan []byte, 256),
	}
	active.lastOffset.Store(0)
	s.sessions[id] = active
	s.mu.Unlock()
	active.wait.Add(2)
	go s.attach(consoleCtx, active)
	go s.emitOutput(consoleCtx, active)
	s.emit(ConsoleEvent{Version: constants.DesktopProtocolVersion, Type: "opened", SessionID: id.String(), Offset: 0, Session: &active.model})
	copy := active.model
	return ConsoleSessionResult{Session: &copy}
}

// Input 通过受管 tmux 会话向已附着的 Java 进程发送命令。
func (s *ConsoleService) Input(ctx context.Context, sessionID, data string) (result ActionResult) {
	active, err := s.get(sessionID)
	if err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	data = strings.ReplaceAll(data, "\r", "\n")
	active.inputMu.Lock()
	err = active.process.Input(ctx, []byte(data))
	active.inputMu.Unlock()
	if err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// Detach 关闭一次桌面附着,不停止 Server 进程。
func (s *ConsoleService) Detach(ctx context.Context, sessionID string) (result ActionResult) {
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

// Close 是 Detach 的幂等别名,永不停止远端 Server。
func (s *ConsoleService) Close(ctx context.Context, sessionID string) ActionResult {
	return s.Detach(ctx, sessionID)
}

func (s *ConsoleService) attach(ctx context.Context, active *activeConsole) {
	defer active.wait.Done()
	writer := &consoleOutputWriter{active: active}
	offset, err := active.process.Attach(ctx, active.model.Offset, writer)
	active.lastOffset.Store(offset)
	if err != nil && !errors.Is(err, context.Canceled) {
		s.close(active, err)
		return
	}
	s.close(active, nil)
}

func (s *ConsoleService) emitOutput(ctx context.Context, active *activeConsole) {
	defer active.wait.Done()
	ticker := time.NewTicker(33 * time.Millisecond)
	defer ticker.Stop()
	batch := make([]byte, 0, 64*1024)
	for {
		select {
		case <-ctx.Done():
			if len(batch) > 0 {
				s.emitData(active, batch)
			}
			return
		case chunk := <-active.output:
			batch = append(batch, chunk...)
			if len(batch) >= 64*1024 {
				s.emitData(active, batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if dropped := active.dropped.Swap(0); dropped > 0 {
				s.emit(ConsoleEvent{Version: constants.DesktopProtocolVersion, Type: "dropped", SessionID: active.model.ID.String(), Offset: active.lastOffset.Load(), Message: "Console 输出队列已丢弃 " + strconv.FormatInt(dropped, 10) + " 字节"})
			}
			if len(batch) > 0 {
				s.emitData(active, batch)
				batch = batch[:0]
			}
		}
	}
}

func (s *ConsoleService) emitData(active *activeConsole, data []byte) {
	s.emit(ConsoleEvent{
		Version: constants.DesktopProtocolVersion, Type: "data", SessionID: active.model.ID.String(),
		Data: base64.StdEncoding.EncodeToString(data), Offset: active.lastOffset.Load(),
	})
}

func (s *ConsoleService) get(sessionID string) (*activeConsole, error) {
	s.mu.Lock()
	active := s.sessions[model.ID(sessionID)]
	s.mu.Unlock()
	if active == nil {
		return nil, apperror.New(apperror.CodeIONotFound, "Console Session 不存在")
	}
	return active, nil
}

func (s *ConsoleService) close(active *activeConsole, failure error) {
	active.closeOnce.Do(func() {
		active.cancel()
		_ = active.process.Detach()
		_ = active.process.Close()
		now := time.Now().UTC()
		active.model.State = "closed"
		active.model.Offset = active.lastOffset.Load()
		active.model.ClosedAt = &now
		s.mu.Lock()
		delete(s.sessions, active.model.ID)
		s.mu.Unlock()
		event := ConsoleEvent{Version: constants.DesktopProtocolVersion, Type: "closed", SessionID: active.model.ID.String(), Offset: active.model.Offset, Session: &active.model}
		if failure != nil {
			dto := apperror.ToDTO(failure)
			event.Type, event.Error = "error", &dto
		}
		s.emit(event)
	})
}

func (s *ConsoleService) emit(event ConsoleEvent) {
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	if app != nil {
		app.Event.Emit(constants.ConsoleEventName, event)
	}
}

func (s *ConsoleService) recoverSession(ctx context.Context, result *ConsoleSessionResult) {
	var err error
	apperror.Recover(ctx, s.logger, "ConsoleService.Open", &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

type consoleOutputWriter struct{ active *activeConsole }

// Write 把控制台输出编码后作为事件推送给前端。
func (w *consoleOutputWriter) Write(value []byte) (int, error) {
	chunk := append([]byte(nil), value...)
	w.active.lastOffset.Add(int64(len(chunk)))
	select {
	case w.active.output <- chunk:
	default:
		w.active.dropped.Add(int64(len(chunk)))
	}
	return len(value), nil
}

var _ io.Writer = (*consoleOutputWriter)(nil)
