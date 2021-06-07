package main

import (
	"context"
	"log"

	"github.com/distribution/distribution/v3/registry"

	"github.com/OpenZeppelin/disco/config"
	_ "github.com/OpenZeppelin/disco/drivers/ipfs"
)

func main() {
	registry, err := registry.NewRegistry(context.Background(), config.DistributionConfig)
	if err != nil {
		log.Panicf("failed to initialize the registry: %v", err)
	}
	registry.ListenAndServe()
}
