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
)

// OperationRecord is the encrypted SQLite representation of a durable Operation.
type OperationRecord struct {
	ID           string `gorm:"primaryKey;size:36"`
	Type         string `gorm:"index;size:32"`
	TargetType   string `gorm:"index:idx_operation_target,priority:1;size:32"`
	TargetID     string `gorm:"index:idx_operation_target,priority:2;size:36"`
	State        string `gorm:"index;size:24"`
	Stage        string
	Progress     float64
	Message      string
	ErrorCode    string
	ErrorDetails map[string]any `gorm:"serializer:json"`
	CreatedAt    time.Time      `gorm:"index"`
	StartedAt    *time.Time
	FinishedAt   *time.Time
	RetryOf      *string `gorm:"index;size:36"`
}

type operationRepository struct {
	database *gorm.DB
}

func (r *operationRepository) Create(ctx context.Context, operation *model.Operation) error {
	record := operationToRecord(operation)
	if err := r.database.WithContext(ctx).Create(&record).Error; err != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "创建 Operation 失败", err)
	}
	return nil
}

func (r *operationRepository) Update(ctx context.Context, operation *model.Operation) error {
	record := operationToRecord(operation)
	result := r.database.WithContext(ctx).Model(&OperationRecord{}).Where("id = ?", record.ID).Select("*").Updates(&record)
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "更新 Operation 失败", result.Error)
	}
	if result.RowsAffected != 1 {
		return apperror.New(apperror.CodeIONotFound, "Operation 不存在")
	}
	return nil
}

func (r *operationRepository) Get(ctx context.Context, id model.ID) (*model.Operation, error) {
	var record OperationRecord
	if err := r.database.WithContext(ctx).First(&record, "id = ?", id.String()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.New(apperror.CodeIONotFound, "Operation 不存在")
		}
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Operation 失败", err)
	}
	operation := recordToOperation(record)
	return &operation, nil
}

func (r *operationRepository) ListActive(ctx context.Context, targetID model.ID) ([]model.Operation, error) {
	states := []string{enums.OperationPending.String(), enums.OperationRunning.String()}
	query := r.database.WithContext(ctx).Where("state IN ?", states).Order("created_at asc")
	if targetID != "" {
		query = query.Where("target_id = ?", targetID.String())
	}
	return queryOperations(query)
}

func (r *operationRepository) ListHistory(ctx context.Context, query repository.OperationQuery) ([]model.Operation, error) {
	database := r.database.WithContext(ctx).Order("created_at desc")
	if query.TargetID != "" {
		database = database.Where("target_id = ?", query.TargetID.String())
	}
	limit := query.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	return queryOperations(database.Limit(limit).Offset(query.Offset))
}

// DeleteHistory deletes one terminal Operation and rejects active records.
func (r *operationRepository) DeleteHistory(ctx context.Context, id model.ID) error {
	terminalStates := []string{
		enums.OperationSucceeded.String(), enums.OperationFailed.String(), enums.OperationCancelled.String(),
	}
	result := r.database.WithContext(ctx).Where("id = ? AND state IN ?", id.String(), terminalStates).Delete(&OperationRecord{})
	if result.Error != nil {
		return apperror.Wrap(apperror.CodeIOWriteFailed, "删除 Operation 历史失败", result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}
	operation, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return apperror.New(apperror.CodeValidationConflict, "活动 Operation 不能删除").WithDetails(map[string]any{
		"operationID": operation.ID, "state": operation.State,
	})
}

// ClearHistory deletes all terminal Operations without touching pending or running records.
func (r *operationRepository) ClearHistory(ctx context.Context) (int64, error) {
	terminalStates := []string{
		enums.OperationSucceeded.String(), enums.OperationFailed.String(), enums.OperationCancelled.String(),
	}
	result := r.database.WithContext(ctx).Where("state IN ?", terminalStates).Delete(&OperationRecord{})
	if result.Error != nil {
		return 0, apperror.Wrap(apperror.CodeIOWriteFailed, "清空 Operation 历史失败", result.Error)
	}
	return result.RowsAffected, nil
}

func queryOperations(database *gorm.DB) ([]model.Operation, error) {
	var records []OperationRecord
	if err := database.Find(&records).Error; err != nil {
		return nil, apperror.Wrap(apperror.CodeIOReadFailed, "查询 Operation 列表失败", err)
	}
	operations := make([]model.Operation, len(records))
	for index, record := range records {
		operations[index] = recordToOperation(record)
	}
	return operations, nil
}

func operationToRecord(operation *model.Operation) OperationRecord {
	if operation == nil {
		return OperationRecord{}
	}
	var retryOf *string
	if operation.RetryOf != nil {
		value := operation.RetryOf.String()
		retryOf = &value
	}
	return OperationRecord{
		ID: operation.ID.String(), Type: operation.Type.String(), TargetType: operation.TargetType.String(),
		TargetID: operation.TargetID.String(), State: operation.State.String(), Stage: operation.Stage,
		Progress: operation.Progress, Message: operation.Message, ErrorCode: operation.ErrorCode,
		ErrorDetails: operation.ErrorDetails, CreatedAt: operation.CreatedAt, StartedAt: operation.StartedAt,
		FinishedAt: operation.FinishedAt, RetryOf: retryOf,
	}
}

func recordToOperation(record OperationRecord) model.Operation {
	var retryOf *model.ID
	if record.RetryOf != nil {
		value := model.ID(*record.RetryOf)
		retryOf = &value
	}
	return model.Operation{
		ID: model.ID(record.ID), Type: enums.OperationType(record.Type), TargetType: enums.OperationTargetType(record.TargetType),
		TargetID: model.ID(record.TargetID), State: enums.OperationState(record.State), Stage: record.Stage,
		Progress: record.Progress, Message: record.Message, ErrorCode: record.ErrorCode,
		ErrorDetails: record.ErrorDetails, CreatedAt: record.CreatedAt, StartedAt: record.StartedAt,
		FinishedAt: record.FinishedAt, RetryOf: retryOf,
	}
}
