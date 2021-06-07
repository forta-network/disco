package ipfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/base"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
	httpapi "github.com/ipfs/go-ipfs-http-client"
)

const driverName = "ipfs"

func init() {
	factory.Register(driverName, &driverFactory{})
}

// driver is the concrete IPFS driver implementation.
type driver struct {
	gatewayURL string
	api        *httpapi.HttpApi
}

type driverFactory struct{}

func (df *driverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

// Driver is the exposed IPFS driver implementation.
type Driver struct {
	base.Base
}

// FromParameters constructs a new driver using given parameters.
// Required parameters:
// - gatewayurl
func FromParameters(parameters map[string]interface{}) (*Driver, error) {
	gatewayURL, ok := parameters["gatewayurl"]
	if !ok {
		return nil, errors.New("no gateway URL specified")
	}
	api, err := httpapi.NewURLApiWithClient(gatewayURL.(string), http.DefaultClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create the IPFS API client: %v", err)
	}
	return &Driver{
		Base: base.Base{
			StorageDriver: &driver{
				gatewayURL: gatewayURL.(string),
				api:        api,
			},
		},
	}, nil
}

// Implement the storagedriver.StorageDriver interface

func (d *driver) Name() string {
	return driverName
}

// GetContent retrieves the content stored at "path" as a []byte.
func (d *driver) GetContent(ctx context.Context, path string) ([]byte, error) {
	return nil, nil
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, contents []byte) error {
	return nil
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	return nil, nil
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (d *driver) Writer(ctx context.Context, path string, appendParam bool) (storagedriver.FileWriter, error) {
	return nil, nil
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	return nil, nil
}

// List returns a list of the objects that are direct descendants of the given path.
func (d *driver) List(ctx context.Context, opath string) ([]string, error) {
	return nil, nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	return nil
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
	return nil
}

// URLFor returns a URL which may be used to retrieve the content stored at the given path.
// May return an UnsupportedMethodErr in certain StorageDriver implementations.
func (d *driver) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	return "", nil
}

// Walk traverses a filesystem defined within driver, starting
// from the given path, calling f on each file
func (d *driver) Walk(ctx context.Context, from string, f storagedriver.WalkFn) error {
	return nil
}
