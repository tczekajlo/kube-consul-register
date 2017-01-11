package consul

import (
	"testing"

	"github.com/stretchr/testify/assert"

	consulapi "github.com/hashicorp/consul/api"

	"github.com/tczekajlo/kube-consul-register/config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Controller: &config.ControllerConfig{
			ConsulAddress: "localhost",
			ConsulPort:    "8500",
			ConsulScheme:  "http",
			RegisterMode:  config.RegisterSingleMode,
		},
		Consul: consulapi.DefaultConfig(),
	}

	consulInstance := Factory{}
	consulInstance.New(cfg, "pod_name", "127.0.0.1")

	assert.Equal(t, "localhost:8500", cfg.Consul.Address, "wrong URI")
	assert.Equal(t, "http", cfg.Consul.Scheme, "wrong scheme")

	// Tests RegisterNodeMode
	cfg.Controller.RegisterMode = config.RegisterNodeMode
	consulInstance.New(cfg, "pod_name", "127.0.0.1")

	assert.Equal(t, "pod_name:8500", cfg.Consul.Address, "wrong URI")

	// Tests RegisterPodMode
	cfg.Controller.RegisterMode = config.RegisterPodMode
	consulInstance.New(cfg, "pod_name", "127.0.0.1")

	assert.Equal(t, "127.0.0.1:8500", cfg.Consul.Address, "wrong URI")

	// Tests https scheme
	cfg.Controller.ConsulScheme = "https"
	consulInstance.New(cfg, "pod_name", "127.0.0.1")

	assert.Equal(t, "https", cfg.Consul.Scheme, "wrong scheme")

	// Tests consul-unix scheme
	cfg.Controller.ConsulScheme = "consul-unix"
	cfg.Controller.RegisterMode = config.RegisterSingleMode
	cfg.Controller.ConsulPort = "8500"
	consulInstance.New(cfg, "pod_name", "127.0.0.1")

	assert.Equal(t, "localhost:8500", cfg.Consul.Address, "wrong URI")

}

func TestConsulAdapterMethods(t *testing.T) {
	var err error
	t.Parallel()

	cfg := &config.Config{
		Controller: &config.ControllerConfig{
			ConsulAddress: "localhost",
			ConsulPort:    "8500",
			ConsulScheme:  "http",
			RegisterMode:  config.RegisterSingleMode,
		},
		Consul: consulapi.DefaultConfig(),
	}

	consulInstance := Factory{}
	consulAgent := consulInstance.New(cfg, "pod_name", "127.0.0.1")

	err = consulAgent.Register(&consulapi.AgentServiceRegistration{})
	assert.NotNil(t, err, "An error was expected")

	err = consulAgent.Deregister(&consulapi.AgentServiceRegistration{})
	assert.NotNil(t, err, "An error was expected")

	_, err = consulAgent.Services()
	assert.NotNil(t, err, "An error was expected")

}
