package ipfs

import (
	"context"
	"io"
	"sync"

	"github.com/forta-protocol/disco/proxy/services/interfaces"
	ipfsapi "github.com/ipfs/go-ipfs-api"
	log "github.com/sirupsen/logrus"
)

type fileWriter struct {
	ctx  context.Context
	api  interfaces.IPFSClient
	path string
	opts []ipfsapi.FilesOpt
	pr   *io.PipeReader
	pw   *io.PipeWriter
	size int64

	err error
	mu  sync.Mutex
}

func newFileWriter(ctx context.Context, api interfaces.IPFSClient, path string, opts []ipfsapi.FilesOpt, size int64) *fileWriter {
	pr, pw := io.Pipe()

	fw := &fileWriter{
		ctx:  ctx,
		api:  api,
		path: path,
		opts: opts,
		pr:   pr,
		pw:   pw,
		size: size,
	}

	go func(fw *fileWriter) {
		fw.mu.Lock()
		fw.err = fw.api.FilesWrite(fw.ctx, fw.path, pr, fw.opts...)
		log.WithError(fw.err).WithField("path", fw.path).Debug("writer done")
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
	log.WithField("path", fw.path).Debug("(*fileWriter).Write")
	n, err := fw.pw.Write(p)
	fw.size += int64(n)
	return n, err
}

func (fw *fileWriter) Size() int64 {
	return fw.size
}

func (fw *fileWriter) Close() error {
	log.WithField("path", fw.path).Debug("(*fileWriter).Close")
	fw.pw.Close()
	return fw.getErr()
}

func (fw *fileWriter) Cancel() error {
	log.WithField("path", fw.path).Debug("(*fileWriter).Cancel")
	return fw.Close()
}

func (fw *fileWriter) Commit() error {
	log.WithField("path", fw.path).Debug("(*fileWriter).Commit")
	return fw.Close()
}
