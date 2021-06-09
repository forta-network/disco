package ipfs

import (
	"bytes"
	"context"
	"fmt"

	ipfsapi "github.com/ipfs/go-ipfs-api"
)

type fileWriter struct {
	ctx       context.Context
	api       *ipfsapi.Shell
	path      string
	opts      []ipfsapi.FilesOpt
	buf       *bytes.Buffer
	size      int64
	closed    bool
	committed bool
	cancelled bool
}

func newFileWriter(ctx context.Context, api *ipfsapi.Shell, path string, opts []ipfsapi.FilesOpt, size int64) *fileWriter {
	return &fileWriter{
		ctx:  ctx,
		api:  api,
		path: path,
		opts: opts,
		buf:  new(bytes.Buffer),
		size: size,
	}
}

func (fw *fileWriter) flush() error {
	return fw.api.FilesWrite(fw.ctx, fw.path, fw.buf, fw.opts...)
}

func (fw *fileWriter) Write(p []byte) (int, error) {
	if fw.closed {
		return 0, fmt.Errorf("already closed")
	} else if fw.committed {
		return 0, fmt.Errorf("already committed")
	} else if fw.cancelled {
		return 0, fmt.Errorf("already cancelled")
	}
	n, err := fw.buf.Write(p)
	fw.size += int64(n)
	return n, err
}

func (fw *fileWriter) Size() int64 {
	return fw.size
}

func (fw *fileWriter) Close() error {
	if fw.closed {
		return fmt.Errorf("already closed")
	}

	if err := fw.flush(); err != nil {
		return err
	}

	fw.closed = true
	return nil
}

func (fw *fileWriter) Cancel() error {
	if fw.closed {
		return fmt.Errorf("already closed")
	}

	fw.cancelled = true
	fw.buf = nil
	return nil
}

func (fw *fileWriter) Commit() error {
	if fw.closed {
		return fmt.Errorf("already closed")
	} else if fw.committed {
		return fmt.Errorf("already committed")
	} else if fw.cancelled {
		return fmt.Errorf("already cancelled")
	}

	if err := fw.flush(); err != nil {
		return err
	}

	fw.committed = true
	return nil
}
