package model

import (
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
)

// InstallationTask 是某台 Minecraft Server 带检查点的持久化安装聚合。
type InstallationTask struct {
	ID          ID                      `json:"id"`
	ServerID    ID                      `json:"serverID"`
	OperationID *ID                     `json:"operationID,omitempty"`
	State       enums.InstallationState `json:"state"`
	CurrentStep int                     `json:"currentStep"`
	LogCursor   int64                   `json:"logCursor"`
	CreatedAt   time.Time               `json:"createdAt"`
	StartedAt   *time.Time              `json:"startedAt,omitempty"`
	FinishedAt  *time.Time              `json:"finishedAt,omitempty"`
	UpdatedAt   time.Time               `json:"updatedAt"`
}

// InstallationStep 是 InstallationTask 中一个可重试、带检查点的阶段。
type InstallationStep struct {
	ID           ID                          `json:"id"`
	TaskID       ID                          `json:"taskID"`
	Order        int                         `json:"order"`
	Name         string                      `json:"name"`
	State        enums.InstallationStepState `json:"state"`
	Attempt      int                         `json:"attempt"`
	Progress     float64                     `json:"progress"`
	Checkpoint   map[string]any              `json:"checkpoint,omitempty"`
	ErrorCode    string                      `json:"errorCode,omitempty"`
	ErrorDetails map[string]any              `json:"errorDetails,omitempty"`
	Message      string                      `json:"message"`
	StartedAt    *time.Time                  `json:"startedAt,omitempty"`
	FinishedAt   *time.Time                  `json:"finishedAt,omitempty"`
	UpdatedAt    time.Time                   `json:"updatedAt"`
}

// InstallationStepCount 是标准安装流程中固定的步骤数量。
//
// 前端进度换算需要这个总数;修改步骤列表时必须同步更新它,NewInstallationTask 会做一致性校验。
const InstallationStepCount = 11

// NewInstallationTask 创建标准的十一步 Minecraft 安装流程。
func NewInstallationTask(clock Clock, serverID ID) (*InstallationTask, []InstallationStep, error) {
	if clock == nil || !serverID.Valid() {
		return nil, nil, apperror.New(apperror.CodeValidationRequired, "Installation Clock 和 Server ID 不能为空")
	}
	now := clock.Now().UTC()
	taskID, err := NewID(now)
	if err != nil {
		return nil, nil, apperror.Wrap(apperror.CodeInternal, "生成 Installation Task ID 失败", err)
	}
	task := &InstallationTask{ID: taskID, ServerID: serverID, State: enums.InstallationWaiting, CreatedAt: now, UpdatedAt: now}
	names := []string{
		"connect_ssh", "initialize_directories", "create_server_directory", "resolve_java",
		"install_java", "download_server", "install_server", "write_eula", "configure_firewall", "first_start", "register_server",
	}
	if len(names) != InstallationStepCount {
		return nil, nil, apperror.New(apperror.CodeInternal, "Installation 步骤总数与 InstallationStepCount 不一致")
	}
	steps := make([]InstallationStep, len(names))
	for index, name := range names {
		stepID, idErr := NewID(now.Add(time.Duration(index+1) * time.Nanosecond))
		if idErr != nil {
			return nil, nil, apperror.Wrap(apperror.CodeInternal, "生成 Installation Step ID 失败", idErr)
		}
		steps[index] = InstallationStep{
			ID: stepID, TaskID: taskID, Order: index + 1, Name: name,
			State: enums.InstallationStepWaiting, UpdatedAt: now,
		}
	}
	return task, steps, nil
}

// Start 把等待中或可重试的任务切换到运行中状态。
func (t *InstallationTask) Start(clock Clock) error {
	if t == nil || clock == nil || t.State != enums.InstallationWaiting && t.State != enums.InstallationFailed {
		return apperror.New(apperror.CodeValidationConflict, "Installation Task 当前不能启动")
	}
	now := clock.Now().UTC()
	t.State = enums.InstallationRunning
	t.StartedAt = &now
	t.FinishedAt = nil
	t.UpdatedAt = now
	return nil
}

// Complete 把运行中的安装切换到成功、失败或已取消。
func (t *InstallationTask) Complete(clock Clock, state enums.InstallationState) error {
	if t == nil || clock == nil || t.State != enums.InstallationRunning {
		return apperror.New(apperror.CodeValidationConflict, "只有 Running Installation Task 可以完成")
	}
	if state != enums.InstallationSucceeded && state != enums.InstallationFailed && state != enums.InstallationCancelled {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Installation Task 完成状态无效")
	}
	now := clock.Now().UTC()
	t.State = state
	t.FinishedAt = &now
	t.UpdatedAt = now
	return nil
}

// PrepareRetry 重置失败或已取消的任务,同时保留已成功步骤的检查点。
func (t *InstallationTask) PrepareRetry(clock Clock) error {
	if t == nil || clock == nil || t.State != enums.InstallationFailed && t.State != enums.InstallationCancelled {
		return apperror.New(apperror.CodeValidationConflict, "Installation Task 当前不能重试")
	}
	t.State = enums.InstallationWaiting
	t.FinishedAt = nil
	t.UpdatedAt = clock.Now().UTC()
	return nil
}

