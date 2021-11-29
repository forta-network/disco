package main

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/distribution/distribution/v3/registry"
	_ "github.com/forta-network/disco/auth/htpasswd"
	"github.com/forta-network/disco/config"
	_ "github.com/forta-network/disco/drivers/ipfs"
	"github.com/forta-network/disco/proxy"
)

func main() {
	if err := config.Init(); err != nil {
		log.WithError(err).Fatal("failed to initialize the config")
	}
	registry, err := registry.NewRegistry(context.Background(), config.DistributionConfig)
	if err != nil {
		log.WithError(err).Fatal("failed to initialize the registry")
	}
	go registry.ListenAndServe()
	if err := proxy.ListenAndServe(); err != nil {
		log.WithError(err).Warn("proxy stopped")
	}
}
