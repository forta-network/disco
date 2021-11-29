package interfaces

import (
	"context"
	"io"

	ipfsapi "github.com/ipfs/go-ipfs-api"
)

// IPFSClient makes requests to an IPFS node.
type IPFSClient interface {
	GetClientFor(ctx context.Context, path string) (IPFSFilesAPI, error)
	IPFSFilesAPI
}

// IPFSFilesAPI makes requests to an IPFS node.
type IPFSFilesAPI interface {
	FilesRead(ctx context.Context, path string, options ...ipfsapi.FilesOpt) (io.ReadCloser, error)
	FilesWrite(ctx context.Context, path string, data io.Reader, options ...ipfsapi.FilesOpt) error
	FilesRm(ctx context.Context, path string, force bool) error
	FilesCp(ctx context.Context, src string, dest string) error
	FilesStat(ctx context.Context, path string, options ...ipfsapi.FilesOpt) (*ipfsapi.FilesStatObject, error)
	FilesMkdir(ctx context.Context, path string, options ...ipfsapi.FilesOpt) error
	FilesLs(ctx context.Context, path string, options ...ipfsapi.FilesOpt) ([]*ipfsapi.MfsLsEntry, error)
	FilesMv(ctx context.Context, src string, dest string) error
}
