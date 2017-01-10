package consul

import (
	consulapi "github.com/hashicorp/consul/api"
)

// FactoryAdapter has a method to work with ConsulAdapter
type FactoryAdapter interface {
	Register(service *consulapi.AgentServiceRegistration) error
	Deregister(service *consulapi.AgentServiceRegistration) error
	Services() (map[string]*consulapi.AgentService, error)
}
