package service

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

const (
	playerDuplicateJoinWindow          = 5 * time.Second
	playerSynchronizationSlots         = 4
	playerSynchronizationInterval      = 15 * time.Second
	playerActivityDaemonInterval       = 2 * time.Second
	playerSynchronizationMaximumOutput = remotePlayerActivityMaximumOutput
)

// PlayerEventPublisher 发布带版本的玩家状态变更。
type PlayerEventPublisher interface {
	PublishPlayer(context.Context, model.PlayerEvent) error
}

// PlayerActivityManager 应用持久化的玩家证据与生命周期边界。
type PlayerActivityManager struct {
	clock     model.Clock
	store     repository.Store
	settings  *appsettings.Manager
	clients   *SSHClientFactory
	processes *RemoteProcessController
	logger    *applog.Logger
	publisher PlayerEventPublisher

	mu          sync.Mutex
	locks       map[model.ID]*sync.Mutex
	slots       chan struct{}
	started     chan model.ID
	stopped     chan model.ID
	failed      chan model.ID
	syncMu      sync.Mutex
	syncing     map[model.ID]bool
	failures    map[model.ID]int
	nextAttempt map[model.ID]time.Time
}

// NewPlayerActivityManager 创建玩家证据状态机与同步协调器。
func NewPlayerActivityManager(clock model.Clock, store repository.Store, settings *appsettings.Manager, clients *SSHClientFactory, processes *RemoteProcessController, logger *applog.Logger, publisher PlayerEventPublisher) (*PlayerActivityManager, error) {
	if clock == nil || store == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Player Activity Manager 依赖不能为空")
	}
	if logger == nil {
		logger = applog.Default()
	}
	return &PlayerActivityManager{
		clock: clock, store: store, settings: settings, clients: clients, processes: processes, logger: logger, publisher: publisher,
		locks: make(map[model.ID]*sync.Mutex), slots: make(chan struct{}, playerSynchronizationSlots),
		started: make(chan model.ID, 64), stopped: make(chan model.ID, 64), failed: make(chan model.ID, 64),
		syncing: make(map[model.ID]bool), failures: make(map[model.ID]int), nextAttempt: make(map[model.ID]time.Time),
	}, nil
}

// Ingest 原子且幂等地应用一个已认领的完整批次。
func (m *PlayerActivityManager) Ingest(ctx context.Context, events []model.PlayerActivityEvent, dropped uint64) error {
	if len(events) == 0 && dropped == 0 {
		return nil
	}
	if len(events) > maximumPlayerActivityBatch {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家事件批次超过限制")
	}
	for index := range events {
		if err := events[index].Validate(); err != nil {
			return err
		}
	}
	serverID := events[0].ServerID
	for index := range events {
		if events[index].ServerID != serverID {
			return apperror.New(apperror.CodeValidationInvalidArgument, "玩家事件批次必须属于同一 Server")
		}
	}
	lock := m.serverLock(serverID)
	lock.Lock()
	defer lock.Unlock()

	sort.SliceStable(events, func(left, right int) bool {
		if events[left].ObservedAt.Equal(events[right].ObservedAt) {
			return events[left].SourceSequence < events[right].SourceSequence
		}
		return events[left].ObservedAt.Before(events[right].ObservedAt)
	})
	now := m.clock.Now().UTC()
	changed := make(map[model.ID]string)
	err := m.store.Transaction(ctx, func(registry repository.Registry) error {
		players := registry.Players()
		for index := range events {
			identity, err := m.resolveEventIdentity(ctx, players, events[index], now)
			if err != nil {
				return err
			}
			events[index].PlayerIdentityID = &identity.ID
			inserted, err := players.InsertEvent(ctx, &events[index])
			if err != nil || !inserted {
				if err != nil {
					return err
				}
				continue
			}
			if err := m.applyEvent(ctx, players, identity, events[index], now); err != nil {
				return err
			}
			changed[identity.ID] = events[index].Type.String()
		}
		status, err := players.GetCollectorStatus(ctx, serverID)
		if err != nil && !isPlayerNotFound(err) {
			return err
		}
		if status == nil {
			status = &model.PlayerCollectorStatus{ServerID: serverID, Accuracy: enums.PlayerAccuracyExact, SchemaVersion: model.PlayerActivitySchemaVersion}
		}
		if len(events) > 0 {
			last := events[len(events)-1]
			status.ProcessIdentityID = &last.ProcessIdentityID
			status.LastSourceSequence = last.SourceSequence
			status.LastObservedAt = &last.ObservedAt
		}
		status.DroppedEventCount += dropped
		status.LastSynchronizedAt = &now
		status.UpdatedAt = now
		status.LastError = ""
		if dropped > 0 {
			status.Accuracy = enums.PlayerAccuracyIncomplete
		}
		return players.SaveCollectorStatus(ctx, status)
	})
	if err != nil {
		return err
	}
	for playerID, eventType := range changed {
		m.publish(ctx, model.PlayerEvent{Version: 1, Type: eventType, ServerID: serverID, PlayerIdentityID: playerID, EmittedAt: now})
		if eventType == enums.PlayerActivityLeave.String() {
			m.publish(ctx, model.PlayerEvent{Version: 1, Type: "statistics", ServerID: serverID, PlayerIdentityID: playerID, EmittedAt: now})
		}
	}
	return nil
}

