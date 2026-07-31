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

// JavaCandidateListResult 承载已校验的远端 Java 候选或稳定错误。
type JavaCandidateListResult struct {
	Candidates []service.JavaCandidate `json:"candidates"`
	Error      *apperror.DTO           `json:"error,omitempty"`
}

// JavaRuntimeResult 承载一条已持久化的 Java 运行时或稳定错误。
type JavaRuntimeResult struct {
	JavaRuntime *model.JavaRuntime `json:"javaRuntime,omitempty"`
	Error       *apperror.DTO      `json:"error,omitempty"`
}

// JavaRuntimeListResult 承载已持久化的 Java 运行时列表或稳定错误。
type JavaRuntimeListResult struct {
	JavaRuntimes []model.JavaRuntime `json:"javaRuntimes"`
	Error        *apperror.DTO       `json:"error,omitempty"`
}

// JDKArtifactListResult 承载已核准、与供应方无关的下载构件或稳定错误。
type JDKArtifactListResult struct {
	Artifacts []port.JDKArtifact `json:"artifacts"`
	Error     *apperror.DTO      `json:"error,omitempty"`
}

// JavaInstallResult 承载已启动的受管 JDK 安装 Operation 或稳定错误。
type JavaInstallResult struct {
	OperationID string        `json:"operationID,omitempty"`
	Error       *apperror.DTO `json:"error,omitempty"`
}

// JavaRuntimeService 对外暴露探测、校验、导入、设默认、删除与推荐流程。
type JavaRuntimeService struct {
	manager *service.JavaRuntimeManager
	logger  *applog.Logger
}

// NewJavaRuntimeService 创建桌面侧的 Java 运行时门面。
func NewJavaRuntimeService(manager *service.JavaRuntimeManager, logger *applog.Logger) *JavaRuntimeService {
	return &JavaRuntimeService{manager: manager, logger: logger}
}

// List 按 SSH Session 与 Java 主版本过滤返回 Java 运行时。
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

// ListArtifacts 返回指定 Java 主版本与架构下已核准的最新 Linux JDK 构件。
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

// Discover 校验远端受管目录、JAVA_HOME、PATH 与系统路径下的候选。
func (s *JavaRuntimeService) Discover(ctx context.Context, sshSessionID string) (result JavaCandidateListResult) {
	defer s.recoverCandidates(ctx, "JavaRuntimeService.Discover", &result)
	candidates, err := s.manager.Discover(ctx, model.ID(sshSessionID))
	if err != nil {
		dto := apperror.ToDTO(err)
		return JavaCandidateListResult{Error: &dto}
	}
	return JavaCandidateListResult{Candidates: candidates}
}

// Validate 校验一个远端 Java 可执行文件,不做持久化。
func (s *JavaRuntimeService) Validate(ctx context.Context, sshSessionID, executablePath string) (result JavaCandidateListResult) {
	defer s.recoverCandidates(ctx, "JavaRuntimeService.Validate", &result)
	candidate, err := s.manager.Validate(ctx, model.ID(sshSessionID), executablePath, model.JavaSourceManual)
	if err != nil {
		dto := apperror.ToDTO(err)
		return JavaCandidateListResult{Error: &dto}
	}
	return JavaCandidateListResult{Candidates: []service.JavaCandidate{candidate}}
}

// Import 校验并登记一个远端 Java 可执行文件。
func (s *JavaRuntimeService) Import(ctx context.Context, sshSessionID, executablePath string) (result JavaRuntimeResult) {
	defer s.recoverOne(ctx, "JavaRuntimeService.Import", &result)
	javaRuntime, err := s.manager.Import(ctx, model.ID(sshSessionID), executablePath, model.JavaSourceManual)
	if err != nil {
		dto := apperror.ToDTO(err)
		return JavaRuntimeResult{Error: &dto}
	}
	return JavaRuntimeResult{JavaRuntime: javaRuntime}
}

// Install 启动一次受管的远端 JDK 下载、安全校验、解压与登记 Operation。
func (s *JavaRuntimeService) Install(ctx context.Context, sshSessionID string, majorVersion int, architecture string) (result JavaInstallResult) {
	defer s.recoverInstall(ctx, &result)
	operationID, err := s.manager.StartInstall(ctx, model.ID(sshSessionID), majorVersion, architecture)
	if err != nil {
		dto := apperror.ToDTO(err)
		return JavaInstallResult{Error: &dto}
	}
	return JavaInstallResult{OperationID: operationID.String()}
}

// SetDefault 把一条 Java 运行时选为该 SSH Session 的默认项。
func (s *JavaRuntimeService) SetDefault(ctx context.Context, sshSessionID, id string) (result ActionResult) {
	defer s.recoverAction(ctx, "JavaRuntimeService.SetDefault", &result)
	if err := s.manager.SetDefault(ctx, model.ID(sshSessionID), model.ID(id)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// Recommend 为某个服务端版本返回最接近的可复用兼容运行时。
func (s *JavaRuntimeService) Recommend(ctx context.Context, sshSessionID, serverType, minecraftVersion string) (result JavaRuntimeResult) {
	defer s.recoverOne(ctx, "JavaRuntimeService.Recommend", &result)
	javaRuntime, err := s.manager.Recommend(ctx, model.ID(sshSessionID), serverType, minecraftVersion)
	if err != nil {
		dto := apperror.ToDTO(err)
		return JavaRuntimeResult{Error: &dto}
	}
	return JavaRuntimeResult{JavaRuntime: javaRuntime}
}

// Delete 删除一条无引用的登记,不删除远端 Java 文件。
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
