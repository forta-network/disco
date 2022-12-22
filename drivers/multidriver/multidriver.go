package multidriver

import (
	"context"
	"fmt"
	"io"
	"net/url"

	"path"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/forta-network/disco/drivers/filewriter"
	log "github.com/sirupsen/logrus"
)

// MultiDriver combines and deals with multiple drivers.
type MultiDriver interface {
	Replicate(contentPath string) (storagedriver.FileInfo, error)
	storagedriver.StorageDriver
}

// driver is a storage driver implementation as a multi-driver.
// It writes to both destinations, fills primary if only found in secondary, prefers
// reading from primary.
type driver struct {
	redirectTo *url.URL
	primary    storagedriver.StorageDriver
	secondary  storagedriver.StorageDriver
}

// New creates a new multi-driver.
func New(redirectTo url.URL, primary storagedriver.StorageDriver, secondary storagedriver.StorageDriver) storagedriver.StorageDriver {
	return &driver{redirectTo: &redirectTo, primary: primary, secondary: secondary}
}

// Is checks if the argument is a multi-driver implementation.
func Is(driver interface{}) (MultiDriver, bool) {
	d, ok := driver.(MultiDriver)
	return d, ok
}

func (d *driver) Name() string {
	return fmt.Sprintf("%s+%s", d.primary.Name(), d.secondary.Name())
}

// Replicate ensures that a specific piece of content is replicated in both storages.
func (d *driver) Replicate(contentPath string) (storagedriver.FileInfo, error) {
	ctx := context.Background() // should not be cancellable
	_, err := d.replicate(ctx, d.primary, d.secondary, contentPath)
	if err != nil {
		return nil, err
	}
	_, err = d.replicate(ctx, d.secondary, d.primary, contentPath)
	if err != nil {
		return nil, err
	}
	s, err := d.secondary.Stat(ctx, contentPath)
	return s, err
}

// replicate replicates from d1 to d2.
func (d *driver) replicate(ctx context.Context, d1, d2 storagedriver.StorageDriver, contentPath string) (storagedriver.FileInfo, error) {
	d2i, err := d2.Stat(ctx, contentPath)
	switch err.(type) {
	case nil:
		return d2i, nil // already exists in second - exit recursion
	case storagedriver.PathNotFoundError:
		// does not exist in second - continue and copy to second
	default:
		return nil, fmt.Errorf("failed to check in '%s' before replication: %v", d2.Name(), err)
	}

	d1i, err := d1.Stat(ctx, contentPath)
	switch err.(type) {
	case nil:
		// exists in first - continue and copy from first
	case storagedriver.PathNotFoundError:
		return nil, err
	default:
		return nil, fmt.Errorf("failed to check in '%s' before replication: %v", d1.Name(), err)
	}

	if !d1i.IsDir() {
		return nil, d.syncD1ToD2(ctx, d1, d2, contentPath)
	}

	return nil, d1.Walk(ctx, contentPath, func(fileInfo storagedriver.FileInfo) error {
		if fileInfo.IsDir() {
			return nil
		}
		return d.syncD1ToD2(ctx, d1, d2, fileInfo.Path())
	})
}

func (d *driver) syncD1ToD2(ctx context.Context, d1, d2 storagedriver.StorageDriver, path string) error {
	d1r, err := d1.Reader(ctx, path, 0)
	if err != nil {
		return err
	}
	defer d1r.Close()

	d2w, err := d2.Writer(ctx, path, false)
	if err != nil {
		return fmt.Errorf("failed to create the '%s' writer: %v", d2.Name(), err)
	}
	defer d2w.Close()

	n, err := io.Copy(d2w, d1r)
	if err != nil {
		return fmt.Errorf("failed to copy from '%s' to '%s': %v", d1.Name(), d2.Name(), err)
	}
	if err := d2w.Commit(); err != nil {
		d2w.Cancel()
		return fmt.Errorf("failed to commit '%s' writer: %v", d2.Name(), err)
	}
	log.WithFields(log.Fields{
		"bytes":   n,
		"path":    path,
		"driver1": d1.Name(),
		"driver2": d2.Name(),
	}).Info("finished copying to the second driver")

	return nil
}

// GetContent retrieves the content stored at "path" as a []byte.
// This should primarily be used for small objects.
func (d *driver) GetContent(ctx context.Context, path string) ([]byte, error) {
	if _, err := d.Replicate(path); err != nil {
		return nil, err
	}
	return d.secondary.GetContent(ctx, path)
}