// ReconcileServer 在一个已校验的生命周期边界上关闭全部未结束会话。
func (m *PlayerActivityManager) ReconcileServer(ctx context.Context, serverID model.ID, processIdentityID model.ID, boundary time.Time, reason enums.PlayerSessionCloseReason, accuracy enums.PlayerActivityAccuracy) error {
	if !serverID.Valid() || !processIdentityID.Valid() || boundary.IsZero() || !reason.Valid() || !accuracy.Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家生命周期结算边界无效")
	}
	lock := m.serverLock(serverID)
	lock.Lock()
	defer lock.Unlock()
	changed := make([]model.ID, 0)
	err := m.store.Transaction(ctx, func(registry repository.Registry) error {
		players := registry.Players()
		sessions, err := players.ListSessions(ctx, repository.PlayerSessionQuery{ServerID: serverID, OpenOnly: true, Limit: 200})
		if err != nil {
			return err
		}
		for index := range sessions {
			if sessions[index].ProcessIdentityID != processIdentityID {
				continue
			}
			if err := closePlayerSession(ctx, players, &sessions[index], boundary.UTC(), reason, accuracy, m.clock.Now().UTC()); err != nil {
				return err
			}
			changed = append(changed, sessions[index].PlayerIdentityID)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, playerID := range changed {
		m.publish(ctx, model.PlayerEvent{Version: 1, Type: "reconciled", ServerID: serverID, PlayerIdentityID: playerID, EmittedAt: m.clock.Now().UTC()})
	}
	return nil
}

// MarkIncomplete 记录被丢弃的远端证据,直到建立起可信的进程边界。
func (m *PlayerActivityManager) MarkIncomplete(ctx context.Context, serverID model.ID, processIdentityID *model.ID, dropped uint64, message string) error {
	if !serverID.Valid() || dropped == 0 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家丢失事件状态无效")
	}
	now := m.clock.Now().UTC()
	status, err := m.store.Players().GetCollectorStatus(ctx, serverID)
	if err != nil && !isPlayerNotFound(err) {
		return err
	}
	if status == nil {
		status = &model.PlayerCollectorStatus{ServerID: serverID, SchemaVersion: model.PlayerActivitySchemaVersion}
	}
	status.ProcessIdentityID = processIdentityID
	status.DroppedEventCount += dropped
	status.Accuracy = enums.PlayerAccuracyIncomplete
	status.LastError = message
	status.UpdatedAt = now
	return m.store.Players().SaveCollectorStatus(ctx, status)
}

func (m *PlayerActivityManager) applyEvent(ctx context.Context, players repository.PlayerRepository, identity *model.PlayerIdentity, event model.PlayerActivityEvent, now time.Time) error {
	open, err := players.GetOpenSession(ctx, event.ServerID, identity.ID)
	if err != nil && !isPlayerNotFound(err) {
		return err
	}
	if event.Type == enums.PlayerActivityJoin {
		if open != nil {
			if open.ProcessIdentityID == event.ProcessIdentityID && event.ObservedAt.Sub(open.JoinedAt) <= playerDuplicateJoinWindow {
				return nil
			}
			if event.ObservedAt.Before(open.JoinedAt) {
				return apperror.New(apperror.CodeValidationInvalidArgument, "玩家 Join 早于现有开放会话")
			}
			if err := closePlayerSession(ctx, players, open, event.ObservedAt, enums.PlayerCloseDuplicateJoin, enums.PlayerAccuracyEstimated, now); err != nil {
				return err
			}
		}
		sessionID, err := model.NewID(now)
		if err != nil {
			return err
		}
		return players.CreateSession(ctx, &model.PlayerSession{
			ID: sessionID, ServerID: event.ServerID, PlayerIdentityID: identity.ID, ProcessIdentityID: event.ProcessIdentityID,
			JoinedAt: event.ObservedAt.UTC(), State: enums.PlayerSessionOpen, Accuracy: enums.PlayerAccuracyExact,
			CreatedAt: now, UpdatedAt: now, SchemaVersion: model.PlayerActivitySchemaVersion,
		})
	}
	if open == nil {
		return nil
	}
	if open.ProcessIdentityID != event.ProcessIdentityID {
		return nil
	}
	return closePlayerSession(ctx, players, open, event.ObservedAt.UTC(), enums.PlayerCloseLeave, enums.PlayerAccuracyExact, now)
}

func (m *PlayerActivityManager) resolveEventIdentity(ctx context.Context, players repository.PlayerRepository, event model.PlayerActivityEvent, now time.Time) (*model.PlayerIdentity, error) {
	identity, err := players.FindIdentity(ctx, repository.PlayerIdentityQuery{ServerID: event.ServerID, NormalizedName: event.NormalizedName})
	if err == nil {
		return identity, nil
	}
	if !isPlayerNotFound(err) {
		return nil, err
	}
	id, err := model.NewID(now)
	if err != nil {
		return nil, err
	}
	identity = &model.PlayerIdentity{
		ID: id, ServerID: event.ServerID, CurrentName: event.PlayerName, NormalizedName: event.NormalizedName,
		Kind: enums.PlayerIdentityNameOnly, CreatedAt: now, UpdatedAt: now, SchemaVersion: model.PlayerActivitySchemaVersion,
	}
	if err := players.CreateIdentity(ctx, identity); err != nil {
		return nil, err
	}
	return identity, nil
}

func closePlayerSession(ctx context.Context, players repository.PlayerRepository, session *model.PlayerSession, leftAt time.Time, reason enums.PlayerSessionCloseReason, accuracy enums.PlayerActivityAccuracy, now time.Time) error {
	if leftAt.Before(session.JoinedAt) {
		return apperror.New(apperror.CodeValidationInvalidArgument, "玩家离开时间早于加入时间")
	}
	session.LeftAt = &leftAt
	session.DurationSeconds = int64(leftAt.Sub(session.JoinedAt) / time.Second)
	session.CloseReason = reason
	session.Accuracy = accuracy
	session.State = enums.PlayerSessionClosed
	if reason != enums.PlayerCloseLeave && reason != enums.PlayerCloseServerStopped {
		session.State = enums.PlayerSessionInterrupted
	}
	session.UpdatedAt = now
	if err := players.UpdateSession(ctx, session); err != nil {
		return err
	}
	statistics, err := players.GetStatistics(ctx, session.PlayerIdentityID)
	if err != nil && !isPlayerNotFound(err) {
		return err
	}
	if statistics == nil {
		statistics = &model.PlayerStatistics{PlayerIdentityID: session.PlayerIdentityID, ServerID: session.ServerID, Accuracy: accuracy, SchemaVersion: model.PlayerActivitySchemaVersion}
	}
	statistics.TotalDurationSeconds += session.DurationSeconds
	statistics.CompletedSessionCount++
	statistics.LongestSessionSeconds = max(statistics.LongestSessionSeconds, session.DurationSeconds)
	if statistics.FirstActivityAt == nil || session.JoinedAt.Before(*statistics.FirstActivityAt) {
		joined := session.JoinedAt
		statistics.FirstActivityAt = &joined
	}
	if statistics.LastActivityAt == nil || leftAt.After(*statistics.LastActivityAt) {
		left := leftAt
		statistics.LastActivityAt = &left
	}
	statistics.Accuracy = lessAccuratePlayerState(statistics.Accuracy, accuracy)
	statistics.UpdatedAt = now
	return players.SaveStatistics(ctx, statistics)
}

func lessAccuratePlayerState(left, right enums.PlayerActivityAccuracy) enums.PlayerActivityAccuracy {
	rank := map[enums.PlayerActivityAccuracy]int{
		enums.PlayerAccuracyExact: 0, enums.PlayerAccuracyServerBoundary: 1, enums.PlayerAccuracyReconstructed: 2,
		enums.PlayerAccuracyEstimated: 3, enums.PlayerAccuracyIncomplete: 4,
	}
	if rank[right] > rank[left] {
		return right
	}
	if left.Valid() {
		return left
	}
	return right
}

func (m *PlayerActivityManager) serverLock(serverID model.ID) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	lock := m.locks[serverID]
	if lock == nil {
		lock = &sync.Mutex{}
		m.locks[serverID] = lock
	}
	return lock
}

func (m *PlayerActivityManager) publish(ctx context.Context, event model.PlayerEvent) {
	if m.publisher == nil {
		return
	}
	if err := m.publisher.PublishPlayer(ctx, event); err != nil && !errors.Is(err, context.Canceled) {
		m.logger.Warn(ctx, "发布玩家状态事件失败", applog.Fields{"server_id": event.ServerID.String(), "error": err.Error()})
	}
}

func isPlayerNotFound(err error) bool {
	return err != nil && apperror.ToDTO(err).Code == apperror.CodeIONotFound.String()
}
