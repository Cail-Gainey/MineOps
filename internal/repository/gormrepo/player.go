package gormrepo

import (
	"context"
	"errors"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PlayerIdentityRecord is the encrypted SQLite player identity representation.
type PlayerIdentityRecord struct {
	ID                   string  `gorm:"primaryKey;size:36"`
	ServerID             string  `gorm:"index:idx_player_identity_name,priority:1;index:idx_player_identity_uuid,priority:1;uniqueIndex:idx_player_identity_uuid_unique,priority:1,where:uuid IS NOT NULL;uniqueIndex:idx_player_identity_name_only_unique,priority:1,where:uuid IS NULL;size:36"`
	UUID                 *string `gorm:"index:idx_player_identity_uuid,priority:2;uniqueIndex:idx_player_identity_uuid_unique,priority:2,where:uuid IS NOT NULL;size:36"`
	CurrentName          string  `gorm:"size:64"`
	NormalizedName       string  `gorm:"index:idx_player_identity_name,priority:2;uniqueIndex:idx_player_identity_name_only_unique,priority:2,where:uuid IS NULL;size:64"`
	Kind                 string  `gorm:"index:idx_player_identity_name,priority:3;size:24"`
	CreatedAt, UpdatedAt time.Time
	SchemaVersion        int
}

// PlayerActivityEventRecord is immutable idempotent collected player evidence.
type PlayerActivityEventRecord struct {
	ID                         string  `gorm:"primaryKey;size:36"`
	ServerID                   string  `gorm:"index:idx_player_event_server_time,priority:1;size:36"`
	ProcessIdentityID          string  `gorm:"index;size:36"`
	PlayerIdentityID           *string `gorm:"index;size:36"`
	SourceSequence             uint64
	Type                       string `gorm:"size:16"`
	PlayerName, NormalizedName string `gorm:"size:64"`
	RawEvidence                string
	ObservedAt                 time.Time `gorm:"index:idx_player_event_server_time,priority:2"`
	CreatedAt                  time.Time
	SchemaVersion              int
}

// PlayerSessionRecord is one durable player connection.
type PlayerSessionRecord struct {
	ID                   string    `gorm:"primaryKey;size:36"`
	ServerID             string    `gorm:"index:idx_player_session_query,priority:1;uniqueIndex:idx_player_open_session_unique,priority:1,where:state = 'open';size:36"`
	PlayerIdentityID     string    `gorm:"index:idx_player_session_query,priority:2;uniqueIndex:idx_player_open_session_unique,priority:2,where:state = 'open';size:36"`
	ProcessIdentityID    string    `gorm:"index;size:36"`
	JoinedAt             time.Time `gorm:"index:idx_player_session_query,priority:3"`
	LeftAt               *time.Time
	DurationSeconds      int64
	State                string `gorm:"index;size:24"`
	CloseReason          string `gorm:"size:32"`
	Accuracy             string `gorm:"size:24"`
	CreatedAt, UpdatedAt time.Time
	SchemaVersion        int
}

// PlayerStatisticsRecord is the rebuildable player aggregate representation.
type PlayerStatisticsRecord struct {
	PlayerIdentityID                                                   string `gorm:"primaryKey;size:36"`
	ServerID                                                           string `gorm:"index;size:36"`
	TotalDurationSeconds, LongestSessionSeconds, CompletedSessionCount int64
	FirstActivityAt, LastActivityAt                                    *time.Time
	Accuracy                                                           string `gorm:"size:24"`
	UpdatedAt                                                          time.Time
	SchemaVersion                                                      int
}

// PlayerDirectorySnapshotRecord is the latest authoritative permission snapshot.
type PlayerDirectorySnapshotRecord struct {
	ID                                                string `gorm:"primaryKey;size:36"`
	ServerID                                          string `gorm:"index;size:36"`
	PlayerIdentityID                                  string `gorm:"uniqueIndex;size:36"`
	Known, Whitelisted, Operator, Banned              bool
	BanReason, BanSource, BanExpiresAt, SourceVersion string
	ObservedAt                                        time.Time
	SchemaVersion                                     int
}

// PlayerCollectorStatusRecord is the per-Server collector synchronization checkpoint.
type PlayerCollectorStatusRecord struct {
	ServerID                              string  `gorm:"primaryKey;size:36"`
	ProcessIdentityID                     *string `gorm:"size:36"`
	LastSourceSequence, DroppedEventCount uint64
	LastObservedAt, LastSynchronizedAt    *time.Time
	Accuracy, LastError                   string
	UpdatedAt                             time.Time
	SchemaVersion                         int
}

type playerRepository struct{ database *gorm.DB }

// CreateIdentity 新增一条玩家身份记录。
func (r *playerRepository) CreateIdentity(ctx context.Context, value *model.PlayerIdentity) error {
	if value == nil {
		return apperror.New(apperror.CodeValidationRequired, "玩家身份不能为空")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	return playerWrite(r.database.WithContext(ctx).Create(identityToRecord(*value)).Error, "创建玩家身份失败")
}

// UpdateIdentity 更新一条玩家身份记录。
func (r *playerRepository) UpdateIdentity(ctx context.Context, value *model.PlayerIdentity) error {
	if value == nil {
		return apperror.New(apperror.CodeValidationRequired, "玩家身份不能为空")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	return playerWrite(r.database.WithContext(ctx).Save(identityToRecord(*value)).Error, "更新玩家身份失败")
}

// FindIdentity 按查询条件查找单条玩家身份,未命中时返回 NotFound。
func (r *playerRepository) FindIdentity(ctx context.Context, q repository.PlayerIdentityQuery) (*model.PlayerIdentity, error) {
	var row PlayerIdentityRecord
	db := r.database.WithContext(ctx).Where("server_id = ?", q.ServerID.String())
	if q.UUID != "" {
		db = db.Where("uuid = ?", q.UUID)
	} else {
		db = db.Where("normalized_name = ?", q.NormalizedName)
	}
	if err := db.Order("CASE WHEN uuid IS NULL THEN 1 ELSE 0 END").First(&row).Error; err != nil {
		return nil, playerRead(err, "查询玩家身份失败")
	}
	value := recordToIdentity(row)
	return &value, nil
}

// ListIdentities 分页列出某台 Server 的玩家身份。
func (r *playerRepository) ListIdentities(ctx context.Context, serverID model.ID, limit, offset int) ([]model.PlayerIdentity, error) {
	var rows []PlayerIdentityRecord
	if err := playerPage(r.database.WithContext(ctx).Where("server_id = ?", serverID.String()).Order("normalized_name asc, id asc"), limit, offset).Find(&rows).Error; err != nil {
		return nil, playerRead(err, "查询玩家身份列表失败")
	}
	result := make([]model.PlayerIdentity, len(rows))
	for index := range rows {
		result[index] = recordToIdentity(rows[index])
	}
	return result, nil
}

// DeleteIdentity 删除一条玩家身份记录。
func (r *playerRepository) DeleteIdentity(ctx context.Context, id model.ID) error {
	return playerWrite(r.database.WithContext(ctx).Delete(&PlayerIdentityRecord{}, "id = ?", id.String()).Error, "删除玩家身份失败")
}

// ReassignIdentity 把一条身份下的活动与会话改挂到另一条身份。
func (r *playerRepository) ReassignIdentity(ctx context.Context, from, to model.ID) error {
	for _, record := range []any{&PlayerActivityEventRecord{}, &PlayerSessionRecord{}} {
		if err := r.database.WithContext(ctx).Model(record).Where("player_identity_id = ?", from.String()).Update("player_identity_id", to.String()).Error; err != nil {
			return playerWrite(err, "迁移玩家身份关联失败")
		}
	}
	if err := r.database.WithContext(ctx).Delete(&PlayerDirectorySnapshotRecord{}, "player_identity_id = ?", from.String()).Error; err != nil {
		return playerWrite(err, "清理被合并玩家目录投影失败")
	}
	if err := r.database.WithContext(ctx).Delete(&PlayerStatisticsRecord{}, "player_identity_id = ?", from.String()).Error; err != nil {
		return playerWrite(err, "清理被合并玩家统计投影失败")
	}
	return nil
}

// InsertEvent 写入一条玩家进出事件,返回该事件是否为新增。
func (r *playerRepository) InsertEvent(ctx context.Context, value *model.PlayerActivityEvent) (bool, error) {
	if value == nil {
		return false, apperror.New(apperror.CodeValidationRequired, "玩家事件不能为空")
	}
	if err := value.Validate(); err != nil {
		return false, err
	}
	result := r.database.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(eventToRecord(*value))
	if result.Error != nil {
		return false, playerWrite(result.Error, "写入玩家事件失败")
	}
	return result.RowsAffected == 1, nil
}

// CreateSession 新增一条玩家在线会话。
func (r *playerRepository) CreateSession(ctx context.Context, value *model.PlayerSession) error {
	if value == nil {
		return apperror.New(apperror.CodeValidationRequired, "玩家会话不能为空")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	return playerWrite(r.database.WithContext(ctx).Create(sessionToRecord(*value)).Error, "创建玩家会话失败")
}

// UpdateSession 更新一条玩家在线会话。
func (r *playerRepository) UpdateSession(ctx context.Context, value *model.PlayerSession) error {
	if value == nil {
		return apperror.New(apperror.CodeValidationRequired, "玩家会话不能为空")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	return playerWrite(r.database.WithContext(ctx).Save(sessionToRecord(*value)).Error, "更新玩家会话失败")
}

// GetOpenSession 返回某位玩家当前未结束的会话。
func (r *playerRepository) GetOpenSession(ctx context.Context, serverID, playerID model.ID) (*model.PlayerSession, error) {
	var row PlayerSessionRecord
	if err := r.database.WithContext(ctx).Where("server_id = ? AND player_identity_id = ? AND state = ?", serverID.String(), playerID.String(), enums.PlayerSessionOpen.String()).Order("joined_at desc, id desc").First(&row).Error; err != nil {
		return nil, playerRead(err, "查询开放玩家会话失败")
	}
	value := recordToSession(row)
	return &value, nil
}

// ListSessions 按查询条件分页列出玩家会话。
func (r *playerRepository) ListSessions(ctx context.Context, q repository.PlayerSessionQuery) ([]model.PlayerSession, error) {
	db := r.database.WithContext(ctx).Where("server_id = ?", q.ServerID.String())
	if q.PlayerIdentityID.Valid() {
		db = db.Where("player_identity_id = ?", q.PlayerIdentityID.String())
	}
	if q.OpenOnly {
		db = db.Where("state = ?", enums.PlayerSessionOpen.String())
	}
	var rows []PlayerSessionRecord
	if err := playerPage(db.Order("joined_at desc, id desc"), q.Limit, q.Offset).Find(&rows).Error; err != nil {
		return nil, playerRead(err, "查询玩家会话失败")
	}
	return sessionsFromRecords(rows), nil
}

// SaveStatistics 写入或覆盖一条玩家统计。
func (r *playerRepository) SaveStatistics(ctx context.Context, value *model.PlayerStatistics) error {
	if value == nil {
		return apperror.New(apperror.CodeValidationRequired, "玩家统计不能为空")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	return playerWrite(r.database.WithContext(ctx).Save(statisticsToRecord(*value)).Error, "保存玩家统计失败")
}

// GetStatistics 返回一条玩家统计。
func (r *playerRepository) GetStatistics(ctx context.Context, id model.ID) (*model.PlayerStatistics, error) {
	var row PlayerStatisticsRecord
	if err := r.database.WithContext(ctx).First(&row, "player_identity_id = ?", id.String()).Error; err != nil {
		return nil, playerRead(err, "查询玩家统计失败")
	}
	value := recordToStatistics(row)
	return &value, nil
}

// ListClosedSessionsForRebuild 分页列出已结束的会话,供统计重算使用。
func (r *playerRepository) ListClosedSessionsForRebuild(ctx context.Context, serverID model.ID, limit, offset int) ([]model.PlayerSession, error) {
	var rows []PlayerSessionRecord
	if err := playerPage(r.database.WithContext(ctx).Where("server_id = ? AND state <> ? AND left_at IS NOT NULL", serverID.String(), enums.PlayerSessionOpen.String()).Order("joined_at asc, id asc"), limit, offset).Find(&rows).Error; err != nil {
		return nil, playerRead(err, "查询统计重建会话失败")
	}
	return sessionsFromRecords(rows), nil
}

// SaveDirectorySnapshot 写入或覆盖一份玩家名录快照。
func (r *playerRepository) SaveDirectorySnapshot(ctx context.Context, value *model.PlayerDirectorySnapshot) error {
	if value == nil {
		return apperror.New(apperror.CodeValidationRequired, "玩家目录快照不能为空")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	record := directoryToRecord(*value)
	return playerWrite(r.database.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "player_identity_id"}}, DoUpdates: clause.AssignmentColumns([]string{"id", "server_id", "known", "whitelisted", "operator", "banned", "ban_reason", "ban_source", "ban_expires_at", "source_version", "observed_at", "schema_version"})}).Create(&record).Error, "保存玩家目录快照失败")
}

// GetDirectorySnapshot 返回某台 Server 的玩家名录快照。
func (r *playerRepository) GetDirectorySnapshot(ctx context.Context, id model.ID) (*model.PlayerDirectorySnapshot, error) {
	var row PlayerDirectorySnapshotRecord
	if err := r.database.WithContext(ctx).First(&row, "player_identity_id = ?", id.String()).Error; err != nil {
		return nil, playerRead(err, "查询玩家目录快照失败")
	}
	value := recordToDirectory(row)
	return &value, nil
}

// SaveCollectorStatus 写入或覆盖玩家活动采集器状态。
func (r *playerRepository) SaveCollectorStatus(ctx context.Context, value *model.PlayerCollectorStatus) error {
	if value == nil {
		return apperror.New(apperror.CodeValidationRequired, "玩家采集状态不能为空")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	return playerWrite(r.database.WithContext(ctx).Save(collectorToRecord(*value)).Error, "保存玩家采集状态失败")
}

// GetCollectorStatus 返回玩家活动采集器状态。
func (r *playerRepository) GetCollectorStatus(ctx context.Context, id model.ID) (*model.PlayerCollectorStatus, error) {
	var row PlayerCollectorStatusRecord
	if err := r.database.WithContext(ctx).First(&row, "server_id = ?", id.String()).Error; err != nil {
		return nil, playerRead(err, "查询玩家采集状态失败")
	}
	value := recordToCollector(row)
	return &value, nil
}

// DeleteEventsBefore 删除某台 Server 指定时间之前的一批玩家事件。
func (r *playerRepository) DeleteEventsBefore(ctx context.Context, serverID model.ID, before time.Time, limit int) (int64, error) {
	var ids []string
	if err := playerPage(r.database.WithContext(ctx).Model(&PlayerActivityEventRecord{}).Where("server_id = ? AND observed_at < ?", serverID.String(), before.UTC()).Order("observed_at asc"), limit, 0).Pluck("id", &ids).Error; err != nil {
		return 0, playerRead(err, "查询待清理玩家事件失败")
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.database.WithContext(ctx).Delete(&PlayerActivityEventRecord{}, "id IN ?", ids)
	return result.RowsAffected, playerWrite(result.Error, "清理玩家事件失败")
}

func playerPage(db *gorm.DB, limit, offset int) *gorm.DB {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return db.Limit(limit).Offset(offset)
}
func playerRead(err error, message string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperror.New(apperror.CodeIONotFound, message)
	}
	return apperror.Wrap(apperror.CodeIOReadFailed, message, err)
}
func playerWrite(err error, message string) error {
	if err == nil {
		return nil
	}
	return apperror.Wrap(apperror.CodeIOWriteFailed, message, err)
}
func identityToRecord(v model.PlayerIdentity) PlayerIdentityRecord {
	var uuid *string
	if v.UUID != "" {
		x := v.UUID
		uuid = &x
	}
	return PlayerIdentityRecord{v.ID.String(), v.ServerID.String(), uuid, v.CurrentName, v.NormalizedName, v.Kind.String(), v.CreatedAt, v.UpdatedAt, v.SchemaVersion}
}
func recordToIdentity(v PlayerIdentityRecord) model.PlayerIdentity {
	uuid := ""
	if v.UUID != nil {
		uuid = *v.UUID
	}
	return model.PlayerIdentity{ID: model.ID(v.ID), ServerID: model.ID(v.ServerID), UUID: uuid, CurrentName: v.CurrentName, NormalizedName: v.NormalizedName, Kind: enums.PlayerIdentityKind(v.Kind), CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, SchemaVersion: v.SchemaVersion}
}
func eventToRecord(v model.PlayerActivityEvent) PlayerActivityEventRecord {
	var player *string
	if v.PlayerIdentityID != nil {
		x := v.PlayerIdentityID.String()
		player = &x
	}
	return PlayerActivityEventRecord{v.ID.String(), v.ServerID.String(), v.ProcessIdentityID.String(), player, v.SourceSequence, v.Type.String(), v.PlayerName, v.NormalizedName, v.RawEvidence, v.ObservedAt, v.CreatedAt, v.SchemaVersion}
}
func sessionToRecord(v model.PlayerSession) PlayerSessionRecord {
	return PlayerSessionRecord{v.ID.String(), v.ServerID.String(), v.PlayerIdentityID.String(), v.ProcessIdentityID.String(), v.JoinedAt, v.LeftAt, v.DurationSeconds, v.State.String(), v.CloseReason.String(), v.Accuracy.String(), v.CreatedAt, v.UpdatedAt, v.SchemaVersion}
}
func recordToSession(v PlayerSessionRecord) model.PlayerSession {
	return model.PlayerSession{ID: model.ID(v.ID), ServerID: model.ID(v.ServerID), PlayerIdentityID: model.ID(v.PlayerIdentityID), ProcessIdentityID: model.ID(v.ProcessIdentityID), JoinedAt: v.JoinedAt, LeftAt: v.LeftAt, DurationSeconds: v.DurationSeconds, State: enums.PlayerSessionState(v.State), CloseReason: enums.PlayerSessionCloseReason(v.CloseReason), Accuracy: enums.PlayerActivityAccuracy(v.Accuracy), CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, SchemaVersion: v.SchemaVersion}
}
func sessionsFromRecords(rows []PlayerSessionRecord) []model.PlayerSession {
	result := make([]model.PlayerSession, len(rows))
	for i := range rows {
		result[i] = recordToSession(rows[i])
	}
	return result
}
func statisticsToRecord(v model.PlayerStatistics) PlayerStatisticsRecord {
	return PlayerStatisticsRecord{v.PlayerIdentityID.String(), v.ServerID.String(), v.TotalDurationSeconds, v.LongestSessionSeconds, v.CompletedSessionCount, v.FirstActivityAt, v.LastActivityAt, v.Accuracy.String(), v.UpdatedAt, v.SchemaVersion}
}
func recordToStatistics(v PlayerStatisticsRecord) model.PlayerStatistics {
	return model.PlayerStatistics{PlayerIdentityID: model.ID(v.PlayerIdentityID), ServerID: model.ID(v.ServerID), TotalDurationSeconds: v.TotalDurationSeconds, LongestSessionSeconds: v.LongestSessionSeconds, CompletedSessionCount: v.CompletedSessionCount, FirstActivityAt: v.FirstActivityAt, LastActivityAt: v.LastActivityAt, Accuracy: enums.PlayerActivityAccuracy(v.Accuracy), UpdatedAt: v.UpdatedAt, SchemaVersion: v.SchemaVersion}
}
func directoryToRecord(v model.PlayerDirectorySnapshot) PlayerDirectorySnapshotRecord {
	return PlayerDirectorySnapshotRecord{v.ID.String(), v.ServerID.String(), v.PlayerIdentityID.String(), v.Known, v.Whitelisted, v.Operator, v.Banned, v.BanReason, v.BanSource, v.BanExpiresAt, v.SourceVersion, v.ObservedAt, v.SchemaVersion}
}
func recordToDirectory(v PlayerDirectorySnapshotRecord) model.PlayerDirectorySnapshot {
	return model.PlayerDirectorySnapshot{ID: model.ID(v.ID), ServerID: model.ID(v.ServerID), PlayerIdentityID: model.ID(v.PlayerIdentityID), Known: v.Known, Whitelisted: v.Whitelisted, Operator: v.Operator, Banned: v.Banned, BanReason: v.BanReason, BanSource: v.BanSource, BanExpiresAt: v.BanExpiresAt, SourceVersion: v.SourceVersion, ObservedAt: v.ObservedAt, SchemaVersion: v.SchemaVersion}
}
func collectorToRecord(v model.PlayerCollectorStatus) PlayerCollectorStatusRecord {
	var process *string
	if v.ProcessIdentityID != nil {
		x := v.ProcessIdentityID.String()
		process = &x
	}
	return PlayerCollectorStatusRecord{v.ServerID.String(), process, v.LastSourceSequence, v.DroppedEventCount, v.LastObservedAt, v.LastSynchronizedAt, v.Accuracy.String(), v.LastError, v.UpdatedAt, v.SchemaVersion}
}
func recordToCollector(v PlayerCollectorStatusRecord) model.PlayerCollectorStatus {
	var process *model.ID
	if v.ProcessIdentityID != nil {
		x := model.ID(*v.ProcessIdentityID)
		process = &x
	}
	return model.PlayerCollectorStatus{ServerID: model.ID(v.ServerID), ProcessIdentityID: process, LastSourceSequence: v.LastSourceSequence, DroppedEventCount: v.DroppedEventCount, LastObservedAt: v.LastObservedAt, LastSynchronizedAt: v.LastSynchronizedAt, Accuracy: enums.PlayerActivityAccuracy(v.Accuracy), LastError: v.LastError, UpdatedAt: v.UpdatedAt, SchemaVersion: v.SchemaVersion}
}