// PutContent stores the []byte content at a location designated by "path".
// This should primarily be used for small objects.
func (d *driver) PutContent(ctx context.Context, path string, content []byte) error {
	if err := d.primary.PutContent(ctx, path, content); err != nil {
		return fmt.Errorf("PutContent() primary: %v", err)
	}
	if err := d.secondary.PutContent(ctx, path, content); err != nil {
		return fmt.Errorf("PutContent() secondary: %v", err)
	}
	return nil
}

// Reader retrieves an io.ReadCloser for the content stored at "path"
// with a given byte offset.
// May be used to resume reading a stream by providing a nonzero offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	if _, err := d.Replicate(path); err != nil {
		return nil, err
	}
	return d.secondary.Reader(ctx, path, offset)
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (d *driver) Writer(ctx context.Context, path string, append bool) (storagedriver.FileWriter, error) {
	priWriter, err := d.primary.Writer(ctx, path, append)
	if err != nil {
		return nil, fmt.Errorf("Writer() primary: %v", err)
	}
	secWriter, err := d.secondary.Writer(ctx, path, append)
	if err != nil {
		return nil, fmt.Errorf("Writer() secondary: %v", err)
	}
	return newMultiFileWriter(
		filewriter.WithLogger(d.primary.Name(), path, priWriter),
		filewriter.WithLogger(d.secondary.Name(), path, secWriter),
	), nil
}

// Stat retrieves the FileInfo for the given path, including the current
// size in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	secStat, err := d.Replicate(path)
	if err != nil {
		return nil, err
	}
	if secStat != nil {
		return secStat, nil
	}
	secStat, err = d.secondary.Stat(ctx, path)
	return secStat, err
}

// List returns a list of the objects that are direct descendants of the
// given path.
func (d *driver) List(ctx context.Context, path string) ([]string, error) {
	if _, err := d.Replicate(path); err != nil {
		return nil, err
	}
	return d.secondary.List(ctx, path)
}

// Move moves an object stored at sourcePath to destPath, removing the
// original object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	// do not replicate - we don't expect `Move()`s before any writes, which already ensure replication
	if err := d.primary.Move(ctx, sourcePath, destPath); err != nil {
		return fmt.Errorf("Move() primary: %v", err)
	}
	if err := d.secondary.Move(ctx, sourcePath, destPath); err != nil {
		return fmt.Errorf("Move() secondary: %v", err)
	}
	return nil
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
	// no need to replicate - just deleting anyways
	if err := d.primary.Delete(ctx, path); err != nil {
		return fmt.Errorf("Delete() primary: %v", err)
	}
	if err := d.secondary.Delete(ctx, path); err != nil {
		return fmt.Errorf("Delete() secondary: %v", err)
	}
	return nil
}

// URLFor returns a URL which may be used to retrieve the content stored at
// the given path, possibly using the given options.
// May return an ErrUnsupportedMethod in certain StorageDriver
// implementations.
func (d *driver) URLFor(ctx context.Context, contentPath string, options map[string]interface{}) (string, error) {
	if d.redirectTo == nil {
		return "", storagedriver.ErrUnsupportedMethod{}
	}

	methodString := "GET"
	method, ok := options["method"]
	if ok {
		methodString, ok = method.(string)
		if !ok || (methodString != "GET" && methodString != "HEAD") {
			return "", storagedriver.ErrUnsupportedMethod{}
		}
	}

	redirectURL := *d.redirectTo
	redirectURL.Path = path.Join(redirectURL.Path, contentPath)
	log.WithField("redirectUrl", redirectURL.String()).Info("created redirect url")
	return redirectURL.String(), nil
}

// Walk traverses a filesystem defined within driver, starting
// from the given path, calling f on each file.
// If the returned error from the WalkFn is ErrSkipDir and fileInfo refers
// to a directory, the directory will not be entered and Walk
// will continue the traversal. If fileInfo refers to a normal file, processing stops
func (d *driver) Walk(ctx context.Context, path string, f storagedriver.WalkFn) error {
	if err := d.primary.Walk(ctx, path, f); err != nil {
		return fmt.Errorf("Walk() primary: %v", err)
	}
	if err := d.secondary.Walk(ctx, path, f); err != nil {
		return fmt.Errorf("Walk() secondary: %v", err)
	}
	return nil
}
