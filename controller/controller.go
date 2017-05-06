package controller

import (
	"github.com/tczekajlo/kube-consul-register/config"
	"github.com/tczekajlo/kube-consul-register/consul"
	"github.com/tczekajlo/kube-consul-register/controller/endpoints"
	"github.com/tczekajlo/kube-consul-register/controller/pods"
	"github.com/tczekajlo/kube-consul-register/controller/services"

	"k8s.io/client-go/kubernetes"
)

// Factory has a method to return a FactoryAdapter
type Factory struct{}

// New creates an instance of controller
func (f *Factory) New(clientset *kubernetes.Clientset, consulInstance consul.Adapter, cfg *config.Config, namespace string) FactoryAdapter {

	switch source := cfg.Controller.RegisterSource; source {
	case "service":
		return services.New(clientset, consulInstance, cfg, namespace)
	case "endpoint":
		return endpoints.New(clientset, consulInstance, cfg, namespace)
	default:
		return pods.New(clientset, consulInstance, cfg, namespace)
	}
}
