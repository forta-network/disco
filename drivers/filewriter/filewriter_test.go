package filewriter

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileWriter(t *testing.T) {
	r := require.New(t)

	out := make([]byte, 1)

	var rc io.ReadCloser
	fileWriter := NewFileWriter(context.Background(), "", func(ctx context.Context, path string, reader io.Reader) error {
		r.Equal(rc, reader)
		n, err := reader.Read(out)
		r.NoError(err)
		r.Equal(1, n)
		return nil
	}, "", 0)
	rc = fileWriter.ReadCloser()

	fw := WithLogger("", "", fileWriter)

	n, err := fw.Write([]byte("1"))
	r.NoError(err)
	r.Equal(1, n)

	r.Equal(int64(1), fw.Size())
	r.NoError(fw.Commit())
	r.NoError(fw.Close())
	r.NoError(fw.Cancel())

	r.Equal([]byte("1"), out)
}
