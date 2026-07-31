package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/applog"
	"github.com/Cail-Gainey/MineOps/internal/global/appsettings"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/service"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// FileResult 承载目录、文本文档或稳定的桌面错误。
type FileResult struct {
	Directory *service.RemoteDirectory  `json:"directory,omitempty"`
	Document  *model.RemoteTextDocument `json:"document,omitempty"`
	Error     *apperror.DTO             `json:"error,omitempty"`
}

// FileTransferResult 承载已启动任务的标识或稳定的桌面错误。
type FileTransferResult struct {
	OperationID    string        `json:"operationID,omitempty"`
	SelectionCount int           `json:"selectionCount,omitempty"`
	Cancelled      bool          `json:"cancelled,omitempty"`
	Error          *apperror.DTO `json:"error,omitempty"`
}

// FileDropEvent 汇报一次原生文件拖放是否启动了上传任务。
type FileDropEvent struct {
	OperationID string        `json:"operationID,omitempty"`
	FileCount   int           `json:"fileCount"`
	Error       *apperror.DTO `json:"error,omitempty"`
}

// FileService 基于已有 SSH Session 暴露有界的远端 Linux 文件操作。
type FileService struct {
	manager  *service.FileManager
	settings *appsettings.Manager
	logger   *applog.Logger
	app      *application.App
	dataRoot string
}

// NewFileService 创建桌面侧的远端文件门面。
func NewFileService(manager *service.FileManager, settings *appsettings.Manager, logger *applog.Logger, dataRoot string) *FileService {
	return &FileService{manager: manager, settings: settings, logger: logger, dataRoot: dataRoot}
}

// SetApplication 挂接用于原生上传下载路径选择的 Wails 应用实例。
func (s *FileService) SetApplication(app *application.App) {
	s.app = app
}

// List 返回一个规范化后的远端目录。
func (s *FileService) List(ctx context.Context, sshSessionID, path, currentDirectory string) (result FileResult) {
	defer s.recover(ctx, "FileService.List", &result)
	directory, err := s.manager.List(ctx, model.ID(sshSessionID), path, currentDirectory)
	if err != nil {
		return fileError(err)
	}
	return FileResult{Directory: &directory}
}

// ReadText 返回一份有界的 UTF-8 远端文档。
func (s *FileService) ReadText(ctx context.Context, sshSessionID, path, currentDirectory string) (result FileResult) {
	defer s.recover(ctx, "FileService.ReadText", &result)
	document, err := s.manager.ReadText(ctx, model.ID(sshSessionID), path, currentDirectory)
	if err != nil {
		return fileError(err)
	}
	return FileResult{Document: document}
}

// SaveText 做冲突检测并原子替换一份已存在的远端文档。
func (s *FileService) SaveText(ctx context.Context, sshSessionID, path, currentDirectory, content, expectedVersion string) (result FileResult) {
	defer s.recover(ctx, "FileService.SaveText", &result)
	document, err := s.manager.SaveText(ctx, model.ID(sshSessionID), path, currentDirectory, content, expectedVersion)
	if err != nil {
		return fileError(err)
	}
	return FileResult{Document: document}
}

// SaveTextAs 原子创建一份新的远端文档,不覆盖已存在的路径。
func (s *FileService) SaveTextAs(ctx context.Context, sshSessionID, path, currentDirectory, content string) (result FileResult) {
	defer s.recover(ctx, "FileService.SaveTextAs", &result)
	document, err := s.manager.SaveTextAs(ctx, model.ID(sshSessionID), path, currentDirectory, content)
	if err != nil {
		return fileError(err)
	}
	return FileResult{Document: document}
}

// CreateDirectory 创建一个远端目录。
func (s *FileService) CreateDirectory(ctx context.Context, sshSessionID, path, currentDirectory string) ActionResult {
	return s.action(ctx, "FileService.CreateDirectory", func() error { return s.manager.CreateDirectory(ctx, model.ID(sshSessionID), path, currentDirectory) })
}

// CreateFile 创建一个空的远端文件。
func (s *FileService) CreateFile(ctx context.Context, sshSessionID, path, currentDirectory string) ActionResult {
	return s.action(ctx, "FileService.CreateFile", func() error { return s.manager.CreateFile(ctx, model.ID(sshSessionID), path, currentDirectory) })
}

// Rename 移动一个远端条目,不覆盖已存在的目标。
func (s *FileService) Rename(ctx context.Context, sshSessionID, sourcePath, targetPath, currentDirectory string) ActionResult {
	return s.action(ctx, "FileService.Rename", func() error {
		return s.manager.Rename(ctx, model.ID(sshSessionID), sourcePath, targetPath, currentDirectory)
	})
}

// Delete 在危险路径检查通过后删除一个远端条目。
func (s *FileService) Delete(ctx context.Context, sshSessionID, path, currentDirectory string, recursive bool) ActionResult {
	return s.action(ctx, "FileService.Delete", func() error { return s.manager.Delete(ctx, model.ID(sshSessionID), path, currentDirectory, recursive) })
}

// Chmod 对远端条目应用一个已校验的八进制权限。
func (s *FileService) Chmod(ctx context.Context, sshSessionID, path, currentDirectory, mode string) ActionResult {
	return s.action(ctx, "FileService.Chmod", func() error { return s.manager.Chmod(ctx, model.ID(sshSessionID), path, currentDirectory, mode) })
}

