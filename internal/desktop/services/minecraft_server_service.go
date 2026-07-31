package services

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
	"github.com/Cail-Gainey/MineOps/internal/service"
)

// MinecraftServerInput 承载可编辑元数据与结构化启动参数。
type MinecraftServerInput struct {
	SSHSessionID   string              `json:"sshSessionID"`
	JavaRuntimeID  string              `json:"javaRuntimeID"`
	Name           string              `json:"name"`
	Type           string              `json:"type"`
	Version        string              `json:"version"`
	RemotePath     string              `json:"remotePath"`
	Group          string              `json:"group"`
	Tags           []string            `json:"tags"`
	Favourite      bool                `json:"favourite"`
	LaunchProfile  model.LaunchProfile `json:"launchProfile"`
	FirewallPolicy string              `json:"firewallPolicy"`
	EULAAccepted   bool                `json:"eulaAccepted"`
}

// MinecraftServerResult 承载一台 Server 或稳定错误。
type MinecraftServerResult struct {
	Server *model.MinecraftServer `json:"server,omitempty"`
	Error  *apperror.DTO          `json:"error,omitempty"`
}

// MinecraftServerListResult 承载 Server 列表或稳定错误。
type MinecraftServerListResult struct {
	Servers []model.MinecraftServer `json:"servers"`
	Error   *apperror.DTO           `json:"error,omitempty"`
}

// RemoteServerInspectionResult 承载只读的导入探测证据或稳定错误。
type RemoteServerInspectionResult struct {
	Inspection *service.RemoteServerInspection `json:"inspection,omitempty"`
	Error      *apperror.DTO                   `json:"error,omitempty"`
}

// MinecraftServerOperationResult 承载已启动的破坏性 Operation 或稳定错误。
type MinecraftServerOperationResult struct {
	OperationID string        `json:"operationID,omitempty"`
	Error       *apperror.DTO `json:"error,omitempty"`
}

// ServerBackupListResult 承载受管的远端 Server 备份或稳定错误。
type ServerBackupListResult struct {
	Backups []service.ServerBackup `json:"backups"`
	Error   *apperror.DTO          `json:"error,omitempty"`
}

// ServerPropertiesResult 承载已解析的 server.properties 状态或稳定错误。
type ServerPropertiesResult struct {
	Properties *service.ServerPropertiesSnapshot `json:"properties,omitempty"`
	Error      *apperror.DTO                     `json:"error,omitempty"`
}

// ServerPropertiesBackupListResult 承载保留的配置历史版本或稳定错误。
type ServerPropertiesBackupListResult struct {
	Backups []service.ServerPropertiesBackup `json:"backups"`
	Error   *apperror.DTO                    `json:"error,omitempty"`
}

// ServerInstallationStatusResult 承载远端安装一致性证据或稳定错误。
type ServerInstallationStatusResult struct {
	Status *service.ServerInstallationStatus `json:"status,omitempty"`
	Error  *apperror.DTO                     `json:"error,omitempty"`
}

// MinecraftServerService 对外暴露加密元数据的增删改查与删除保护。
type MinecraftServerService struct {
	manager *service.MinecraftServerManager
	store   repository.Store
	logger  *applog.Logger
}

// NewMinecraftServerService 创建桌面侧的 Minecraft Server 门面。
func NewMinecraftServerService(manager *service.MinecraftServerManager, store repository.Store, logger *applog.Logger) *MinecraftServerService {
	return &MinecraftServerService{manager: manager, store: store, logger: logger}
}

// List 按过滤条件返回 Server 列表。
func (s *MinecraftServerService) List(ctx context.Context, search, sshSessionID, group, tag, state string, includeDeleted bool, limit, offset int) (result MinecraftServerListResult) {
	defer s.recoverList(ctx, "MinecraftServerService.List", &result)
	servers, err := s.store.MinecraftServers().List(ctx, repository.MinecraftServerQuery{
		Search: search, SSHSessionID: model.ID(sshSessionID), Group: group, Tag: tag,
		State: enums.LifecycleState(state), IncludeDeleted: includeDeleted, Limit: limit, Offset: offset,
	})
	if err != nil {
		dto := apperror.ToDTO(err)
		return MinecraftServerListResult{Error: &dto}
	}
	return MinecraftServerListResult{Servers: servers}
}

