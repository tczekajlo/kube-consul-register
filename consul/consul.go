package consul

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/tczekajlo/kube-consul-register/config"

	"github.com/golang/glog"
	consulapi "github.com/hashicorp/consul/api"
	cleanhttp "github.com/hashicorp/go-cleanhttp"
)

// Adapter builds configuration and returns Consul Client
type Adapter struct {
	client *consulapi.Client
}

// New returns the ConsulAdapter.
func (c *Adapter) New(cfg *config.Config, podNodeName string, podIP string) *Adapter {
	var address string

	//Build URI
	switch mode := cfg.Controller.RegisterMode; mode {
	case config.RegisterSingleMode:
		address = fmt.Sprintf("%s://%s:%s",
			cfg.Controller.ConsulScheme, cfg.Controller.ConsulAddress, cfg.Controller.ConsulPort)
	case config.RegisterNodeMode:
		address = fmt.Sprintf("%s://%s:%s",
			cfg.Controller.ConsulScheme, podNodeName, cfg.Controller.ConsulPort)
	case config.RegisterPodMode:
		address = fmt.Sprintf("%s://%s:%s",
			cfg.Controller.ConsulScheme, podIP, cfg.Controller.ConsulPort)
	}

	uri, err := url.Parse(address)
	if err != nil {
		glog.Fatalf("bad adapter uri: ")
	}

	switch uri.Scheme {
	case "consul-unix":
		cfg.Consul.Address = strings.TrimPrefix(uri.String(), "consul-")

	case "https":
		tlsConfigDesc := &consulapi.TLSConfig{
			Address:            uri.Host,
			CAFile:             cfg.Controller.ConsulCAFile,
			CertFile:           cfg.Controller.ConsulCertFile,
			KeyFile:            cfg.Controller.ConsulKeyFile,
			InsecureSkipVerify: cfg.Controller.ConsulInsecureSkipVerify,
		}
		tlsConfig, err := consulapi.SetupTLSConfig(tlsConfigDesc)
		if err != nil {
			glog.Fatalf("Cannot set up Consul TLSConfig: %s", err)
		}
		cfg.Consul.Scheme = uri.Scheme
		transport := cleanhttp.DefaultPooledTransport()
		transport.TLSClientConfig = tlsConfig
		cfg.Consul.HttpClient.Transport = transport
		cfg.Consul.Address = uri.Host

	default:
		cfg.Consul.Address = uri.Host
	}

	// Add Token
	if cfg.Controller.ConsulToken != "" {
		cfg.Consul.Token = cfg.Controller.ConsulToken
	}

	//Timeout
	cfg.Consul.HttpClient.Timeout = cfg.Controller.ConsulTimeout

	client, err := consulapi.NewClient(cfg.Consul)
	if err != nil {
		glog.Fatalf("consul: %s", uri.Scheme)
	}

	c.client = client
	return c
}

// Register registers new service in Consul
func (c *Adapter) Register(service *consulapi.AgentServiceRegistration) error {
	glog.V(1).Infof("Registering service %s with ID: %s", service.Name, service.ID)
	return c.client.Agent().ServiceRegister(service)
}

// Deregister deregisters a service in Consul
func (c *Adapter) Deregister(service *consulapi.AgentServiceRegistration) error {
	glog.V(1).Infof("Deregistering service with ID: %s", service.ID)
	return c.client.Agent().ServiceDeregister(service.ID)
}

// Services returns all services from a Consul Agent
func (c *Adapter) Services() (map[string]*consulapi.AgentService, error) {
	glog.V(1).Info("Getting Consul services")
	return c.client.Agent().Services()
}
