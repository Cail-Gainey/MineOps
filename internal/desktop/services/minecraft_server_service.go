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

// MinecraftServerInput contains editable metadata and structured launch arguments.
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

// MinecraftServerResult contains one Server or a stable error.
type MinecraftServerResult struct {
	Server *model.MinecraftServer `json:"server,omitempty"`
	Error  *apperror.DTO          `json:"error,omitempty"`
}

// MinecraftServerListResult contains Server rows or a stable error.
type MinecraftServerListResult struct {
	Servers []model.MinecraftServer `json:"servers"`
	Error   *apperror.DTO           `json:"error,omitempty"`
}

// RemoteServerInspectionResult contains read-only import evidence or a stable error.
type RemoteServerInspectionResult struct {
	Inspection *service.RemoteServerInspection `json:"inspection,omitempty"`
	Error      *apperror.DTO                   `json:"error,omitempty"`
}

// MinecraftServerOperationResult contains a started destructive Operation or a stable error.
type MinecraftServerOperationResult struct {
	OperationID string        `json:"operationID,omitempty"`
	Error       *apperror.DTO `json:"error,omitempty"`
}

// ServerBackupListResult contains managed remote Server backups or a stable error.
type ServerBackupListResult struct {
	Backups []service.ServerBackup `json:"backups"`
	Error   *apperror.DTO          `json:"error,omitempty"`
}

// ServerPropertiesResult contains parsed server.properties state or a stable error.
type ServerPropertiesResult struct {
	Properties *service.ServerPropertiesSnapshot `json:"properties,omitempty"`
	Error      *apperror.DTO                     `json:"error,omitempty"`
}

// ServerPropertiesBackupListResult contains retained configuration revisions or a stable error.
type ServerPropertiesBackupListResult struct {
	Backups []service.ServerPropertiesBackup `json:"backups"`
	Error   *apperror.DTO                    `json:"error,omitempty"`
}

// ServerInstallationStatusResult contains remote installation consistency evidence or a stable error.
type ServerInstallationStatusResult struct {
	Status *service.ServerInstallationStatus `json:"status,omitempty"`
	Error  *apperror.DTO                     `json:"error,omitempty"`
}

// MinecraftServerService exposes encrypted metadata CRUD and deletion protection.
type MinecraftServerService struct {
	manager *service.MinecraftServerManager
	store   repository.Store
	logger  *applog.Logger
}

// NewMinecraftServerService creates the desktop Minecraft Server facade.
func NewMinecraftServerService(manager *service.MinecraftServerManager, store repository.Store, logger *applog.Logger) *MinecraftServerService {
	return &MinecraftServerService{manager: manager, store: store, logger: logger}
}

// List returns filtered Server rows.
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

// Get returns one Server, optionally including soft-deleted records.
func (s *MinecraftServerService) Get(ctx context.Context, id string, includeDeleted bool) (result MinecraftServerResult) {
	defer s.recoverOne(ctx, "MinecraftServerService.Get", &result)
	server, err := s.store.MinecraftServers().Get(ctx, model.ID(id), includeDeleted)
	if err != nil {
		dto := apperror.ToDTO(err)
		return MinecraftServerResult{Error: &dto}
	}
	return MinecraftServerResult{Server: server}
}

// Create persists a new SSH-bound Creating Server record.
func (s *MinecraftServerService) Create(ctx context.Context, input MinecraftServerInput) (result MinecraftServerResult) {
	defer s.recoverOne(ctx, "MinecraftServerService.Create", &result)
	server, err := s.manager.Create(ctx, input.command())
	if err != nil {
		dto := apperror.ToDTO(err)
		return MinecraftServerResult{Error: &dto}
	}
	return MinecraftServerResult{Server: server}
}

// InspectRemote performs read-only validation before importing an existing remote Server.
func (s *MinecraftServerService) InspectRemote(ctx context.Context, sshSessionID, remotePath string) (result RemoteServerInspectionResult) {
	defer s.recoverInspection(ctx, &result)
	inspection, err := s.manager.InspectRemote(ctx, model.ID(sshSessionID), remotePath)
	if err != nil {
		dto := apperror.ToDTO(err)
		return RemoteServerInspectionResult{Error: &dto}
	}
	return RemoteServerInspectionResult{Inspection: &inspection}
}

// ImportRemote registers inspected remote files without modifying the original directory.
func (s *MinecraftServerService) ImportRemote(ctx context.Context, input MinecraftServerInput) (result MinecraftServerResult) {
	defer s.recoverOne(ctx, "MinecraftServerService.ImportRemote", &result)
	server, err := s.manager.ImportRemote(ctx, input.command())
	if err != nil {
		dto := apperror.ToDTO(err)
		return MinecraftServerResult{Error: &dto}
	}
	return MinecraftServerResult{Server: server}
}

// Update changes metadata without moving the remote directory or changing SSH binding.
func (s *MinecraftServerService) Update(ctx context.Context, id string, input MinecraftServerInput) (result MinecraftServerResult) {
	defer s.recoverOne(ctx, "MinecraftServerService.Update", &result)
	server, err := s.manager.Update(ctx, model.ID(id), input.command())
	if err != nil {
		dto := apperror.ToDTO(err)
		return MinecraftServerResult{Error: &dto}
	}
	return MinecraftServerResult{Server: server}
}

