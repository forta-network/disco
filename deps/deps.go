package deps

import (
	"sync"

	"github.com/forta-network/disco/config"
	"github.com/forta-network/disco/proxy/services/interfaces"
	"github.com/forta-network/disco/proxy/services/ipfsclient"
	log "github.com/sirupsen/logrus"
)

var (
	client interfaces.IPFSClient

	once sync.Once
)

// Get checks the config and returns the service dependencies.
func Get() interfaces.IPFSClient {
	once.Do(func() {
		client = initialize()
	})
	return client
}

func initialize() interfaces.IPFSClient {
	ipfsURL, ok := config.DistributionConfig.Storage["ipfs"]["url"]
	if ok {
		return ipfsclient.NewClient(ipfsURL.(string))
	}
	if len(config.Router.Nodes) == 0 {
		panic("no routed nodes")
	}

	log.Info("running with ipfs router client")
	return ipfsclient.NewRouterClient(&config.Router)
}
