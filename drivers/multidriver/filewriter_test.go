package multidriver

import (
	"testing"

	"github.com/forta-network/disco/drivers/filewriter"
	"github.com/stretchr/testify/require"
)

func TestFileWriter(t *testing.T) {
	r := require.New(t)

	priW := &filewriter.StubWriter{}
	secW := &filewriter.StubWriter{}

	fw := newMultiFileWriter(priW, secW)

	n, err := fw.Write([]byte("1"))
	r.NoError(err)
	r.Equal(1, n)

	r.Equal(int64(1), fw.Size())
	r.NoError(fw.Commit())
	r.NoError(fw.Close())
	r.NoError(fw.Cancel())
}