// Get 返回一台 Server,可选择是否包含已软删除记录。
func (s *MinecraftServerService) Get(ctx context.Context, id string, includeDeleted bool) (result MinecraftServerResult) {
	defer s.recoverOne(ctx, "MinecraftServerService.Get", &result)
	server, err := s.store.MinecraftServers().Get(ctx, model.ID(id), includeDeleted)
	if err != nil {
		dto := apperror.ToDTO(err)
		return MinecraftServerResult{Error: &dto}
	}
	return MinecraftServerResult{Server: server}
}

// Create 持久化一条绑定 SSH、处于创建中状态的 Server 记录。
func (s *MinecraftServerService) Create(ctx context.Context, input MinecraftServerInput) (result MinecraftServerResult) {
	defer s.recoverOne(ctx, "MinecraftServerService.Create", &result)
	server, err := s.manager.Create(ctx, input.command())
	if err != nil {
		dto := apperror.ToDTO(err)
		return MinecraftServerResult{Error: &dto}
	}
	return MinecraftServerResult{Server: server}
}

// InspectRemote 在导入已有远端 Server 之前执行只读校验。
func (s *MinecraftServerService) InspectRemote(ctx context.Context, sshSessionID, remotePath string) (result RemoteServerInspectionResult) {
	defer s.recoverInspection(ctx, &result)
	inspection, err := s.manager.InspectRemote(ctx, model.ID(sshSessionID), remotePath)
	if err != nil {
		dto := apperror.ToDTO(err)
		return RemoteServerInspectionResult{Error: &dto}
	}
	return RemoteServerInspectionResult{Inspection: &inspection}
}

// ImportRemote 登记已探测的远端文件,不改动原目录。
func (s *MinecraftServerService) ImportRemote(ctx context.Context, input MinecraftServerInput) (result MinecraftServerResult) {
	defer s.recoverOne(ctx, "MinecraftServerService.ImportRemote", &result)
	server, err := s.manager.ImportRemote(ctx, input.command())
	if err != nil {
		dto := apperror.ToDTO(err)
		return MinecraftServerResult{Error: &dto}
	}
	return MinecraftServerResult{Server: server}
}

// Update 只改元数据,不移动远端目录也不改 SSH 绑定。
func (s *MinecraftServerService) Update(ctx context.Context, id string, input MinecraftServerInput) (result MinecraftServerResult) {
	defer s.recoverOne(ctx, "MinecraftServerService.Update", &result)
	server, err := s.manager.Update(ctx, model.ID(id), input.command())
	if err != nil {
		dto := apperror.ToDTO(err)
		return MinecraftServerResult{Error: &dto}
	}
	return MinecraftServerResult{Server: server}
}

