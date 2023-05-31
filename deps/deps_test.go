package deps

import (
	"testing"

	"github.com/forta-network/disco/config"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	r := require.New(t)

	config.Router.Nodes = []*config.Node{
		{
			URL: "http://ipfs.url:5001",
		},
	}
	client := initialize()

	r.NotNil(client)
}
