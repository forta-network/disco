package config

import (
	"log"
	"os"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/kelseyhightower/envconfig"
)

type envVars struct {
	RegistryConfigurationPath string `envconfig:"registry_configuration_path" default:"./config/default-config.yaml"`
	IPFSGatewayURL            string `envconfig:"ipfs_gateway_url"`
}

// Configuration variables
var (
	Vars               envVars
	DistributionConfig *configuration.Configuration
)

// Init parses and prepares all config variables.
func init() {
	envconfig.MustProcess("", &Vars)

	file, err := os.Open(Vars.RegistryConfigurationPath)
	if err != nil {
		log.Panicf("failed to open config file: %v", err)
	}
	defer file.Close()

	DistributionConfig, err = configuration.Parse(file)
	if err != nil {
		log.Panicf("error parsing %s: %v", Vars.RegistryConfigurationPath, err)
	}

	// Override/set IPFS config for the IPFS driver to consume.
	// Following the original naming convention in https://github.com/distribution/distribution.
	ipfsConfig := DistributionConfig.Storage["ipfs"]
	if len(Vars.IPFSGatewayURL) > 0 {
		ipfsConfig["url"] = Vars.IPFSGatewayURL
	}
}
