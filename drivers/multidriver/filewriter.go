package multidriver

import (
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
)

type fileWriter struct {
	primary   storagedriver.FileWriter
	secondary storagedriver.FileWriter
}

func newMultiFileWriter(primary storagedriver.FileWriter, secondary storagedriver.FileWriter) *fileWriter {
	fw := &fileWriter{
		primary:   primary,
		secondary: secondary,
	}
	return fw
}

func (fw *fileWriter) Write(p []byte) (int, error) {
	n, errSec := fw.secondary.Write(p)
	_, errPri := fw.primary.Write(p)
	if errSec != nil {
		return n, errSec
	}
	if errPri != nil {
		return n, errPri
	}
	return n, nil
}

func (fw *fileWriter) Size() int64 {
	return fw.secondary.Size()
}

func (fw *fileWriter) Close() error {
	if err := fw.secondary.Close(); err != nil {
		return err
	}
	if err := fw.primary.Close(); err != nil {
		return err
	}
	return nil
}

func (fw *fileWriter) Cancel() error {
	if err := fw.secondary.Cancel(); err != nil {
		return err
	}
	if err := fw.primary.Cancel(); err != nil {
		return err
	}
	return nil
}

func (fw *fileWriter) Commit() error {
	if err := fw.secondary.Commit(); err != nil {
		return err
	}
	if err := fw.primary.Commit(); err != nil {
		return err
	}
	return nil
}
