package ipfsclient

import (
	"context"
	"net/http"

	"github.com/forta-protocol/disco/proxy/services/interfaces"
	ipfsapi "github.com/ipfs/go-ipfs-api"
)

// Client is just the default client that implements the interface.
type Client struct {
	ipfsapi.Shell
}

// NewClient creates a new client.
func NewClient(apiURL string) *Client {
	return &Client{*ipfsapi.NewShellWithClient(apiURL, http.DefaultClient)}
}

// GetClientFor returns the single client that is being used.
func (client *Client) GetClientFor(ctx context.Context, path string) (interfaces.IPFSFilesAPI, error) {
	return &client.Shell, nil
}
