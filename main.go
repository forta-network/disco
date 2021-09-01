package main

import (
	"context"
	"log"

	"github.com/distribution/distribution/v3/registry"
	_ "github.com/forta-network/disco/auth/htpasswd"
	"github.com/forta-network/disco/config"
	_ "github.com/forta-network/disco/drivers/ipfs"
	"github.com/forta-network/disco/proxy"
)

func main() {
	config.Init()
	registry, err := registry.NewRegistry(context.Background(), config.DistributionConfig)
	if err != nil {
		log.Panicf("failed to initialize the registry: %v", err)
	}
	go registry.ListenAndServe()
	if err := proxy.ListenAndServe(); err != nil {
		log.Printf("proxy stopped: %v", err)
	}
}
