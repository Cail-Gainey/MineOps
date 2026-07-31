package gormrepo

import (
	"context"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SparkCapabilityRecord is the one-row-per-Server durable capability representation.
type SparkCapabilityRecord struct {
	ServerID          string `gorm:"primaryKey;size:36"`
	Status            string `gorm:"index;size:32"`
	ServerType        string `gorm:"size:32"`
	Platform          string `gorm:"size:80"`
	Distribution      string `gorm:"size:80"`
	Installed         bool
	PluginVersion     string `gorm:"size:80"`
	ParserVersion     string `gorm:"size:80"`
	CollectionMethod  string `gorm:"size:80"`
	SourceSchemaHash  string `gorm:"size:128"`
	TPSSupported      bool
	MSPTSupported     bool
	ReportSupported   bool
	PermissionGranted bool
	ArtifactPath      string
	BackupPath        string
	InstallManifest   string
	RestartRequired   bool
	DetectedAt        time.Time `gorm:"index"`
	LastErrorCode     string    `gorm:"size:96"`
	LastError         string
	SchemaVersion     int
}

// SparkSnapshotRecord is the indexed durable TPS/MSPT snapshot representation.
type SparkSnapshotRecord struct {
	ID                 string `gorm:"primaryKey;size:36"`
	ServerID           string `gorm:"index:idx_spark_snapshot_server_time,priority:1;size:36"`
	SourceID           string `gorm:"index;size:36"`
	ServerType         string `gorm:"size:32"`
	Platform           string `gorm:"size:80"`
	PluginVersion      string `gorm:"size:80"`
	ParserVersion      string `gorm:"size:80"`
	CollectionMethod   string `gorm:"size:80"`
	SourceSchemaHash   string `gorm:"size:128"`
	TPS5Seconds        float64
	TPS5SecondsCapped  bool
	TPS10Seconds       float64
	TPS10SecondsCapped bool
	TPS1Minute         float64
	TPS1MinuteCapped   bool
	TPS5Minutes        float64
	TPS5MinutesCapped  bool
	TPS15Minutes       float64
	TPS15MinutesCapped bool
	MSPTAvailable      bool
	MSPTMinimum        float64
	MSPTMedian         float64
	MSPTP95            float64
	MSPTMaximum        float64
	CollectedAt        time.Time `gorm:"index:idx_spark_snapshot_server_time,priority:2"`
	SchemaVersion      int
}

// SparkReportRecord is the indexed durable report workflow representation.
type SparkReportRecord struct {
	ID                  string  `gorm:"primaryKey;size:36"`
	ServerID            string  `gorm:"index:idx_spark_report_server_created,priority:1;size:36"`
	OperationID         *string `gorm:"index;size:36"`
	Kind                string  `gorm:"index;size:32"`
	State               string  `gorm:"index;size:32"`
	DurationSeconds     int
	ReportURL           string
	PrivacyAcknowledged bool
	PluginVersion       string `gorm:"size:80"`
	ParserVersion       string `gorm:"size:80"`
	StartedAt           *time.Time
	FinishedAt          *time.Time
	ErrorCode           string `gorm:"size:96"`
	ErrorMessage        string
	CreatedAt           time.Time `gorm:"index:idx_spark_report_server_created,priority:2"`
	UpdatedAt           time.Time
	SchemaVersion       int
}

// sparkRepository 跨两个库:Capability 与 Report 可能带远端路径和玩家名,留在加密库;
// Snapshot 只有 TPS/MSPT 数值,和 Metric 一起放未加密监控库,两者才能在同一个事务里原子写入。
type sparkRepository struct {
	database *gorm.DB
	metrics  *gorm.DB
}

func (r *sparkRepository) SaveCapability(ctx context.Context, capability *model.SparkCapability) error {
	if capability == nil {
		return apperror.New(apperror.CodeValidationRequired, "Spark Capability 不能为空")
	}
	if err := capability.Validate(); err != nil {
		return err
	}
	record := capabilityToRecord(*capability)
	if err := r.database.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&record).Error; err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "保存 Spark Capability 失败", err)
	}
	return nil
}

func (r *sparkRepository) GetCapability(ctx context.Context, serverID model.ID) (*model.SparkCapability, error) {
	if !serverID.Valid() {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "Spark Server ID 无效")
	}
	var record SparkCapabilityRecord
	if err := r.database.WithContext(ctx).First(&record, "server_id = ?", serverID.String()).Error; err != nil {
		return nil, mapSparkNotFound("Spark Capability 不存在", err)
	}
	capability := recordToCapability(record)
	return &capability, nil
}

