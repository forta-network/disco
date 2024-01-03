package drivers

import (
	"context"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/forta-network/disco/drivers/multidriver"
)

// Copy copies from src to dst.
func Copy(ctx context.Context, driver storagedriver.StorageDriver, src, dst string) (storagedriver.FileInfo, error) {
	return multidriver.Replicate(ctx, driver, driver, src, dst, true)
}
