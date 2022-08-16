package main

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/distribution/distribution/v3/registry"

	// first init() the built-in drivers
	_ "github.com/distribution/distribution/v3/registry/auth/htpasswd"
	_ "github.com/distribution/distribution/v3/registry/auth/silly"
	_ "github.com/distribution/distribution/v3/registry/auth/token"
	_ "github.com/distribution/distribution/v3/registry/proxy"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/azure"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/gcs"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/middleware/alicdn"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/middleware/cloudfront"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/middleware/redirect"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/oss"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/s3-aws"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/swift"

	// then init() the custom drivers
	_ "github.com/forta-network/disco/drivers/ipfs"

	"github.com/forta-network/disco/config"
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
