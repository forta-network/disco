package filewriter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStub(t *testing.T) {
	r := require.New(t)

	sw := &StubWriter{}

	n, err := sw.Write([]byte("1"))
	r.NoError(err)
	r.NoError(sw.Cancel())
	r.NoError(sw.Close())
	r.NoError(sw.Commit())
	r.Equal(1, n)
	r.Equal(int64(1), sw.Size())
}
