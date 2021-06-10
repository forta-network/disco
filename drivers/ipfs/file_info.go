package ipfs

import (
	"time"

	ipfsapi "github.com/ipfs/go-ipfs-api"
)

// fileInfo implements storagedriver.FileInfo.
type fileInfo struct {
	*ipfsapi.FilesStatObject
	path string
}

// Path provides the full path of the target of this file info.
func (fi *fileInfo) Path() string {
	return fi.path
}

// Size returns current length in bytes of the file. The return value can
// be used to write to the end of the file at path.
func (fi *fileInfo) Size() int64 {
	return int64(fi.FilesStatObject.Size)
}

// ModTime returns the modification time for the file. We return an arbitrary value
// for now since IPFS doesn't seem to support this.
func (fi *fileInfo) ModTime() time.Time {
	return time.Now()
}

// IsDir returns true if the path is a directory.
func (fi *fileInfo) IsDir() bool {
	return fi.Type == "directory"
}
