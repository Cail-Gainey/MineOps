package gormrepo

import (
	"context"
	"errors"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"gorm.io/gorm"
)

// InstallationTaskRecord 是安装任务在加密 SQLite 中的表示。
type InstallationTaskRecord struct {
	ID          string  `gorm:"primaryKey;size:36"`
	ServerID    string  `gorm:"index;size:36"`
	OperationID *string `gorm:"index;size:36"`
	State       string  `gorm:"index;size:24"`
	CurrentStep int
	LogCursor   int64
	CreatedAt   time.Time `gorm:"index"`
	StartedAt   *time.Time
	FinishedAt  *time.Time
	UpdatedAt   time.Time `gorm:"index"`
}

// InstallationStepRecord 是安装检查点在加密 SQLite 中的表示。
type InstallationStepRecord struct {
	ID           string `gorm:"primaryKey;size:36"`
	TaskID       string `gorm:"uniqueIndex:idx_installation_step_order,priority:1;index;size:36"`
	Order        int    `gorm:"uniqueIndex:idx_installation_step_order,priority:2"`
	Name         string `gorm:"size:80"`
	State        string `gorm:"index;size:24"`
	Attempt      int
	Progress     float64
	Checkpoint   map[string]any `gorm:"serializer:json"`
	ErrorCode    string         `gorm:"size:80"`
	ErrorDetails map[string]any `gorm:"serializer:json"`
	Message      string
	StartedAt    *time.Time
	FinishedAt   *time.Time
	UpdatedAt    time.Time `gorm:"index"`
}

type installationRepository struct{ store *Store }

// Create 在同一事务内写入安装任务及其全部步骤。
func (r *installationRepository) Create(ctx context.Context, task *model.InstallationTask, steps []model.InstallationStep) error {
	if task == nil || len(steps) == 0 {
		return apperror.New(apperror.CodeValidationRequired, "Installation Task 和 Steps 不能为空")
	}
	return r.store.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		taskRecord := installationTaskToRecord(task)
		if err := transaction.Create(&taskRecord).Error; err != nil {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Installation Task 失败", err)
		}
		stepRecords := make([]InstallationStepRecord, len(steps))
		for index := range steps {
			stepRecords[index] = installationStepToRecord(&steps[index])
		}
		if err := transaction.Create(&stepRecords).Error; err != nil {
			return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Installation Steps 失败", err)
		}
		return nil
	})
}

// UpdateTask 更新一条安装任务。
func (r *installationRepository) UpdateTask(ctx context.Context, task *model.InstallationTask) error {
	record := installationTaskToRecord(task)
	result := r.store.database.WithContext(ctx).Model(&InstallationTaskRecord{}).Where("id = ?", record.ID).Select("*").Updates(&record)
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "更新 Installation Task 失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "Installation Task 不存在")
	}
	return nil
}

// UpdateStep 更新一条安装步骤。
func (r *installationRepository) UpdateStep(ctx context.Context, step *model.InstallationStep) error {
	record := installationStepToRecord(step)
	result := r.store.database.WithContext(ctx).Model(&InstallationStepRecord{}).Where("id = ?", record.ID).Select("*").Updates(&record)
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "更新 Installation Step 失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "Installation Step 不存在")
	}
	return nil
}

// Get 按 ID 返回安装任务及其有序步骤。
func (r *installationRepository) Get(ctx context.Context, id model.ID) (*model.InstallationTask, []model.InstallationStep, error) {
	var taskRecord InstallationTaskRecord
	if err := r.store.database.WithContext(ctx).First(&taskRecord, "id = ?", id.String()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, apperror.New(apperror.CodeIONotFound, "Installation Task 不存在")
		}
		return nil, nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Installation Task 失败", err)
	}
	var stepRecords []InstallationStepRecord
	if err := r.store.database.WithContext(ctx).Where("task_id = ?", id.String()).Order("`order` asc").Find(&stepRecords).Error; err != nil {
		return nil, nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Installation Steps 失败", err)
	}
	task := recordToInstallationTask(taskRecord)
	steps := make([]model.InstallationStep, len(stepRecords))
	for index, record := range stepRecords {
		steps[index] = recordToInstallationStep(record)
	}
	return &task, steps, nil
}

