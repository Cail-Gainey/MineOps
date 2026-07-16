// Package port defines infrastructure boundaries implemented by remote adapters.
package port

import (
	"context"
	"io"

	"github.com/Cail-Gainey/MineOps/internal/model"
)

// SFTPSession is one cancellable remote filesystem connection owned by an application service.
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

// SFTPFactory opens isolated SFTP Sessions over an existing SSH Session identity.
type SFTPFactory interface {
	Open(context.Context, model.ID) (SFTPSession, error)
}
