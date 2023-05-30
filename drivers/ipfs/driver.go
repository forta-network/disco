package ipfs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/distribution/distribution/v3/configuration"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/base"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
	"github.com/forta-network/disco/config"
	"github.com/forta-network/disco/deps"
	"github.com/forta-network/disco/drivers"
	"github.com/forta-network/disco/drivers/filewriter"
	"github.com/forta-network/disco/drivers/multidriver"
	"github.com/forta-network/disco/proxy/services/interfaces"
	ipfsapi "github.com/ipfs/go-ipfs-api"
)

const (
	driverName = "ipfs"
)

var (
	defaultFactory = &driverFactory{}
	defaultDriver  storagedriver.StorageDriver
)

func init() {
	factory.Register(driverName, defaultFactory)
}

// driver is the concrete IPFS driver implementation.
type driver struct {
	api interfaces.IPFSClient
}

type driverFactory struct{}

func (df *driverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	ipfsDriver, err := fromParameters(parameters)
	if err != nil {
		defaultDriver = ipfsDriver
		return nil, fmt.Errorf("failed to create ipfs driver: %v", err)
	}
	if config.Cache == nil {
		return ipfsDriver, nil
	}
	// create multidriver by using cache as secondary
	var (
		driverName   string
		driverParams configuration.Parameters
	)
	for dName, dParams := range config.Cache {
		driverName = dName
		driverParams = dParams
		break
	}
	cacheDriver, err := factory.Create(driverName, driverParams)
	if err != nil {
		return nil, fmt.Errorf("failed to create the cache driver (%s): %v", driverName, err)
	}
	defaultDriver, err = multidriver.New(config.RedirectTo, ipfsDriver, cacheDriver), nil
	return defaultDriver, err
}

// New creates a new IPFS-only driver.
func New(api interfaces.IPFSClient) storagedriver.StorageDriver {
	return &driver{
		api: api,
	}
}

// Get returns the already created default driver.
func Get() storagedriver.StorageDriver {
	return defaultDriver
}

// Create create creates a new driver instance from parameters.
func Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return defaultFactory.Create(parameters)
}

// Driver is the exposed IPFS driver implementation.
type Driver struct {
	base.Base
}

// fromParameters constructs a new driver using given parameters.
func fromParameters(parameters map[string]interface{}) (*Driver, error) {
	api := deps.Get()
	return &Driver{
		Base: base.Base{
			StorageDriver: &driver{
				api: api,
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
	path = drivers.FixUploadPath(path)
	readCloser, err := d.Reader(ctx, path, 0)
	if err != nil {
		return nil, err
	}
	defer readCloser.Close()
	return ioutil.ReadAll(readCloser)
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, contents []byte) error {
	path = drivers.FixUploadPath(path)
	return d.api.FilesWrite(ctx, path, bytes.NewBuffer(contents), ipfsapi.FilesWrite.Create(true), ipfsapi.FilesWrite.Parents(true))
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	path = drivers.FixUploadPath(path)
	reader, err := d.api.FilesRead(ctx, path, ipfsapi.FilesRead.Offset(offset))
	if err != nil && isNotFoundErr(err) {
		return nil, storagedriver.PathNotFoundError{Path: path, DriverName: driverName}
	}
	if err != nil {
		return nil, err
	}
	return reader, err
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (d *driver) Writer(ctx context.Context, path string, shouldAppend bool) (storagedriver.FileWriter, error) {
	path = drivers.FixUploadPath(path)
	fileOpts := []ipfsapi.FilesOpt{ipfsapi.FilesWrite.Create(true), ipfsapi.FilesWrite.Parents(true)}
	var offset int64
	if shouldAppend {
		stat, err := d.api.FilesStat(ctx, path, ipfsapi.FilesStat.Size(true))
		if err != nil && isNotFoundErr(err) {
			return nil, storagedriver.PathNotFoundError{Path: path, DriverName: driverName}
		}
		if err != nil {
			return nil, err
		}
		offset = int64(stat.Size)
		fileOpts = append(fileOpts, ipfsapi.FilesWrite.Offset(offset))
	}
	return filewriter.NewFileWriter(ctx, d.Name(), d.writeFunc(path, fileOpts), path, offset), nil
}

func (d *driver) writeFunc(path string, opts []ipfsapi.FilesOpt) filewriter.WriteFunc {
	return func(ctx context.Context, path string, r io.Reader) error {
		return d.api.FilesWrite(ctx, path, r, opts...)
	}
}

func isNotFoundErr(err error) bool {
	e, ok := err.(*ipfsapi.Error)
	if !ok {
		return false
	}
	return e.Code == 0
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	path = drivers.FixUploadPath(path)
	stat, err := d.api.FilesStat(ctx, path)
	if err != nil && isNotFoundErr(err) {
		return nil, storagedriver.PathNotFoundError{Path: path, DriverName: driverName}
	}
	if err != nil {
		return nil, err
	}
	return &fileInfo{FilesStatObject: stat, path: path}, nil
}

// List returns a list of the objects that are direct descendants of the given path.
func (d *driver) List(ctx context.Context, path string) ([]string, error) {
	path = drivers.FixUploadPath(path)
	results, err := d.api.FilesLs(ctx, path)
	if err != nil && isNotFoundErr(err) {
		return nil, storagedriver.PathNotFoundError{Path: path, DriverName: driverName}
	}
	if err != nil {
		return nil, err
	}
	var list []string
	for _, result := range results {
		list = append(list, path+"/"+result.Name)
	}
	return list, nil
}

// Move moves an object stored at sourcePath to destPath, removing the original object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	sourcePath = drivers.FixUploadPath(sourcePath)
	destPath = drivers.FixUploadPath(destPath)
	folderPath := destPath[:strings.LastIndex(destPath, "/")+1]
	if err := d.api.FilesMkdir(ctx, folderPath, ipfsapi.FilesMkdir.Parents(true)); err != nil {
		return err
	}
	return d.api.FilesMv(ctx, sourcePath, destPath)
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
	path = drivers.FixUploadPath(path)
	return d.api.FilesRm(ctx, path, true)
}

// URLFor returns a URL which may be used to retrieve the content stored at the given path.
// May return an UnsupportedMethodErr in certain StorageDriver implementations and we don't
// currently support this method, too.
func (d *driver) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	return "", storagedriver.ErrUnsupportedMethod{DriverName: driverName}
}

// Walk traverses a filesystem defined within driver, starting
// from the given path, calling f on each file.
func (d *driver) Walk(ctx context.Context, path string, f storagedriver.WalkFn) error {
	path = drivers.FixUploadPath(path)
	return storagedriver.WalkFallback(ctx, d, path, f)
}
