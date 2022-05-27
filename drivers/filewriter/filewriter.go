package filewriter

import (
	"context"
	"io"
	"sync"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	log "github.com/sirupsen/logrus"
)

type fileWriter struct {
	ctx        context.Context
	driverName string
	path       string
	pr         *io.PipeReader
	pw         *io.PipeWriter
	size       int64

	err error
	mu  sync.Mutex
}

// WriteFunc abstracts away the writer method.
type WriteFunc func(ctx context.Context, path string, reader io.Reader) error

// NewFileWriter creates a new file writer.
func NewFileWriter(ctx context.Context, driverName string, writeFunc WriteFunc, path string, size int64) *fileWriter {
	pr, pw := io.Pipe()

	fw := &fileWriter{
		ctx:  ctx,
		path: path,
		pr:   pr,
		pw:   pw,
		size: size,
	}

	go func(fw *fileWriter) {
		fw.mu.Lock()
		fw.err = writeFunc(ctx, path, pr)
		log.WithField("driver", driverName).WithError(fw.err).Debug("writer done")
		fw.mu.Unlock()
	}(fw)

	return fw
}

func (fw *fileWriter) getErr() error {
	fw.mu.Lock()
	err := fw.err
	fw.mu.Unlock()
	return err
}

func (fw *fileWriter) Write(p []byte) (int, error) {
	n, err := fw.pw.Write(p)
	fw.size += int64(n)
	return n, err
}

func (fw *fileWriter) Size() int64 {
	return fw.size
}

func (fw *fileWriter) Close() error {
	fw.pw.Close()
	return fw.getErr()
}

func (fw *fileWriter) Cancel() error {
	return fw.Close()
}

func (fw *fileWriter) Commit() error {
	return fw.Close()
}

type loggerWriter struct {
	name string
	path string
	fw   storagedriver.FileWriter
}

// WithLogger wraps given driver with a logger.
func WithLogger(name, path string, fw storagedriver.FileWriter) storagedriver.FileWriter {
	return &loggerWriter{name: name, path: path, fw: fw}
}

func (lw *loggerWriter) logger() *log.Entry {
	return log.WithFields(log.Fields{
		"driver": lw.name,
		"path":   lw.path,
	})
}

func (lw *loggerWriter) Write(p []byte) (int, error) {
	n, err := lw.fw.Write(p)
	lw.logger().WithFields(log.Fields{
		"wrote":   n,
		"newSize": lw.fw.Size(),
	}).Debug("(FileWriter).Write")
	return n, err
}

func (lw *loggerWriter) Size() int64 {
	size := lw.fw.Size()
	lw.logger().WithField("size", size).Debug("(FileWriter).Size")
	return size
}

func (lw *loggerWriter) Close() error {
	lw.logger().Debug("(FileWriter).Close")
	return lw.fw.Close()
}

func (lw *loggerWriter) Cancel() error {
	lw.logger().Debug("(FileWriter).Cancel")
	return lw.fw.Cancel()
}

func (lw *loggerWriter) Commit() error {
	lw.logger().Debug("(FileWriter).Commit")
	return lw.fw.Commit()
}