// SoftDelete preserves the remote directory and marks metadata Deleted.
func (s *MinecraftServerService) SoftDelete(ctx context.Context, id string) (result ActionResult) {
	defer s.recoverAction(ctx, "MinecraftServerService.SoftDelete", &result)
	if err := s.manager.SoftDelete(ctx, model.ID(id)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// Restore restores a soft-deleted Server to Stopped state.
func (s *MinecraftServerService) Restore(ctx context.Context, id string) (result ActionResult) {
	defer s.recoverAction(ctx, "MinecraftServerService.Restore", &result)
	if err := s.manager.Restore(ctx, model.ID(id)); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// HardDeleteRegistration removes only metadata after exact Name and Path confirmation.
func (s *MinecraftServerService) HardDeleteRegistration(ctx context.Context, id, confirmedName, confirmedPath string) (result ActionResult) {
	defer s.recoverAction(ctx, "MinecraftServerService.HardDeleteRegistration", &result)
	if err := s.manager.HardDeleteRegistration(ctx, model.ID(id), confirmedName, confirmedPath); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

// HardDeleteRemote starts permanent remote directory and registration deletion after exact confirmation.
func (s *MinecraftServerService) HardDeleteRemote(ctx context.Context, id, confirmedName, confirmedPath string) (result MinecraftServerOperationResult) {
	defer s.recoverOperation(ctx, "MinecraftServerService.HardDeleteRemote", &result)
	operationID, err := s.manager.StartHardDeleteRemote(ctx, model.ID(id), confirmedName, confirmedPath)
	if err != nil {
		dto := apperror.ToDTO(err)
		return MinecraftServerOperationResult{Error: &dto}
	}
	return MinecraftServerOperationResult{OperationID: operationID.String()}
}

// ListBackups returns MineOps-managed remote archives for one Server.
func (s *MinecraftServerService) ListBackups(ctx context.Context, id string) (result ServerBackupListResult) {
	defer s.recoverBackupList(ctx, &result)
	backups, err := s.manager.ListBackups(ctx, model.ID(id))
	if err != nil {
		dto := apperror.ToDTO(err)
		return ServerBackupListResult{Error: &dto}
	}
	return ServerBackupListResult{Backups: backups}
}

// CreateBackup starts a managed remote tar.gz backup Operation.
func (s *MinecraftServerService) CreateBackup(ctx context.Context, id string) (result MinecraftServerOperationResult) {
	defer s.recoverOperation(ctx, "MinecraftServerService.CreateBackup", &result)
	operationID, err := s.manager.StartBackup(ctx, model.ID(id))
	if err != nil {
		dto := apperror.ToDTO(err)
		return MinecraftServerOperationResult{Error: &dto}
	}
	return MinecraftServerOperationResult{OperationID: operationID.String()}
}

// RestoreBackup starts safe validation and atomic Server directory restoration.
func (s *MinecraftServerService) RestoreBackup(ctx context.Context, id, backupPath string) (result MinecraftServerOperationResult) {
	defer s.recoverOperation(ctx, "MinecraftServerService.RestoreBackup", &result)
	operationID, err := s.manager.StartRestoreBackup(ctx, model.ID(id), backupPath)
	if err != nil {
		dto := apperror.ToDTO(err)
		return MinecraftServerOperationResult{Error: &dto}
	}
	return MinecraftServerOperationResult{OperationID: operationID.String()}
}

// ReadProperties returns raw and structured server.properties state.
func (s *MinecraftServerService) ReadProperties(ctx context.Context, id string) (result ServerPropertiesResult) {
	defer s.recoverProperties(ctx, "MinecraftServerService.ReadProperties", &result)
	properties, err := s.manager.ReadProperties(ctx, model.ID(id))
	if err != nil {
		dto := apperror.ToDTO(err)
		return ServerPropertiesResult{Error: &dto}
	}
	return ServerPropertiesResult{Properties: properties}
}

// SaveProperties saves raw content or structured updates with conflict detection and retained backups.
func (s *MinecraftServerService) SaveProperties(ctx context.Context, id, mode, rawContent, expectedVersion string, updates []service.ServerPropertyUpdate, firewallConfirmed bool) (result ServerPropertiesResult) {
	defer s.recoverProperties(ctx, "MinecraftServerService.SaveProperties", &result)
	properties, err := s.manager.SaveProperties(ctx, model.ID(id), mode, rawContent, expectedVersion, updates, firewallConfirmed)
	if err != nil {
		dto := apperror.ToDTO(err)
		return ServerPropertiesResult{Error: &dto}
	}
	return ServerPropertiesResult{Properties: properties}
}

// ListPropertyBackups returns retained pre-save server.properties revisions.
func (s *MinecraftServerService) ListPropertyBackups(ctx context.Context, id string) (result ServerPropertiesBackupListResult) {
	defer s.recoverPropertyBackups(ctx, &result)
	backups, err := s.manager.ListPropertyBackups(ctx, model.ID(id))
	if err != nil {
		dto := apperror.ToDTO(err)
		return ServerPropertiesBackupListResult{Error: &dto}
	}
	return ServerPropertiesBackupListResult{Backups: backups}
}

// RestorePropertyBackup restores one retained revision after checking the current version token.
func (s *MinecraftServerService) RestorePropertyBackup(ctx context.Context, id, backupPath, expectedVersion string, firewallConfirmed bool) (result ServerPropertiesResult) {
	defer s.recoverProperties(ctx, "MinecraftServerService.RestorePropertyBackup", &result)
	properties, err := s.manager.RestorePropertyBackup(ctx, model.ID(id), backupPath, expectedVersion, firewallConfirmed)
	if err != nil {
		dto := apperror.ToDTO(err)
		return ServerPropertiesResult{Error: &dto}
	}
	return ServerPropertiesResult{Properties: properties}
}

// InspectInstallationStatus compares remote files with Server metadata and the latest Installation checkpoints.
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