func (r *sparkRepository) CreateSnapshot(ctx context.Context, snapshot *model.SparkSnapshot) error {
	if snapshot == nil {
		return apperror.New(apperror.CodeValidationRequired, "Spark Snapshot 不能为空")
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	record := snapshotToRecord(*snapshot)
	if err := r.metrics.WithContext(ctx).Create(&record).Error; err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Spark Snapshot 失败", err)
	}
	return nil
}

func (r *sparkRepository) LatestSnapshot(ctx context.Context, serverID model.ID) (*model.SparkSnapshot, error) {
	var record SparkSnapshotRecord
	if err := r.metrics.WithContext(ctx).Where("server_id = ?", serverID.String()).Order("collected_at desc").First(&record).Error; err != nil {
		return nil, mapSparkNotFound("Spark Snapshot 不存在", err)
	}
	snapshot := recordToSnapshot(record)
	return &snapshot, nil
}

func (r *sparkRepository) ListSnapshots(ctx context.Context, query repository.SparkSnapshotQuery) ([]model.SparkSnapshot, error) {
	database := r.metrics.WithContext(ctx).Order("collected_at desc")
	if query.ServerID.Valid() {
		database = database.Where("server_id = ?", query.ServerID.String())
	}
	var records []SparkSnapshotRecord
	if err := applySparkPagination(database, query.Limit, query.Offset).Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Spark Snapshot 历史失败", err)
	}
	result := make([]model.SparkSnapshot, len(records))
	for index, record := range records {
		result[index] = recordToSnapshot(record)
	}
	return result, nil
}

func (r *sparkRepository) DeleteSnapshotsBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 || limit > 50_000 {
		limit = 10_000
	}
	var ids []string
	database := r.metrics.WithContext(ctx).Model(&SparkSnapshotRecord{}).
		Where("collected_at < ?", before.UTC()).
		Order("collected_at asc").Limit(limit)
	if err := database.Pluck("id", &ids).Error; err != nil {
		return 0, apperror.Wrap(apperror.CodeIOReadFailed, "查询待清理 Spark Snapshot 失败", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.metrics.WithContext(ctx).Delete(&SparkSnapshotRecord{}, "id IN ?", ids)
	if result.Error != nil {
		return 0, apperror.Wrap(apperror.CodeIOWriteFailed, "清理 Spark Snapshot 数据失败", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *sparkRepository) DeleteServerSnapshots(ctx context.Context, serverID model.ID, batchSize int) (int64, error) {
	if !serverID.Valid() {
		return 0, apperror.New(apperror.CodeValidationInvalidArgument, "Spark Snapshot Server ID 无效")
	}
	if batchSize <= 0 || batchSize > 50_000 {
		batchSize = 10_000
	}
	var total int64
	for {
		var ids []string
		if err := r.metrics.WithContext(ctx).Model(&SparkSnapshotRecord{}).
			Where("server_id = ?", serverID.String()).Limit(batchSize).Pluck("id", &ids).Error; err != nil {
			return total, apperror.Wrap(apperror.CodeIOReadFailed, "查询待删除 Server Spark Snapshot 失败", err)
		}
		if len(ids) == 0 {
			break
		}
		result := r.metrics.WithContext(ctx).Delete(&SparkSnapshotRecord{}, "id IN ?", ids)
		if result.Error != nil {
			return total, apperror.Wrap(apperror.CodeIOWriteFailed, "删除 Server Spark Snapshot 历史失败", result.Error)
		}
		total += result.RowsAffected
		if len(ids) < batchSize {
			break
		}
	}
	return total, nil
}

func (r *sparkRepository) CreateReport(ctx context.Context, report *model.SparkReport) error {
	if report == nil {
		return apperror.New(apperror.CodeValidationRequired, "Spark Report 不能为空")
	}
	if err := report.Validate(); err != nil {
		return err
	}
	record := reportToRecord(*report)
	if err := r.database.WithContext(ctx).Create(&record).Error; err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Spark Report 失败", err)
	}
	return nil
}

func (r *sparkRepository) UpdateReport(ctx context.Context, report *model.SparkReport) error {
	if report == nil {
		return apperror.New(apperror.CodeValidationRequired, "Spark Report 不能为空")
	}
	if err := report.Validate(); err != nil {
		return err
	}
	record := reportToRecord(*report)
	database := r.database.WithContext(ctx).Model(&SparkReportRecord{}).Where("id = ?", report.ID.String()).Select("*")
	if report.OperationID == nil {
		database = database.Omit("OperationID")
	}
	result := database.Updates(&record)
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "更新 Spark Report 失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperror.New(apperror.CodeIONotFound, "Spark Report 不存在")
	}
	return nil
}

func (r *sparkRepository) GetReport(ctx context.Context, id model.ID) (*model.SparkReport, error) {
	var record SparkReportRecord
	if err := r.database.WithContext(ctx).First(&record, "id = ?", id.String()).Error; err != nil {
		return nil, mapSparkNotFound("Spark Report 不存在", err)
	}
	report := recordToReport(record)
	return &report, nil
}

func (r *sparkRepository) ListReports(ctx context.Context, query repository.SparkReportQuery) ([]model.SparkReport, error) {
	database := r.database.WithContext(ctx).Order("created_at desc")
	if query.ServerID.Valid() {
		database = database.Where("server_id = ?", query.ServerID.String())
	}
	if query.Kind.Valid() {
		database = database.Where("kind = ?", query.Kind.String())
	}
	var records []SparkReportRecord
	if err := applySparkPagination(database, query.Limit, query.Offset).Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Spark Report 历史失败", err)
	}
	result := make([]model.SparkReport, len(records))
	for index, record := range records {
		result[index] = recordToReport(record)
	}
	return result, nil
}