// CancelAfterFailure 记录用户在失败决策点之后的显式取消。
func (t *InstallationTask) CancelAfterFailure(clock Clock) error {
	if t == nil || clock == nil || t.State != enums.InstallationFailed {
		return apperror.New(apperror.CodeValidationConflict, "只有 Failed Installation Task 可以取消")
	}
	now := clock.Now().UTC()
	t.State = enums.InstallationCancelled
	t.FinishedAt = &now
	t.UpdatedAt = now
	return nil
}

// Start 开始一个等待中或失败的步骤,并递增其尝试次数。
func (s *InstallationStep) Start(clock Clock) error {
	if s == nil || clock == nil || s.State != enums.InstallationStepWaiting && s.State != enums.InstallationStepFailed {
		return apperror.New(apperror.CodeValidationConflict, "Installation Step 当前不能启动")
	}
	now := clock.Now().UTC()
	s.State = enums.InstallationStepRunning
	s.Attempt++
	s.Progress = 0
	s.ErrorCode = ""
	s.ErrorDetails = nil
	s.Message = ""
	s.StartedAt = &now
	s.FinishedAt = nil
	s.UpdatedAt = now
	return nil
}

// SetProgress 更新运行中步骤的归一化进度与持久化日志游标。
func (s *InstallationStep) SetProgress(clock Clock, progress float64, message string, logCursor int64) error {
	if s == nil || clock == nil || s.State != enums.InstallationStepRunning {
		return apperror.New(apperror.CodeValidationConflict, "只有 Running Installation Step 可以更新进度")
	}
	if progress < 0 || progress > 1 || logCursor < s.LogCursor() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Installation Step 进度或日志游标无效")
	}
	s.Progress = progress
	s.Message = message
	if s.Checkpoint == nil {
		s.Checkpoint = make(map[string]any)
	}
	if logCursor > s.LogCursor() && strings.TrimSpace(message) != "" {
		logs := checkpointStrings(s.Checkpoint["logs"])
		logs = append(logs, strings.TrimSpace(message))
		if len(logs) > 200 {
			logs = logs[len(logs)-200:]
		}
		s.Checkpoint["logs"] = logs
	}
	s.Checkpoint["logCursor"] = logCursor
	s.UpdatedAt = clock.Now().UTC()
	return nil
}

// LogCursor 返回已检查点化、单调递增的日志游标。
func (s InstallationStep) LogCursor() int64 {
	if s.Checkpoint == nil {
		return 0
	}
	switch value := s.Checkpoint["logCursor"].(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	default:
		return 0
	}
}

// PrepareRetry 把失败或已取消的步骤重置为等待中,不移除此前的检查点。
func (s *InstallationStep) PrepareRetry(clock Clock) error {
	if s == nil || clock == nil || !s.Retryable() {
		return apperror.New(apperror.CodeValidationConflict, "Installation Step 当前不能重试")
	}
	s.State = enums.InstallationStepWaiting
	s.Progress = 0
	s.ErrorCode = ""
	s.ErrorDetails = nil
	s.Message = ""
	s.StartedAt = nil
	s.FinishedAt = nil
	s.UpdatedAt = clock.Now().UTC()
	return nil
}

// Complete 记录成功、失败、跳过或已取消,并附带检查点与稳定错误字段。
func (s *InstallationStep) Complete(clock Clock, state enums.InstallationStepState, checkpoint map[string]any, failure error) error {
	if s == nil || clock == nil || s.State != enums.InstallationStepRunning {
		return apperror.New(apperror.CodeValidationConflict, "只有 Running Installation Step 可以完成")
	}
	if state != enums.InstallationStepSuccess && state != enums.InstallationStepFailed && state != enums.InstallationStepSkipped && state != enums.InstallationStepCancelled {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Installation Step 完成状态无效")
	}
	now := clock.Now().UTC()
	s.State = state
	s.Checkpoint = mergeCheckpoint(s.Checkpoint, checkpoint)
	s.FinishedAt = &now
	s.UpdatedAt = now
	if state == enums.InstallationStepSuccess || state == enums.InstallationStepSkipped {
		s.Progress = 1
	}
	if failure != nil {
		dto := apperror.ToDTO(failure)
		s.ErrorCode = dto.Code
		s.ErrorDetails = cloneCheckpoint(dto.Details)
		s.Message = dto.Message
	}
	return nil
}

// CancelAfterFailure 记录用户在失败决策点上的显式取消。
func (s *InstallationStep) CancelAfterFailure(clock Clock) error {
	if s == nil || clock == nil || s.State != enums.InstallationStepFailed {
		return apperror.New(apperror.CodeValidationConflict, "只有 Failed Installation Step 可以取消")
	}
	now := clock.Now().UTC()
	s.State = enums.InstallationStepCancelled
	s.FinishedAt = &now
	s.UpdatedAt = now
	return nil
}

// Retryable 返回该失败或已取消步骤能否在不重跑已成功步骤的前提下重置。
func (s InstallationStep) Retryable() bool {
	return s.State == enums.InstallationStepFailed || s.State == enums.InstallationStepCancelled
}

func cloneCheckpoint(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func mergeCheckpoint(current, next map[string]any) map[string]any {
	result := cloneCheckpoint(current)
	if result == nil && len(next) > 0 {
		result = make(map[string]any, len(next))
	}
	for key, value := range next {
		result[key] = value
	}
	return result
}

func checkpointStrings(value any) []string {
	switch values := value.(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}
