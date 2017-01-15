package consul

import (
	"testing"
	"time"

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
			ConsulToken:   "token",
			RegisterMode:  config.RegisterSingleMode,
		},
		Consul: consulapi.DefaultConfig(),
	}

	consulInstance := Adapter{}
	consulInstance.New(cfg, "pod_name", "127.0.0.1")

	assert.Equal(t, cfg.Consul.Address, "localhost:8500", "wrong URI")
	assert.Equal(t, cfg.Consul.Scheme, "http", "wrong scheme")
	assert.Equal(t, cfg.Consul.Token, "token", "wrong token")
	assert.Equal(t, cfg.Consul.HttpClient.Timeout, time.Duration(0), "wrong timeout")

	// Tests Consul Timeout
	cfg.Controller.ConsulTimeout = time.Duration(1 * time.Second)
	consulInstance.New(cfg, "pod_name", "127.0.0.1")

	assert.Equal(t, cfg.Consul.HttpClient.Timeout, time.Duration(1*time.Second), "wrong timeout")

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

	consulInstance := Adapter{}
	consulAgent := consulInstance.New(cfg, "pod_name", "127.0.0.1")

	err = consulAgent.Register(&consulapi.AgentServiceRegistration{})
	assert.NotNil(t, err, "An error was expected")

	err = consulAgent.Deregister(&consulapi.AgentServiceRegistration{})
	assert.NotNil(t, err, "An error was expected")

	_, err = consulAgent.Services()
	assert.NotNil(t, err, "An error was expected")

}
