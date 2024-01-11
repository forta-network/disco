package drivers

import (
	"context"
	"strings"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/forta-network/disco/drivers/multidriver"
)

// FixUploadPath rewrites .../repository/<name>/_uploads to /disco-uploads to make things easier.
func FixUploadPath(path string) string {
	if !strings.Contains(path, "/_uploads") {
		return path
	}
	newPath := "/disco-uploads"
	var append bool
	// append the segments after "_uploads"
	for _, segment := range strings.Split(path, "/") {
		if append {
			newPath += "/" + segment
		}
		if segment == "_uploads" {
			append = true
		}
	}
	return newPath
}

// Copy copies from src to dst.
func Copy(ctx context.Context, driver storagedriver.StorageDriver, src, dst string) (storagedriver.FileInfo, error) {
	return multidriver.Replicate(ctx, driver, driver, src, dst, true)
}
