package config

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang/glog"
	consulapi "github.com/hashicorp/consul/api"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/apis/meta/v1"
)

// RegisterMode is a name of register mode
type RegisterMode string

// "RegisterSingleMode", "RegisterNodeMode" and "RegisterPodMode"
// defines correct value of `register_mode` option.
// "RegisterSingleMode" determine correct value for `single` mode.
// "RegisterNodeMode" determine correct value for `node` mode.
// "RegisterNodeMode" determine correct value for `pod` mode.
const (
	RegisterSingleMode RegisterMode = "single"
	RegisterNodeMode   RegisterMode = "node"
	RegisterPodMode    RegisterMode = "pod"
)

// Config describes the attributes that are uses to create configuration structure
type Config struct {
	Controller *ControllerConfig
	Consul     *consulapi.Config
}

// ControllerConfig describes the attributes for the controller configuration
type ControllerConfig struct {
	ConsulAddress            string
	ConsulPort               string
	ConsulScheme             string
	ConsulCAFile             string
	ConsulCertFile           string
	ConsulKeyFile            string
	ConsulInsecureSkipVerify bool
	ConsulToken              string
	ConsulTimeout            time.Duration
	ConsulContainerName      string
	K8sTag                   string
	RegisterMode             RegisterMode
}

var config = &Config{}

// Load function loads configuration from ConfigMap resource in Kubernetes cluster and fills
// the attributes of ControllerConfig struct
func Load(clientset *kubernetes.Clientset, namespace string, name string) (*Config, error) {
	var filledConfig *Config

	cfg, err := clientset.Core().ConfigMaps(namespace).Get(name, v1.GetOptions{})
	if err != nil {
		return config, fmt.Errorf(err.Error())
	}

	filledConfig, err = config.fillConfig(cfg.Data)
	if err != nil {
		return config, fmt.Errorf("Can't fill configuration: %s", err)
	}
	return filledConfig, nil
}

func (c *Config) fillConfig(data map[string]string) (*Config, error) {
	//Consul configuration
	c.Consul = consulapi.DefaultConfig()
	c.Controller = &ControllerConfig{}

	if value, ok := data["consul_address"]; ok && value != "" {
		c.Controller.ConsulAddress = value
	} else {
		c.Controller.ConsulAddress = "localhost"
	}

	if value, ok := data["consul_port"]; ok && value != "" {
		c.Controller.ConsulPort = value
	} else {
		c.Controller.ConsulPort = "8500"
	}

	if value, ok := data["consul_scheme"]; ok && value != "" {
		c.Controller.ConsulScheme = value
	} else {
		c.Controller.ConsulScheme = "http"
	}

	if value, ok := data["consul_ca_file"]; ok {
		c.Controller.ConsulCAFile = value
	}

	if value, ok := data["consul_cert_file"]; ok {
		c.Controller.ConsulCertFile = value
	}

	if value, ok := data["consul_key_file"]; ok {
		c.Controller.ConsulKeyFile = value
	}

	if value, ok := data["consul_insecure_skip_verify"]; ok && value != "" {
		v, err := strconv.ParseBool(value)
		if err != nil {
			return c, err
		}
		c.Controller.ConsulInsecureSkipVerify = v
	} else {
		c.Controller.ConsulInsecureSkipVerify = false
	}

	if value, ok := data["consul_token"]; ok && value != "" {
		c.Controller.ConsulToken = value
	}

	if value, ok := data["consul_timeout"]; ok && value != "" {
		timeout, err := time.ParseDuration(value)
		if err != nil {
			return c, err
		}
		c.Controller.ConsulTimeout = timeout
	} else {
		c.Controller.ConsulTimeout = 2 * time.Second
	}

	if value, ok := data["consul_container_name"]; ok && value != "" {
		c.Controller.ConsulContainerName = value
	} else {
		c.Controller.ConsulContainerName = "consul"
	}

	if value, ok := data["k8s_tag"]; ok && value != "" {
		c.Controller.K8sTag = value
	} else {
		c.Controller.K8sTag = "kubernetes"
	}

	if value, ok := data["register_mode"]; ok {
		switch value {
		case string(RegisterSingleMode):
			c.Controller.RegisterMode = RegisterSingleMode
		case string(RegisterNodeMode):
			c.Controller.RegisterMode = RegisterNodeMode
		case string(RegisterPodMode):
			c.Controller.RegisterMode = RegisterPodMode
		default:
			glog.Warning("Wrong value of 'register_mode' option. Permitted values: %s|%s|%s, is %s",
				RegisterSingleMode, RegisterNodeMode, RegisterPodMode, value)

			c.Controller.RegisterMode = RegisterSingleMode
		}
	} else {
		c.Controller.RegisterMode = RegisterSingleMode
	}

	return c, nil
}
