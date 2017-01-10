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

// Factory has a method to return a FactoryAdapter.
type Factory struct{}

// New returns the ConsulAdapter.
func (f *Factory) New(cfg *config.Config, podNodeName string, podIP string) FactoryAdapter {
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

	if uri.Scheme == "consul-unix" {
		cfg.Consul.Address = strings.TrimPrefix(uri.String(), "consul-")

	} else if uri.Scheme == "consul-tls" {
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
		cfg.Consul.Scheme = "https"
		transport := cleanhttp.DefaultPooledTransport()
		transport.TLSClientConfig = tlsConfig
		cfg.Consul.HttpClient.Transport = transport
		cfg.Consul.Address = uri.Host

	} else if uri.Host != "" {
		cfg.Consul.Address = uri.Host
	}

	client, err := consulapi.NewClient(cfg.Consul)
	if err != nil {
		glog.Fatalf("consul: %s", uri.Scheme)
	}
	return &ConsulAdapter{client: client}
}

// ConsulAdapter returns Consul Client.
type ConsulAdapter struct {
	client *consulapi.Client
}

// Register registers new service in Consul
func (r *ConsulAdapter) Register(service *consulapi.AgentServiceRegistration) error {
	glog.V(1).Infof("Registering service %s with ID: %s", service.Name, service.ID)
	return r.client.Agent().ServiceRegister(service)
}

// Deregister deregisters a service in Consul
func (r *ConsulAdapter) Deregister(service *consulapi.AgentServiceRegistration) error {
	glog.V(1).Infof("Deregistering service with ID: %s", service.ID)
	return r.client.Agent().ServiceDeregister(service.ID)
}

// Services returns all services from a Consul Agent
func (r *ConsulAdapter) Services() (map[string]*consulapi.AgentService, error) {
	glog.V(1).Info("Getting Consul services")
	return r.client.Agent().Services()
}
