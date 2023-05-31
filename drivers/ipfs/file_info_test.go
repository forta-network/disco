package ipfs

import (
	"testing"
	"time"

	shell "github.com/ipfs/go-ipfs-api"
	"github.com/stretchr/testify/require"
)

func TestFileIn(t *testing.T) {
	r := require.New(t)

	fi := &fileInfo{
		FilesStatObject: &shell.FilesStatObject{
			Size: 1,
			Type: "directory",
		},
		path: "foo",
	}

	r.Equal(int64(fi.FilesStatObject.Size), fi.Size())
	r.Equal("foo", fi.Path())
	r.True(fi.IsDir())
	r.Less(time.Since(fi.ModTime()), time.Hour)
}
