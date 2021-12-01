package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v3"
)

type envVars struct {
	RegistryConfigurationPath string `envconfig:"registry_configuration_path" default:"./config/default-config.yaml"`
	IPFSURL                   string `envconfig:"ipfs_url"`
	DiscoPort                 int    `envconfig:"disco_port" default:"1970"`
}

// Node contains IPFS node parameters.
type Node struct {
	URL string `yaml:"url"`
}

// RouterConfig contains router config parameters.
type RouterConfig struct {
	Nodes []*Node `yaml:"nodes"`
}

// Configuration variables
var (
	Vars               envVars
	DistributionConfig *configuration.Configuration
	Router             RouterConfig
)

var customConfig struct {
	Storage struct {
		IPFS struct {
			Router RouterConfig `yaml:"router"`
		} `yaml:"ipfs"`
	} `yaml:"storage"`
}

// Init parses and prepares all config variables.
func Init() error {
	envconfig.MustProcess("", &Vars)

	file, err := os.Open(Vars.RegistryConfigurationPath)
	if err != nil {
		return fmt.Errorf("failed to open config file: %v", err)
	}
	defer file.Close()

	DistributionConfig, err = configuration.Parse(file)
	if err != nil {
		return fmt.Errorf("error parsing %s: %v", Vars.RegistryConfigurationPath, err)
	}

	// Override/set IPFS config for the IPFS driver to consume.
	// Following the original naming convention in https://github.com/distribution/distribution.
	ipfsConfig := DistributionConfig.Storage["ipfs"]
	if ipfsConfig["url"] == nil && ipfsConfig["router"] == nil {
		return errors.New("please specify at least one of 'url' or 'router' in ipfs driver config")
	}
	if len(Vars.IPFSURL) > 0 {
		ipfsConfig["url"] = Vars.IPFSURL
	}
	if ipfsConfig["router"] != nil {
		file, _ = os.Open(Vars.RegistryConfigurationPath)
		defer file.Close()
		err = yaml.NewDecoder(file).Decode(&customConfig)
		if err != nil {
			return err
		}
		Router = customConfig.Storage.IPFS.Router
	}

	return nil
}