// DeleteReport 删除一条已结束的 Spark 报告记录。
func (r *sparkRepository) DeleteReport(ctx context.Context, id model.ID) error {
	result := r.database.WithContext(ctx).Delete(&SparkReportRecord{}, "id = ?", id.String())
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "删除 Spark Report 失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperror.New(apperror.CodeIONotFound, "Spark Report 不存在")
	}
	return nil
}

func applySparkPagination(database *gorm.DB, limit, offset int) *gorm.DB {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return database.Limit(limit).Offset(offset)
}

func mapSparkNotFound(message string, err error) error {
	if err == gorm.ErrRecordNotFound {
		return apperror.New(apperror.CodeIONotFound, message)
	}
	return apperror.Wrap(apperror.CodeIOReadFailed, message, err)
}

func capabilityToRecord(value model.SparkCapability) SparkCapabilityRecord {
	return SparkCapabilityRecord{
		ServerID: value.ServerID.String(), Status: value.Status.String(), ServerType: value.ServerType.String(), Platform: value.Platform,
		Distribution: value.Distribution, Installed: value.Installed, PluginVersion: value.PluginVersion, ParserVersion: value.ParserVersion,
		CollectionMethod: value.CollectionMethod, SourceSchemaHash: value.SourceSchemaHash, TPSSupported: value.TPSSupported,
		MSPTSupported: value.MSPTSupported, ReportSupported: value.ReportSupported, PermissionGranted: value.PermissionGranted,
		ArtifactPath: value.ArtifactPath, BackupPath: value.BackupPath, InstallManifest: value.InstallManifest, RestartRequired: value.RestartRequired, DetectedAt: value.DetectedAt,
		LastErrorCode: value.LastErrorCode, LastError: value.LastError, SchemaVersion: value.SchemaVersion,
	}
}

func recordToCapability(value SparkCapabilityRecord) model.SparkCapability {
	return model.SparkCapability{
		ServerID: model.ID(value.ServerID), Status: enums.SparkStatus(value.Status), ServerType: enums.MinecraftServerType(value.ServerType), Platform: value.Platform,
		Distribution: value.Distribution, Installed: value.Installed, PluginVersion: value.PluginVersion, ParserVersion: value.ParserVersion,
		CollectionMethod: value.CollectionMethod, SourceSchemaHash: value.SourceSchemaHash, TPSSupported: value.TPSSupported,
		MSPTSupported: value.MSPTSupported, ReportSupported: value.ReportSupported, PermissionGranted: value.PermissionGranted,
		ArtifactPath: value.ArtifactPath, BackupPath: value.BackupPath, InstallManifest: value.InstallManifest, RestartRequired: value.RestartRequired, DetectedAt: value.DetectedAt,
		LastErrorCode: value.LastErrorCode, LastError: value.LastError, SchemaVersion: value.SchemaVersion,
	}
}