// SoftDelete 保留远端目录,仅把元数据标记为已删除。
func (s *MinecraftServerService) SoftDelete(ctx context.Context, id string) (result ActionResult) {
	defer s.recoverAction(ctx, "MinecraftServerService.SoftDelete", &result)
	if err := s.manager.SoftDelete(ctx, model.ID(id)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// Restore 把已软删除的 Server 恢复为已停止状态。
func (s *MinecraftServerService) Restore(ctx context.Context, id string) (result ActionResult) {
	defer s.recoverAction(ctx, "MinecraftServerService.Restore", &result)
	if err := s.manager.Restore(ctx, model.ID(id)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// HardDeleteRegistration 在名称与路径精确确认后,仅删除元数据。
func (s *MinecraftServerService) HardDeleteRegistration(ctx context.Context, id, confirmedName, confirmedPath string) (result ActionResult) {
	defer s.recoverAction(ctx, "MinecraftServerService.HardDeleteRegistration", &result)
	if err := s.manager.HardDeleteRegistration(ctx, model.ID(id), confirmedName, confirmedPath); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// HardDeleteRemote 在精确确认后启动远端目录与登记的永久删除。
func (s *MinecraftServerService) HardDeleteRemote(ctx context.Context, id, confirmedName, confirmedPath string) (result MinecraftServerOperationResult) {
	defer s.recoverOperation(ctx, "MinecraftServerService.HardDeleteRemote", &result)
	operationID, err := s.manager.StartHardDeleteRemote(ctx, model.ID(id), confirmedName, confirmedPath)
	if err != nil {
		dto := apperror.ToDTO(err)
		return MinecraftServerOperationResult{Error: &dto}
	}
	return MinecraftServerOperationResult{OperationID: operationID.String()}
}

// ListBackups 返回某台 Server 由 MineOps 管理的远端归档。
func (s *MinecraftServerService) ListBackups(ctx context.Context, id string) (result ServerBackupListResult) {
	defer s.recoverBackupList(ctx, &result)
	backups, err := s.manager.ListBackups(ctx, model.ID(id))
	if err != nil {
		dto := apperror.ToDTO(err)
		return ServerBackupListResult{Error: &dto}
	}
	return ServerBackupListResult{Backups: backups}
}

// CreateBackup 启动一次受管的远端 tar.gz 备份 Operation。
func (s *MinecraftServerService) CreateBackup(ctx context.Context, id string) (result MinecraftServerOperationResult) {
	defer s.recoverOperation(ctx, "MinecraftServerService.CreateBackup", &result)
	operationID, err := s.manager.StartBackup(ctx, model.ID(id))
	if err != nil {
		dto := apperror.ToDTO(err)
		return MinecraftServerOperationResult{Error: &dto}
	}
	return MinecraftServerOperationResult{OperationID: operationID.String()}
}

// RestoreBackup 启动安全校验与 Server 目录的原子恢复。
func (s *MinecraftServerService) RestoreBackup(ctx context.Context, id, backupPath string) (result MinecraftServerOperationResult) {
	defer s.recoverOperation(ctx, "MinecraftServerService.RestoreBackup", &result)
	operationID, err := s.manager.StartRestoreBackup(ctx, model.ID(id), backupPath)
	if err != nil {
		dto := apperror.ToDTO(err)
		return MinecraftServerOperationResult{Error: &dto}
	}
	return MinecraftServerOperationResult{OperationID: operationID.String()}
}

// ReadProperties 返回 server.properties 的原文与结构化状态。
func (s *MinecraftServerService) ReadProperties(ctx context.Context, id string) (result ServerPropertiesResult) {
	defer s.recoverProperties(ctx, "MinecraftServerService.ReadProperties", &result)
	properties, err := s.manager.ReadProperties(ctx, model.ID(id))
	if err != nil {
		dto := apperror.ToDTO(err)
		return ServerPropertiesResult{Error: &dto}
	}
	return ServerPropertiesResult{Properties: properties}
}

// SaveProperties 按原文或结构化方式保存,带冲突检测并保留备份。
func (s *MinecraftServerService) SaveProperties(ctx context.Context, id, mode, rawContent, expectedVersion string, updates []service.ServerPropertyUpdate, firewallConfirmed bool) (result ServerPropertiesResult) {
	defer s.recoverProperties(ctx, "MinecraftServerService.SaveProperties", &result)
	properties, err := s.manager.SaveProperties(ctx, model.ID(id), mode, rawContent, expectedVersion, updates, firewallConfirmed)
	if err != nil {
		dto := apperror.ToDTO(err)
		return ServerPropertiesResult{Error: &dto}
	}
	return ServerPropertiesResult{Properties: properties}
}

// ListPropertyBackups 返回保存前保留的 server.properties 历史版本。
func (s *MinecraftServerService) ListPropertyBackups(ctx context.Context, id string) (result ServerPropertiesBackupListResult) {
	defer s.recoverPropertyBackups(ctx, &result)
	backups, err := s.manager.ListPropertyBackups(ctx, model.ID(id))
	if err != nil {
		dto := apperror.ToDTO(err)
		return ServerPropertiesBackupListResult{Error: &dto}
	}
	return ServerPropertiesBackupListResult{Backups: backups}
}

// RestorePropertyBackup 校验当前版本标识后恢复一个保留的历史版本。
func (s *MinecraftServerService) RestorePropertyBackup(ctx context.Context, id, backupPath, expectedVersion string, firewallConfirmed bool) (result ServerPropertiesResult) {
	defer s.recoverProperties(ctx, "MinecraftServerService.RestorePropertyBackup", &result)
	properties, err := s.manager.RestorePropertyBackup(ctx, model.ID(id), backupPath, expectedVersion, firewallConfirmed)
	if err != nil {
		dto := apperror.ToDTO(err)
		return ServerPropertiesResult{Error: &dto}
	}
	return ServerPropertiesResult{Properties: properties}
}

// InspectInstallationStatus 把远端文件与 Server 元数据、最近的安装检查点做比对。
func (s *MinecraftServerService) InspectInstallationStatus(ctx context.Context, id string) (result ServerInstallationStatusResult) {
	defer s.recoverInstallationStatus(ctx, &result)
	status, err := s.manager.InspectInstallationStatus(ctx, model.ID(id))
	if err != nil {
		dto := apperror.ToDTO(err)
		return ServerInstallationStatusResult{Error: &dto}
	}
	return ServerInstallationStatusResult{Status: &status}
}

func (i MinecraftServerInput) command() service.MinecraftServerCommand {
	var javaRuntimeID *model.ID
	if i.JavaRuntimeID != "" {
		value := model.ID(i.JavaRuntimeID)
		javaRuntimeID = &value
	}
	return service.MinecraftServerCommand{
		SSHSessionID: model.ID(i.SSHSessionID), JavaRuntimeID: javaRuntimeID,
		Name: i.Name, Type: enums.MinecraftServerType(i.Type), Version: i.Version,
		RemotePath: i.RemotePath, Group: i.Group, Tags: i.Tags, Favourite: i.Favourite,
		LaunchProfile: i.LaunchProfile, FirewallPolicy: enums.FirewallPolicy(i.FirewallPolicy),
		EULAAccepted: i.EULAAccepted,
	}
}

func (s *MinecraftServerService) recoverOne(ctx context.Context, boundary string, result *MinecraftServerResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *MinecraftServerService) recoverList(ctx context.Context, boundary string, result *MinecraftServerListResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *MinecraftServerService) recoverInspection(ctx context.Context, result *RemoteServerInspectionResult) {
	var err error
	apperror.Recover(ctx, s.logger, "MinecraftServerService.InspectRemote", &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *MinecraftServerService) recoverOperation(ctx context.Context, boundary string, result *MinecraftServerOperationResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *MinecraftServerService) recoverBackupList(ctx context.Context, result *ServerBackupListResult) {
	var err error
	apperror.Recover(ctx, s.logger, "MinecraftServerService.ListBackups", &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *MinecraftServerService) recoverProperties(ctx context.Context, boundary string, result *ServerPropertiesResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *MinecraftServerService) recoverPropertyBackups(ctx context.Context, result *ServerPropertiesBackupListResult) {
	var err error
	apperror.Recover(ctx, s.logger, "MinecraftServerService.ListPropertyBackups", &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *MinecraftServerService) recoverInstallationStatus(ctx context.Context, result *ServerInstallationStatusResult) {
	var err error
	apperror.Recover(ctx, s.logger, "MinecraftServerService.InspectInstallationStatus", &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *MinecraftServerService) recoverAction(ctx context.Context, boundary string, result *ActionResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}
