package ipfsclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultClient(t *testing.T) {
	r := require.New(t)

	client := NewClient("http://foo.bar")
	api, err := client.GetClientFor(context.Background(), "")
	r.NoError(err)
	r.Equal(&client.Shell, api)
}