func snapshotToRecord(value model.SparkSnapshot) SparkSnapshotRecord {
	return SparkSnapshotRecord{
		ID: value.ID.String(), ServerID: value.ServerID.String(), SourceID: value.SourceID.String(), ServerType: value.ServerType.String(), Platform: value.Platform,
		PluginVersion: value.PluginVersion, ParserVersion: value.ParserVersion, CollectionMethod: value.CollectionMethod, SourceSchemaHash: value.SourceSchemaHash,
		TPS5Seconds: value.TPS5Seconds, TPS5SecondsCapped: value.TPS5SecondsCapped, TPS10Seconds: value.TPS10Seconds, TPS10SecondsCapped: value.TPS10SecondsCapped,
		TPS1Minute: value.TPS1Minute, TPS1MinuteCapped: value.TPS1MinuteCapped, TPS5Minutes: value.TPS5Minutes, TPS5MinutesCapped: value.TPS5MinutesCapped,
		TPS15Minutes: value.TPS15Minutes, TPS15MinutesCapped: value.TPS15MinutesCapped,
		MSPTAvailable: value.MSPTAvailable, MSPTMinimum: value.MSPTMinimum, MSPTMedian: value.MSPTMedian, MSPTP95: value.MSPTP95, MSPTMaximum: value.MSPTMaximum,
		CollectedAt: value.CollectedAt, SchemaVersion: value.SchemaVersion,
	}
}

func recordToSnapshot(value SparkSnapshotRecord) model.SparkSnapshot {
	return model.SparkSnapshot{
		ID: model.ID(value.ID), ServerID: model.ID(value.ServerID), SourceID: model.ID(value.SourceID), ServerType: enums.MinecraftServerType(value.ServerType), Platform: value.Platform,
		PluginVersion: value.PluginVersion, ParserVersion: value.ParserVersion, CollectionMethod: value.CollectionMethod, SourceSchemaHash: value.SourceSchemaHash,
		TPS5Seconds: value.TPS5Seconds, TPS5SecondsCapped: value.TPS5SecondsCapped, TPS10Seconds: value.TPS10Seconds, TPS10SecondsCapped: value.TPS10SecondsCapped,
		TPS1Minute: value.TPS1Minute, TPS1MinuteCapped: value.TPS1MinuteCapped, TPS5Minutes: value.TPS5Minutes, TPS5MinutesCapped: value.TPS5MinutesCapped,
		TPS15Minutes: value.TPS15Minutes, TPS15MinutesCapped: value.TPS15MinutesCapped,
		MSPTAvailable: value.MSPTAvailable, MSPTMinimum: value.MSPTMinimum, MSPTMedian: value.MSPTMedian, MSPTP95: value.MSPTP95, MSPTMaximum: value.MSPTMaximum,
		CollectedAt: value.CollectedAt, SchemaVersion: value.SchemaVersion,
	}
}

func reportToRecord(value model.SparkReport) SparkReportRecord {
	var operationID *string
	if value.OperationID != nil {
		text := value.OperationID.String()
		operationID = &text
	}
	return SparkReportRecord{
		ID: value.ID.String(), ServerID: value.ServerID.String(), OperationID: operationID, Kind: value.Kind.String(), State: value.State.String(),
		DurationSeconds: value.DurationSeconds, ReportURL: value.ReportURL, PrivacyAcknowledged: value.PrivacyAcknowledged,
		PluginVersion: value.PluginVersion, ParserVersion: value.ParserVersion, StartedAt: value.StartedAt, FinishedAt: value.FinishedAt,
		ErrorCode: value.ErrorCode, ErrorMessage: value.ErrorMessage, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, SchemaVersion: value.SchemaVersion,
	}
}

func recordToReport(value SparkReportRecord) model.SparkReport {
	var operationID *model.ID
	if value.OperationID != nil {
		converted := model.ID(*value.OperationID)
		operationID = &converted
	}
	return model.SparkReport{
		ID: model.ID(value.ID), ServerID: model.ID(value.ServerID), OperationID: operationID, Kind: enums.SparkReportKind(value.Kind), State: enums.SparkReportState(value.State),
		DurationSeconds: value.DurationSeconds, ReportURL: value.ReportURL, PrivacyAcknowledged: value.PrivacyAcknowledged,
		PluginVersion: value.PluginVersion, ParserVersion: value.ParserVersion, StartedAt: value.StartedAt, FinishedAt: value.FinishedAt,
		ErrorCode: value.ErrorCode, ErrorMessage: value.ErrorMessage, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, SchemaVersion: value.SchemaVersion,
	}
}

var _ repository.SparkRepository = (*sparkRepository)(nil)
