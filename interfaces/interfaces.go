package interfaces

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
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

// R2Client makes requests to an R2 API.
type R2Client interface {
	manager.DeleteObjectsAPIClient
	manager.UploadAPIClient
	manager.DownloadAPIClient
	manager.ListObjectsV2APIClient
	s3.ListMultipartUploadsAPIClient
	s3.ListPartsAPIClient
	s3.HeadObjectAPIClient
	CopyObject(ctx context.Context, params *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
	UploadPartCopy(ctx context.Context, params *s3.UploadPartCopyInput, optFns ...func(*s3.Options)) (*s3.UploadPartCopyOutput, error)
}

// StorageDriver is storage driver interface.
type StorageDriver interface {
	storagedriver.StorageDriver
}
