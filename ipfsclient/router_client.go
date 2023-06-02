package ipfsclient

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/forta-network/disco/config"
	"github.com/forta-network/disco/interfaces"
	"github.com/forta-network/disco/utils"
	ipfsapi "github.com/ipfs/go-ipfs-api"
	log "github.com/sirupsen/logrus"
)

// RouterClient implements the client interface to route the requests to multiple
// IPFS nodes.
type RouterClient struct {
	router *Router
	nodes  []*ipfsNode
}

type ipfsNode struct {
	info   *config.Node
	client interfaces.IPFSFilesAPI
}

// NewRouterClient creates a new router client. Files client implementation
// methods look for a client for a specific content provider (node) at read operations in general.
func NewRouterClient(routerCfg *config.RouterConfig) *RouterClient {
	var ipfsNodes []*ipfsNode
	for _, node := range routerCfg.Nodes {
		ipfsNodes = append(ipfsNodes, &ipfsNode{
			info:   node,
			client: ipfsapi.NewShellWithClient(node.URL, http.DefaultClient),
		})
	}
	return &RouterClient{
		router: NewRouter(len(ipfsNodes)),
		nodes:  ipfsNodes,
	}
}

// GetClientFor returns a client for a node which given content path should point to.
func (client *RouterClient) GetClientFor(ctx context.Context, path string) (interfaces.IPFSFilesAPI, error) {
	log.Debugf("GetClientFor(%s)", path)

	id, index, err := client.router.RouteContent(path)
	if err != nil {
		return nil, err
	}
	node := client.nodes[index]
	log.WithFields(log.Fields{
		"mfsPath":           path,
		"originalContentId": id,
		"routedNodeIndex":   index,
	}).Debug("routed client")

	return node.client, err
}

// FilesRead implements the interface.
func (client *RouterClient) FilesRead(ctx context.Context, path string, options ...ipfsapi.FilesOpt) (io.ReadCloser, error) {
	log.Debugf("FilesRead(%s, ...)", path)
	c, err := client.GetClientFor(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.FilesRead(ctx, path, options...)
}

// FilesWrite implements the interface.
func (client *RouterClient) FilesWrite(ctx context.Context, path string, data io.Reader, options ...ipfsapi.FilesOpt) error {
	log.Debugf("FilesWrite(%s, _, ...)", path)
	c, err := client.GetClientFor(ctx, path)
	if err != nil {
		return err
	}
	return c.FilesWrite(ctx, path, data, options...)
}

// FilesRm implements the interface.
func (client *RouterClient) FilesRm(ctx context.Context, path string, force bool) error {
	log.Debugf("FilesRm(%s, %t)", path, force)
	c, err := client.GetClientFor(ctx, path)
	if err != nil {
		return err
	}
	return c.FilesRm(ctx, path, force)
}

// FilesCp implements the interface.
func (client *RouterClient) FilesCp(ctx context.Context, src string, dest string) error {
	log.Debugf("FilesCp(%s, %s)", src, dest)
	// find the IPFS path if this is an fs path
	if !utils.IsIPFSPath(src) {
		stat, err := client.FilesStat(ctx, src)
		if err != nil {
			return err
		}
		src = fmt.Sprintf("/ipfs/%s", stat.Hash)
	}
	// get dest node client and copy there
	c, err := client.GetClientFor(ctx, dest)
	if err != nil {
		return err
	}
	return c.FilesCp(ctx, src, dest)
}

// FilesStat implements the interface.
func (client *RouterClient) FilesStat(ctx context.Context, path string, options ...ipfsapi.FilesOpt) (*ipfsapi.FilesStatObject, error) {
	log.Debugf("FilesStat(%s, ...)", path)
	c, err := client.GetClientFor(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.FilesStat(ctx, path, options...)
}

// FilesMkdir implements the interface.
func (client *RouterClient) FilesMkdir(ctx context.Context, path string, options ...ipfsapi.FilesOpt) error {
	log.Debugf("FilesMkdir(%s, ...)", path)
	c, err := client.GetClientFor(ctx, path)
	if err != nil {
		return err
	}
	return c.FilesMkdir(ctx, path, options...)
}

// FilesLs implements the interface.
func (client *RouterClient) FilesLs(ctx context.Context, path string, options ...ipfsapi.FilesOpt) ([]*ipfsapi.MfsLsEntry, error) {
	log.Debugf("FilesLs(%s, ...)", path)
	c, err := client.GetClientFor(ctx, path)
	if err != nil {
		return nil, err
	}
	return c.FilesLs(ctx, path, options...)
}

// FilesMv implements the interface.
//
// This is a tricky one since we can't do as easily as in FilesCp() since the src and dest nodes can be same or
// different. If they are the same, we can continue by doing FilesMv(). If not, we need to
// copy from first to second and then remove from the first.
func (client *RouterClient) FilesMv(ctx context.Context, src string, dest string) error {
	log.Debugf("FilesMv(%s, %s)", src, dest)

	srcClient, err := client.GetClientFor(ctx, src)
	if err != nil {
		return err
	}
	destClient, err := client.GetClientFor(ctx, dest)
	if err != nil {
		return err
	}
	if srcClient == destClient { // compare the pointers in memory
		return srcClient.FilesMv(ctx, src, dest)
	}

	// multiplexing results in different nodes - clear dest, do cp to dest and rm from src
	_ = client.FilesRm(ctx, dest, true)
	if err := client.FilesCp(ctx, src, dest); err != nil {
		return fmt.Errorf("cp failed while doing mv alternative: %v", err)
	}
	return srcClient.FilesRm(ctx, src, true)
}
