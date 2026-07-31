// Package port 定义由远端适配器实现的基础设施边界。
package port

import (
	"context"
	"io"

	"github.com/Cail-Gainey/MineOps/internal/model"
)

// SFTPSession 是由应用服务持有、可取消的一条远端文件系统连接。
type SFTPSession interface {
	List(context.Context, string) ([]model.RemoteFile, error)
	Stat(context.Context, string) (model.RemoteFile, error)
	ReadLink(context.Context, string) (string, error)
	OpenReader(context.Context, string) (io.ReadCloser, error)
	OpenWriter(context.Context, string, uint32) (io.WriteCloser, error)
	CreateDirectory(context.Context, string, uint32) error
	Rename(context.Context, string, string) error
	Remove(context.Context, string, bool) error
	Chmod(context.Context, string, uint32) error
	Close() error
}

// SFTPFactory 基于既有 SSH Session 身份打开相互隔离的 SFTP 会话。
type SFTPFactory interface {
	Open(context.Context, model.ID) (SFTPSession, error)
}
