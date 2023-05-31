package ipfsclient

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRouter(t *testing.T) {
	r := require.New(t)

	router := NewRouter(2)

	repo := "/docker/registry/v2/repositories/aa"
	uploads := "/docker/registry/v2/uploads/ac"
	blobs := "/docker/registry/v2/blobs/sha256/aa/aa"

	id, n, err := router.RouteContent(repo)
	r.NoError(err)
	r.Equal(0, n)
	r.Equal("aa", id)

	id, n, err = router.RouteContent(uploads)
	r.NoError(err)
	r.Equal(1, n)
	r.Equal("ac", id)

	id, n, err = router.RouteContent(blobs)
	r.NoError(err)
	r.Equal(0, n)
	r.Equal("aa", id)
}