// ListByServer 分页列出某台 Server 的安装历史。
func (r *installationRepository) ListByServer(ctx context.Context, serverID model.ID, limit, offset int) ([]model.InstallationTask, error) {
	query := r.store.database.WithContext(ctx).Where("server_id = ?", serverID.String()).Order("created_at desc")
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return queryInstallationTasks(query.Limit(limit).Offset(offset))
}

// ListRecoverable 列出应用重启后需要恢复的未完成安装任务。
func (r *installationRepository) ListRecoverable(ctx context.Context) ([]model.InstallationTask, error) {
	return queryInstallationTasks(r.store.database.WithContext(ctx).Where("state IN ?", []string{
		enums.InstallationWaiting.String(), enums.InstallationRunning.String(), enums.InstallationFailed.String(),
	}).Order("created_at asc"))
}

func queryInstallationTasks(database *gorm.DB) ([]model.InstallationTask, error) {
	var records []InstallationTaskRecord
	if err := database.Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Installation Tasks 失败", err)
	}
	result := make([]model.InstallationTask, len(records))
	for index, record := range records {
		result[index] = recordToInstallationTask(record)
	}
	return result, nil
}

func installationTaskToRecord(task *model.InstallationTask) InstallationTaskRecord {
	if task == nil {
		return InstallationTaskRecord{}
	}
	var operationID *string
	if task.OperationID != nil {
		value := task.OperationID.String()
		operationID = &value
	}
	return InstallationTaskRecord{
		ID: task.ID.String(), ServerID: task.ServerID.String(), OperationID: operationID,
		State: task.State.String(), CurrentStep: task.CurrentStep, LogCursor: task.LogCursor,
		CreatedAt: task.CreatedAt, StartedAt: task.StartedAt, FinishedAt: task.FinishedAt, UpdatedAt: task.UpdatedAt,
	}
}

func installationStepToRecord(step *model.InstallationStep) InstallationStepRecord {
	if step == nil {
		return InstallationStepRecord{}
	}
	return InstallationStepRecord{
		ID: step.ID.String(), TaskID: step.TaskID.String(), Order: step.Order, Name: step.Name,
		State: step.State.String(), Attempt: step.Attempt, Progress: step.Progress,
		Checkpoint: step.Checkpoint, ErrorCode: step.ErrorCode, ErrorDetails: step.ErrorDetails, Message: step.Message,
		StartedAt: step.StartedAt, FinishedAt: step.FinishedAt, UpdatedAt: step.UpdatedAt,
	}
}

func recordToInstallationTask(record InstallationTaskRecord) model.InstallationTask {
	var operationID *model.ID
	if record.OperationID != nil {
		value := model.ID(*record.OperationID)
		operationID = &value
	}
	return model.InstallationTask{
		ID: model.ID(record.ID), ServerID: model.ID(record.ServerID), OperationID: operationID,
		State: enums.InstallationState(record.State), CurrentStep: record.CurrentStep, LogCursor: record.LogCursor,
		CreatedAt: record.CreatedAt, StartedAt: record.StartedAt, FinishedAt: record.FinishedAt, UpdatedAt: record.UpdatedAt,
	}
}

func recordToInstallationStep(record InstallationStepRecord) model.InstallationStep {
	return model.InstallationStep{
		ID: model.ID(record.ID), TaskID: model.ID(record.TaskID), Order: record.Order, Name: record.Name,
		State: enums.InstallationStepState(record.State), Attempt: record.Attempt, Progress: record.Progress,
		Checkpoint: record.Checkpoint, ErrorCode: record.ErrorCode, ErrorDetails: record.ErrorDetails, Message: record.Message,
		StartedAt: record.StartedAt, FinishedAt: record.FinishedAt, UpdatedAt: record.UpdatedAt,
	}
}
