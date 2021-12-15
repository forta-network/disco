package main

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/distribution/distribution/v3/registry"
	"github.com/forta-protocol/disco/config"
	_ "github.com/forta-protocol/disco/drivers/ipfs"
	"github.com/forta-protocol/disco/proxy"
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
