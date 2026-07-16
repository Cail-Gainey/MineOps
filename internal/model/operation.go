package model

import (
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
)

// Operation is the durable record for a cancellable long-running MineOps action.
type Operation struct {
	ID           ID                        `json:"id"`
	Type         enums.OperationType       `json:"type"`
	TargetType   enums.OperationTargetType `json:"targetType"`
	TargetID     ID                        `json:"targetID"`
	State        enums.OperationState      `json:"state"`
	Stage        string                    `json:"stage"`
	Progress     float64                   `json:"progress"`
	Message      string                    `json:"message"`
	ErrorCode    string                    `json:"errorCode,omitempty"`
	ErrorDetails map[string]any            `json:"errorDetails,omitempty"`
	CreatedAt    time.Time                 `json:"createdAt"`
	StartedAt    *time.Time                `json:"startedAt,omitempty"`
	FinishedAt   *time.Time                `json:"finishedAt,omitempty"`
	RetryOf      *ID                       `json:"retryOf,omitempty"`
}

// NewOperation creates a validated pending operation with an ordered ID and UTC timestamp.
func NewOperation(clock Clock, operationType enums.OperationType, targetType enums.OperationTargetType, targetID ID) (*Operation, error) {
	if clock == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Clock 不能为空")
	}
	if !operationType.Valid() || !targetType.Valid() || !targetID.Valid() {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "Operation 类型或目标无效")
	}
	now := clock.Now().UTC()
	id, err := NewID(now)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, "生成 Operation ID 失败", err)
	}
	return &Operation{
		ID:         id,
		Type:       operationType,
		TargetType: targetType,
		TargetID:   targetID,
		State:      enums.OperationPending,
		CreatedAt:  now,
	}, nil
}

// CanTransition reports whether the durable state machine permits the requested transition.
func (o Operation) CanTransition(next enums.OperationState) bool {
	if !next.Valid() || o.State == next {
		return false
	}
	switch o.State {
	case enums.OperationPending:
		return next == enums.OperationRunning || next == enums.OperationCancelled || next == enums.OperationFailed
	case enums.OperationRunning:
		return next == enums.OperationSucceeded || next == enums.OperationFailed || next == enums.OperationCancelled
	case enums.OperationSucceeded, enums.OperationFailed, enums.OperationCancelled:
		return false
	default:
		return false
	}
}

// Transition applies a legal state change and maintains start/finish timestamps.
func (o *Operation) Transition(clock Clock, next enums.OperationState, failure error) error {
	if o == nil || clock == nil {
		return apperror.New(apperror.CodeValidationRequired, "Operation 和 Clock 不能为空")
	}
	if !o.CanTransition(next) {
		return apperror.New(apperror.CodeValidationConflict, "Operation 状态转换无效").WithDetails(map[string]any{
			"operationID": o.ID,
			"current":     o.State,
			"next":        next,
		})
	}
	now := clock.Now().UTC()
	if next == enums.OperationRunning {
		o.StartedAt = &now
	}
	if next == enums.OperationSucceeded || next == enums.OperationFailed || next == enums.OperationCancelled {
		o.FinishedAt = &now
	}
	o.State = next
	if next == enums.OperationSucceeded {
		o.Progress = 1
		o.ErrorCode = ""
		o.ErrorDetails = nil
	}
	if failure != nil {
		dto := apperror.ToDTO(failure)
		o.ErrorCode = dto.Code
		o.ErrorDetails = dto.Details
		o.Message = dto.Message
	}
	return nil
}

// SetProgress updates the current stage and normalized progress for a running operation.
func (o *Operation) SetProgress(stage string, progress float64, message string) error {
	if o == nil || o.State != enums.OperationRunning {
		return apperror.New(apperror.CodeValidationConflict, "只有 Running Operation 可以更新进度")
	}
	if progress < 0 || progress > 1 {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Operation 进度必须在 0 到 1 之间")
	}
	o.Stage = stage
	o.Progress = progress
	o.Message = message
	return nil
}
