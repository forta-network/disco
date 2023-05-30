package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/kelseyhightower/envconfig"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type envVars struct {
	RegistryConfigurationPath string `envconfig:"registry_configuration_path" default:"./config/default-config.yaml"`
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
	Cache              configuration.Storage
	RedirectTo         *url.URL
)

var customConfig struct {
	Storage struct {
		IPFS struct {
			Router   RouterConfig          `yaml:"router"`
			Cache    configuration.Storage `yaml:"cache"`
			Redirect string                `yaml:"redirect"`
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
	log.WithField("config", Vars.RegistryConfigurationPath).Info("found configuration")

	DistributionConfig, err = configuration.Parse(file)
	if err != nil {
		return fmt.Errorf("error parsing %s: %v", Vars.RegistryConfigurationPath, err)
	}

	// Override/set IPFS config for the IPFS driver to consume.
	// Following the original naming convention in https://github.com/distribution/distribution.
	ipfsConfig := DistributionConfig.Storage["ipfs"]
	if ipfsConfig["router"] == nil {
		return errors.New("please specify 'router' in ipfs driver config")
	}
	if ipfsConfig["router"] != nil {
		file, _ = os.Open(Vars.RegistryConfigurationPath)
		defer file.Close()
		err = yaml.NewDecoder(file).Decode(&customConfig)
		if err != nil {
			return err
		}
		Router = customConfig.Storage.IPFS.Router
		Cache = customConfig.Storage.IPFS.Cache
		if len(customConfig.Storage.IPFS.Redirect) > 0 {
			RedirectTo, err = url.Parse(customConfig.Storage.IPFS.Redirect)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
