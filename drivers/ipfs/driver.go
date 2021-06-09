package ipfs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/base"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
	ipfsapi "github.com/ipfs/go-ipfs-api"
)

const driverName = "ipfs"

func init() {
	factory.Register(driverName, &driverFactory{})
}

// driver is the concrete IPFS driver implementation.
type driver struct {
	ipfsURL string
	api     *ipfsapi.Shell
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
// - url
func FromParameters(parameters map[string]interface{}) (*Driver, error) {
	ipfsURL, ok := parameters["url"]
	if !ok {
		return nil, errors.New("no IPFS URL specified")
	}
	api := ipfsapi.NewShellWithClient(ipfsURL.(string), http.DefaultClient)
	return &Driver{
		Base: base.Base{
			StorageDriver: &driver{
				ipfsURL: ipfsURL.(string),
				api:     api,
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
	readCloser, err := d.Reader(ctx, path, 0)
	if err != nil {
		return nil, err
	}
	defer readCloser.Close()
	return ioutil.ReadAll(readCloser)
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, contents []byte) error {
	return d.api.FilesWrite(ctx, path, bytes.NewBuffer(contents), ipfsapi.FilesWrite.Create(true), ipfsapi.FilesWrite.Parents(true))
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
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
	return newFileWriter(ctx, d.api, path, fileOpts, offset), nil
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
	results, err := d.api.List(path)
	if err != nil && isNotFoundErr(err) {
		return nil, storagedriver.PathNotFoundError{Path: path, DriverName: driverName}
	}
	if err != nil {
		return nil, err
	}
	var list []string
	for _, result := range results {
		list = append(list, result.Name)
	}
	return list, nil
}

// Move moves an object stored at sourcePath to destPath, removing the original object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	folderPath := destPath[:strings.LastIndex(destPath, "/")+1]
	if err := d.api.FilesMkdir(ctx, folderPath, ipfsapi.FilesMkdir.Parents(true)); err != nil {
		return err
	}
	return d.api.FilesMv(ctx, sourcePath, destPath)
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
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
	return storagedriver.WalkFallback(ctx, d, path, f)
}
