package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/port"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// JavaCandidateListResult contains verified remote candidates or a stable error.
type JavaCandidateListResult struct {
	Candidates []service.JavaCandidate `json:"candidates"`
	Error      *apperror.DTO           `json:"error,omitempty"`
}

// JavaRuntimeResult contains one persisted Java Runtime or a stable error.
type JavaRuntimeResult struct {
	JavaRuntime *model.JavaRuntime `json:"javaRuntime,omitempty"`
	Error       *apperror.DTO      `json:"error,omitempty"`
}

// JavaRuntimeListResult contains persisted Java Runtimes or a stable error.
type JavaRuntimeListResult struct {
	JavaRuntimes []model.JavaRuntime `json:"javaRuntimes"`
	Error        *apperror.DTO       `json:"error,omitempty"`
}

// JDKArtifactListResult contains approved provider-neutral download artifacts or a stable error.
type JDKArtifactListResult struct {
	Artifacts []port.JDKArtifact `json:"artifacts"`
	Error     *apperror.DTO      `json:"error,omitempty"`
}

// JavaInstallResult contains a started managed JDK installation Operation or a stable error.
type JavaInstallResult struct {
	OperationID string        `json:"operationID,omitempty"`
	Error       *apperror.DTO `json:"error,omitempty"`
}

// JavaRuntimeService exposes discovery, validation, import, default, delete, and recommendation workflows.
type JavaRuntimeService struct {
	manager *service.JavaRuntimeManager
	logger  *applog.Logger
}

// NewJavaRuntimeService creates the desktop Java Runtime facade.
func NewJavaRuntimeService(manager *service.JavaRuntimeManager, logger *applog.Logger) *JavaRuntimeService {
	return &JavaRuntimeService{manager: manager, logger: logger}
}

// List returns Java Runtimes filtered by SSH Session and Java major.
func (s *JavaRuntimeService) List(ctx context.Context, sshSessionID string, majorVersion, limit, offset int) (result JavaRuntimeListResult) {
	defer s.recoverList(ctx, "JavaRuntimeService.List", &result)
	rows, err := s.manager.List(ctx, repository.JavaRuntimeQuery{
		SSHSessionID: model.ID(sshSessionID), MajorVersion: majorVersion, Limit: limit, Offset: offset,
	})
	if err != nil {
		dto := apperror.ToDTO(err)
		return JavaRuntimeListResult{Error: &dto}
	}
	return JavaRuntimeListResult{JavaRuntimes: rows}
}

// ListArtifacts returns approved latest Linux JDK artifacts for one Java major and architecture.
func (s *JavaRuntimeService) ListArtifacts(ctx context.Context, majorVersion int, architecture string) (result JDKArtifactListResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, "JavaRuntimeService.ListArtifacts", &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	artifacts, err := s.manager.ListArtifacts(ctx, majorVersion, architecture)
	if err != nil {
		dto := apperror.ToDTO(err)
		return JDKArtifactListResult{Error: &dto}
	}
	return JDKArtifactListResult{Artifacts: artifacts}
}

// Discover validates remote managed, JAVA_HOME, PATH, and system candidates.
func (s *JavaRuntimeService) Discover(ctx context.Context, sshSessionID string) (result JavaCandidateListResult) {
	defer s.recoverCandidates(ctx, "JavaRuntimeService.Discover", &result)
	candidates, err := s.manager.Discover(ctx, model.ID(sshSessionID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return JavaCandidateListResult{Error: &dto}
	}
	return JavaCandidateListResult{Candidates: candidates}
}

// Validate checks one remote Java executable without persisting it.
func (s *JavaRuntimeService) Validate(ctx context.Context, sshSessionID, executablePath string) (result JavaCandidateListResult) {
	defer s.recoverCandidates(ctx, "JavaRuntimeService.Validate", &result)
	candidate, err := s.manager.Validate(ctx, model.ID(sshSessionID), executablePath, model.JavaSourceManual)
	if err != nil {
		dto := apperror.ToDTO(err)
		return JavaCandidateListResult{Error: &dto}
	}
	return JavaCandidateListResult{Candidates: []service.JavaCandidate{candidate}}
}

// Import validates and registers one remote Java executable.
func (s *JavaRuntimeService) Import(ctx context.Context, sshSessionID, executablePath string) (result JavaRuntimeResult) {
	defer s.recoverOne(ctx, "JavaRuntimeService.Import", &result)
	javaRuntime, err := s.manager.Import(ctx, model.ID(sshSessionID), executablePath, model.JavaSourceManual)
	if err != nil {
		dto := apperror.ToDTO(err)
		return JavaRuntimeResult{Error: &dto}
	}
	return JavaRuntimeResult{JavaRuntime: javaRuntime}
}

// Install starts a managed remote JDK download, safety validation, extraction, and registration Operation.
func (s *JavaRuntimeService) Install(ctx context.Context, sshSessionID string, majorVersion int, architecture string) (result JavaInstallResult) {
	defer s.recoverInstall(ctx, &result)
	operationID, err := s.manager.StartInstall(ctx, model.ID(sshSessionID), majorVersion, architecture)
	if err != nil {
		dto := apperror.ToDTO(err)
		return JavaInstallResult{Error: &dto}
	}
	return JavaInstallResult{OperationID: operationID.String()}
}

// SetDefault selects one Java Runtime as the SSH Session default.
func (s *JavaRuntimeService) SetDefault(ctx context.Context, sshSessionID, id string) (result ActionResult) {
	defer s.recoverAction(ctx, "JavaRuntimeService.SetDefault", &result)
	if err := s.manager.SetDefault(ctx, model.ID(sshSessionID), model.ID(id)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// Recommend returns the closest compatible reusable Runtime for a server version.
func (s *JavaRuntimeService) Recommend(ctx context.Context, sshSessionID, serverType, minecraftVersion string) (result JavaRuntimeResult) {
	defer s.recoverOne(ctx, "JavaRuntimeService.Recommend", &result)
	javaRuntime, err := s.manager.Recommend(ctx, model.ID(sshSessionID), serverType, minecraftVersion)
	if err != nil {
		dto := apperror.ToDTO(err)
		return JavaRuntimeResult{Error: &dto}
	}
	return JavaRuntimeResult{JavaRuntime: javaRuntime}
}

// Delete removes an unreferenced registration without deleting remote Java files.
func (s *JavaRuntimeService) Delete(ctx context.Context, id string) (result ActionResult) {
	defer s.recoverAction(ctx, "JavaRuntimeService.Delete", &result)
	if err := s.manager.Delete(ctx, model.ID(id)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

func (s *JavaRuntimeService) recoverCandidates(ctx context.Context, boundary string, result *JavaCandidateListResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *JavaRuntimeService) recoverOne(ctx context.Context, boundary string, result *JavaRuntimeResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *JavaRuntimeService) recoverList(ctx context.Context, boundary string, result *JavaRuntimeListResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *JavaRuntimeService) recoverAction(ctx context.Context, boundary string, result *ActionResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *JavaRuntimeService) recoverInstall(ctx context.Context, result *JavaInstallResult) {
	var err error
	apperror.Recover(ctx, s.logger, "JavaRuntimeService.Install", &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}