// PickAndUpload 打开原生多选文件框并启动一次 SSH 标准输入上传任务。
func (s *FileService) PickAndUpload(ctx context.Context, sshSessionID, remoteDirectory, currentDirectory string) (result FileTransferResult) {
	defer s.recoverTransfer(ctx, "FileService.PickAndUpload", &result)
	if s.app == nil || s.app.Dialog == nil || s.settings == nil {
		return transferError(apperror.New(apperror.CodeInternal, "原生文件选择器尚未初始化"))
	}
	directory, err := s.localInitialDirectory(s.settings.Snapshot().Paths.ServersDirectory)
	if err != nil {
		return transferError(err)
	}
	selected, err := s.app.Dialog.OpenFile().
		SetTitle("选择要上传到 MineOps 远程目录的文件").
		SetDirectory(directory).
		CanChooseFiles(true).
		CanChooseDirectories(false).
		PromptForMultipleSelection()
	if err != nil {
		return transferError(apperror.Wrap(apperror.CodeIOReadFailed, "打开本地上传文件选择器失败", err))
	}
	if len(selected) == 0 {
		return FileTransferResult{Cancelled: true}
	}
	operationID, err := s.manager.StartUpload(ctx, model.ID(sshSessionID), selected, remoteDirectory, currentDirectory)
	if err != nil {
		return transferError(err)
	}
	return FileTransferResult{OperationID: operationID.String(), SelectionCount: len(selected)}
}

// PickAndDownload 打开原生保存框并启动一次 SSH 标准输出下载任务。
func (s *FileService) PickAndDownload(ctx context.Context, sshSessionID, remotePath, currentDirectory string) (result FileTransferResult) {
	defer s.recoverTransfer(ctx, "FileService.PickAndDownload", &result)
	if s.app == nil || s.app.Dialog == nil || s.settings == nil {
		return transferError(apperror.New(apperror.CodeInternal, "原生文件选择器尚未初始化"))
	}
	directory, err := s.localInitialDirectory(s.settings.Snapshot().Paths.DownloadsDirectory)
	if err != nil {
		return transferError(err)
	}
	destination, err := s.app.Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
		Title: "保存 MineOps 远程文件", Directory: directory, Filename: filepath.Base(remotePath), CanCreateDirectories: true,
	}).PromptForSingleSelection()
	if err != nil {
		return transferError(apperror.Wrap(apperror.CodeIOWriteFailed, "打开本地下载目标选择器失败", err))
	}
	if destination == "" {
		return FileTransferResult{Cancelled: true}
	}
	operationID, err := s.manager.StartDownload(ctx, model.ID(sshSessionID), remotePath, currentDirectory, destination)
	if err != nil {
		return transferError(err)
	}
	return FileTransferResult{OperationID: operationID.String(), SelectionCount: 1}
}

func (s *FileService) localInitialDirectory(configured string) (string, error) {
	if s.settings == nil || strings.TrimSpace(s.dataRoot) == "" {
		return "", apperror.New(apperror.CodeInternal, "本地默认目录服务尚未初始化")
	}
	cleaned := filepath.Clean(strings.TrimSpace(configured))
	if cleaned == "." || filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", apperror.New(apperror.CodeValidationInvalidArgument, "本地默认目录必须是受控相对路径")
	}
	directory := filepath.Join(s.dataRoot, cleaned)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", apperror.Wrap(apperror.CodeIOWriteFailed, "创建本地默认目录失败", err)
	}
	return directory, nil
}

// ExtractZIP 启动一次安全解压,把远端 ZIP 解到新的远端目录。
func (s *FileService) ExtractZIP(ctx context.Context, sshSessionID, archivePath, destinationPath, currentDirectory string) (result FileTransferResult) {
	defer s.recoverTransfer(ctx, "FileService.ExtractZIP", &result)
	operationID, err := s.manager.StartExtractZIP(ctx, model.ID(sshSessionID), archivePath, destinationPath, currentDirectory)
	if err != nil {
		return transferError(err)
	}
	return FileTransferResult{OperationID: operationID.String(), SelectionCount: 1}
}

func (s *FileService) action(ctx context.Context, boundary string, action func() error) (result ActionResult) {
	defer func() {
		var err error
		apperror.Recover(ctx, s.logger, boundary, &err)
		if err != nil {
			dto := apperror.ToDTO(err)
			result.Error = &dto
		}
	}()
	if err := action(); err != nil {
		dto := apperror.ToDTO(err)
		return ActionResult{Error: &dto}
	}
	return ActionResult{}
}

func (s *FileService) recover(ctx context.Context, boundary string, result *FileResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func (s *FileService) recoverTransfer(ctx context.Context, boundary string, result *FileTransferResult) {
	var err error
	apperror.Recover(ctx, s.logger, boundary, &err)
	if err != nil {
		dto := apperror.ToDTO(err)
		result.Error = &dto
	}
}

func fileError(err error) FileResult {
	dto := apperror.ToDTO(err)
	return FileResult{Error: &dto}
}

func transferError(err error) FileTransferResult {
	dto := apperror.ToDTO(err)
	return FileTransferResult{Error: &dto}
}
