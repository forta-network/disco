package main

import (
	"context"
	"log"

	"github.com/OpenZeppelin/disco/config"
	_ "github.com/OpenZeppelin/disco/drivers/ipfs"
	"github.com/OpenZeppelin/disco/proxy"
	"github.com/distribution/distribution/v3/registry"
	_ "github.com/distribution/distribution/v3/registry/auth/htpasswd"
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
