package multidriver

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"path"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/forta-network/disco/drivers/filewriter"
	log "github.com/sirupsen/logrus"
)

// MultiDriver combines and deals with multiple drivers.
type MultiDriver interface {
	ReplicateInPrimary(contentPath string) (storagedriver.FileInfo, error)
	ReplicateInSecondary(contentPath string) (storagedriver.FileInfo, error)
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
func New(redirectTo *url.URL, primary storagedriver.StorageDriver, secondary storagedriver.StorageDriver) storagedriver.StorageDriver {
	return &driver{redirectTo: redirectTo, primary: primary, secondary: secondary}
}

// Is checks if the argument is a multi-driver implementation.
func Is(driver interface{}) (MultiDriver, bool) {
	d, ok := driver.(MultiDriver)
	return d, ok
}

// Name returns the name of the driver by implementing storagedriver.Storagedriver.
func (d *driver) Name() string {
	return fmt.Sprintf("%s+%s", d.primary.Name(), d.secondary.Name())
}

// ReplicateInPrimary ensures that a specific piece of content is replicated from the secondary
// store to the primary.
func (d *driver) ReplicateInPrimary(contentPath string) (storagedriver.FileInfo, error) {
	ctx := context.Background() // should not be cancellable
	_, err := Replicate(ctx, d.secondary, d.primary, contentPath, contentPath, false)
	if err != nil {
		return nil, err
	}
	s, err := d.primary.Stat(ctx, contentPath)
	return s, err
}

// ReplicateInSecondary ensures that a specific piece of content is replicated from the primary
// store to the secondary.
func (d *driver) ReplicateInSecondary(contentPath string) (storagedriver.FileInfo, error) {
	ctx := context.Background() // should not be cancellable
	_, err := Replicate(ctx, d.primary, d.secondary, contentPath, contentPath, false)
	if err != nil {
		return nil, err
	}
	s, err := d.secondary.Stat(ctx, contentPath)
	return s, err
}

// Replicate replicates from driver 1 to driver 2.
func Replicate(ctx context.Context, d1, d2 storagedriver.StorageDriver, src, dst string, mergeTree bool) (storagedriver.FileInfo, error) {
	if !mergeTree {
		d2i, err := d2.Stat(ctx, dst)
		switch err.(type) {
		case nil:
			return d2i, nil // already exists in second - exit recursion
		case storagedriver.PathNotFoundError:
			// does not exist in second - continue and copy to second
		default:
			return nil, fmt.Errorf("failed to check in '%s' before replication: %v", d2.Name(), err)
		}
	}

	d1i, err := d1.Stat(ctx, src)
	switch err.(type) {
	case nil:
		// exists in first - continue and copy from first
	case storagedriver.PathNotFoundError:
		return nil, err
	default:
		return nil, fmt.Errorf("failed to check in '%s' before replication: %v", d1.Name(), err)
	}

	if !d1i.IsDir() {
		return nil, syncD1ToD2(ctx, d1, d2, src, dst)
	}

	srcBase := src
	dstBase := dst

	return nil, d1.Walk(ctx, src, func(fileInfo storagedriver.FileInfo) error {
		if fileInfo.IsDir() {
			return nil
		}
		srcPath := fileInfo.Path()
		// dst path can be constructed by rewriting the base path
		dstPath := strings.Replace(srcPath, srcBase, dstBase, 1)
		return syncD1ToD2(ctx, d1, d2, srcPath, dstPath)
	})
}

func syncD1ToD2(ctx context.Context, d1, d2 storagedriver.StorageDriver, src, dst string) error {
	d1r, err := d1.Reader(ctx, src, 0)
	if err != nil {
		return err
	}
	defer d1r.Close()

	d2w, err := d2.Writer(ctx, dst, false)
	if err != nil {
		return fmt.Errorf("failed to create the '%s' writer: %v", d2.Name(), err)
	}
	defer d2w.Close()

	n, err := io.Copy(d2w, d1r)
	if err != nil {
		return fmt.Errorf("failed to copy from '%s' to '%s': %v", d1.Name(), d2.Name(), err)
	}
	if err := d2w.Commit(); err != nil {
		_ = d2w.Cancel()
		return fmt.Errorf("failed to commit '%s' writer: %v", d2.Name(), err)
	}
	log.WithFields(log.Fields{
		"bytes":   n,
		"src":     src,
		"dst":     dst,
		"driver1": d1.Name(),
		"driver2": d2.Name(),
	}).Debug("finished copying to the second driver")

	return nil
}

// GetContent retrieves the content stored at "path" as a []byte.
// This should primarily be used for small objects.
func (d *driver) GetContent(ctx context.Context, path string) ([]byte, error) {
	if _, err := d.ReplicateInSecondary(path); err != nil {
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
	if _, err := d.ReplicateInSecondary(path); err != nil {
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
	secStat, err := d.ReplicateInSecondary(path)
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
	if _, err := d.ReplicateInSecondary(path); err != nil {
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

	method, ok := options["method"]
	if ok {
		methodString, ok := method.(string)
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
