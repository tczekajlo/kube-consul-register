package services

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tczekajlo/kube-consul-register/config"
	"github.com/tczekajlo/kube-consul-register/consul"
	"github.com/tczekajlo/kube-consul-register/metrics"
	"github.com/tczekajlo/kube-consul-register/utils"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/fields"
	"k8s.io/client-go/tools/cache"

	consulapi "github.com/hashicorp/consul/api"
)

// These are valid annotations names which are take into account.
// "ConsulRegisterEnabledAnnotation" is a name of annotation key for `enabled` option.
const (
	ConsulRegisterEnabledAnnotation string = "consul.register/enabled"
)

var (
	allAddedServices = make(map[string]bool)
	addedServices    = make(map[string]bool)

	consulAgents map[string]*consul.Adapter
)

// Controller describes the attributes that are uses by Controller
type Controller struct {
	clientset      *kubernetes.Clientset
	consulInstance consul.Adapter
	cfg            *config.Config
	namespace      string
	mutex          *sync.Mutex
}

// New creates an instance of controller
func New(clientset *kubernetes.Clientset, consulInstance consul.Adapter, cfg *config.Config, namespace string) FactoryAdapter {
	return &Controller{
		clientset:      clientset,
		consulInstance: consulInstance,
		cfg:            cfg,
		namespace:      namespace,
		mutex:          &sync.Mutex{}}
}

func (c *Controller) cacheConsulAgent() (map[string]*consul.Adapter, error) {
	consulAgents = make(map[string]*consul.Adapter)
	//Cache Consul's Agents
	if c.cfg.Controller.RegisterMode == config.RegisterSingleMode {
		consulAgent := c.consulInstance.New(c.cfg, "", "")
		consulAgents[c.cfg.Controller.ConsulAddress] = consulAgent

	} else if c.cfg.Controller.RegisterMode == config.RegisterNodeMode {
		nodes, err := c.clientset.Core().Nodes().List(v1.ListOptions{
			LabelSelector: c.cfg.Controller.ConsulNodeSelector,
		})
		if err != nil {
			return consulAgents, err
		}

		for _, node := range nodes.Items {
			consulInstance := consul.Adapter{}
			consulAgent := consulInstance.New(c.cfg, node.ObjectMeta.Name, "")
			consulAgents[node.ObjectMeta.Name] = consulAgent
		}
	} else if c.cfg.Controller.RegisterMode == config.RegisterPodMode {
		pods, err := c.clientset.Core().Pods("").List(v1.ListOptions{
			LabelSelector: c.cfg.Controller.PodLabelSelector,
		})
		if err != nil {
			return consulAgents, err
		}
		for _, pod := range pods.Items {
			consulInstance := consul.Adapter{}
			consulAgent := consulInstance.New(c.cfg, "", pod.Status.HostIP)
			consulAgents[pod.Status.HostIP] = consulAgent
		}
	}

	return consulAgents, nil
}

// Clean checks Consul services and remove them if service does not appear in K8S cluster
func (c *Controller) Clean() error {
	timer := prometheus.NewTimer(metrics.FuncDuration.WithLabelValues("clean"))
	defer timer.ObserveDuration()

	var err error

	c.mutex.Lock()

	consulAgents, err = c.cacheConsulAgent()
	if err != nil {
		c.mutex.Unlock()
		return fmt.Errorf("Can't cache Consul' Agents: %s", err)
	}

	// Get list of added Consul' services
	// addedConsulServices map[string]string serviceConsulID:consul_agent_hostname
	// registeredConsulServices map[string][]string UID:serviceConsulID
	addedConsulServices, registeredConsulServices, err := c.getAddedConsulServices()
	if err != nil {
		c.mutex.Unlock()
		return err
	}
	glog.V(3).Infof("Added services: %#v", addedConsulServices)

	allServices, err := c.clientset.Core().Services(c.namespace).List(v1.ListOptions{})
	if err != nil {
		c.mutex.Unlock()
		return err
	}

	var currentAddedServices = make(map[string]string)
	for _, service := range allServices.Items {
		if !isRegisterEnabled(&service) {
			continue
		}
		currentAddedServices[string(service.ObjectMeta.UID)] = service.ObjectMeta.Name
	}

	for uid, serviceConsulID := range registeredConsulServices {
		if name, ok := currentAddedServices[uid]; !ok {
			for _, serviceID := range serviceConsulID {
				consulAgent := consulAgents[addedConsulServices[serviceID]]
				consulService := &consulapi.AgentServiceRegistration{
					ID: serviceID,
				}

				err = consulAgent.Deregister(consulService)
				if err != nil {
					glog.Errorf("Cannot deregister service in Consul: %s", err)
					metrics.ConsulFailure.WithLabelValues("deregister", consulAgent.Config.Address).Inc()
				} else {
					delete(allAddedServices, serviceID)
					glog.Infof("Service %s has been deregistered in Consul with ID: %s", name, serviceID)
					metrics.ConsulSuccess.WithLabelValues("deregister", consulAgent.Config.Address).Inc()
				}
			}
		}
	}

	c.mutex.Unlock()
	return nil
}

// Sync synchronizes services between Consul and K8S cluster
func (c *Controller) Sync() error {
	timer := prometheus.NewTimer(metrics.FuncDuration.WithLabelValues("sync"))
	defer timer.ObserveDuration()

	var err error

	c.mutex.Lock()

	consulAgents, err = c.cacheConsulAgent()
	if err != nil {
		c.mutex.Unlock()
		return fmt.Errorf("Can't cache Consul' Agents: %s", err)
	}
	glog.V(2).Infof("Agents: %#v", consulAgents)

	// Get list of added Consul' services
	// addedConsulServices map[string]string serviceConsulID:consul_agent_hostname
	// registeredConsulServices map[string][]string UID:serviceConsulID
	addedConsulServices, registeredConsulServices, err := c.getAddedConsulServices()
	if err != nil {
		c.mutex.Unlock()
		return err
	}
	glog.V(3).Infof("Added services: %#v", addedConsulServices)

	allServices, err := c.clientset.Core().Services(c.namespace).List(v1.ListOptions{})
	if err != nil {
		c.mutex.Unlock()
		return err
	}

	for _, service := range allServices.Items {
		if !isRegisterEnabled(&service) {
			continue
		}

		// Check if service has already added to Consul
		if consulServices, ok := registeredConsulServices[string(service.ObjectMeta.UID)]; ok {
			for _, serviceConsulID := range consulServices {
				if _, ok := addedConsulServices[serviceConsulID]; !ok {
					err := c.eventAddFunc(&service)
					if err != nil {
						return err
					}
				}
			}
		} else {
			err := c.eventAddFunc(&service)
			if err != nil {
				return err
			}
		}
	}
	c.mutex.Unlock()
	return nil
}

// nodeDelete deletes service after node deletion
func (c *Controller) nodeDelete(obj interface{}) error {
	timer := prometheus.NewTimer(metrics.FuncDuration.WithLabelValues("sync"))
	defer timer.ObserveDuration()

	var err error

	c.mutex.Lock()
	allAddedServices = make(map[string]bool)

	consulAgents, err = c.cacheConsulAgent()
	if err != nil {
		c.mutex.Unlock()
		return fmt.Errorf("Can't cache Consul' Agents: %s", err)
	}
	glog.V(2).Infof("Agents: %#v", consulAgents)

	// Get list of added Consul' services
	// addedConsulServices map[string]string serviceConsulID:consul_agent_hostname
	// registeredConsulServices map[string][]string UID:serviceConsulID
	addedConsulServices, _, err := c.getAddedConsulServices()
	if err != nil {
		c.mutex.Unlock()
		return err
	}
	glog.V(3).Infof("Added services: %#v", addedConsulServices)

	for serviceConsulID, consulAgentHostname := range addedConsulServices {
		for _, address := range obj.(*v1.Node).Status.Addresses {
			if strings.Contains(serviceConsulID, "-"+address.Address+"-") {
				consulAgent := consulAgents[consulAgentHostname]
				consulService := &consulapi.AgentServiceRegistration{
					ID: serviceConsulID,
				}

				err = consulAgent.Deregister(consulService)
				if err != nil {
					glog.Errorf("Cannot deregister service in Consul: %s", err)
					metrics.ConsulFailure.WithLabelValues("deregister", consulAgent.Config.Address).Inc()
				} else {
					delete(allAddedServices, serviceConsulID)
					glog.Infof("Service has been deregistered in Consul with ID: %s", serviceConsulID)
					metrics.ConsulSuccess.WithLabelValues("deregister", consulAgent.Config.Address).Inc()
				}
			}
		}
	}

	c.mutex.Unlock()
	return nil
}

// Watch watches events in K8S cluster
func (c *Controller) Watch() {
	go c.watchNodes()
	go c.watchServices()
}

func (c *Controller) watchNodes() {
	watchlist := cache.NewListWatchFromClient(c.clientset.Core().RESTClient(), "nodes", c.namespace,
		fields.Everything())
	_, controller := cache.NewInformer(
		watchlist,
		&v1.Node{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				c.mutex.Lock()
				glog.Info("Add node.")
				allServices, err := c.clientset.Core().Services(c.namespace).List(v1.ListOptions{})
				if err != nil {
					c.mutex.Unlock()
				}

				for _, service := range allServices.Items {
					if !isRegisterEnabled(&service) {
						continue
					}

					c.eventAddFunc(&service)
				}
				c.mutex.Unlock()
			},
			DeleteFunc: func(obj interface{}) {
				glog.Info("Delete node. ")
				err := c.nodeDelete(obj)
				if err != nil {
					glog.Error(err)
				}
			},
		},
	)

	stop := make(chan struct{})
	controller.Run(stop)
}

func (c *Controller) watchServices() {
	watchlist := cache.NewListWatchFromClient(c.clientset.Core().RESTClient(), "services", c.namespace,
		fields.Everything())
	_, controller := cache.NewInformer(
		watchlist,
		&v1.Service{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				if !isRegisterEnabled(obj) {
					return
				}

				c.mutex.Lock()
				c.eventAddFunc(obj)
				c.mutex.Unlock()
			},
			DeleteFunc: func(obj interface{}) {
				timer := prometheus.NewTimer(metrics.FuncDuration.WithLabelValues("delete"))
				defer timer.ObserveDuration()
				if !isRegisterEnabled(obj) {
					return
				}

				c.mutex.Lock()
				c.eventDeleteFunc(obj)
				c.mutex.Unlock()
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				timer := prometheus.NewTimer(metrics.FuncDuration.WithLabelValues("update"))
				defer timer.ObserveDuration()
				if !isRegisterEnabled(newObj) {
					// Deregister the service on update if disabled
					c.mutex.Lock()
					c.eventDeleteFunc(newObj)
					c.mutex.Unlock()
				} else {
					c.mutex.Lock()
					c.eventAddFunc(newObj)
					c.mutex.Unlock()
				}
			},
		},
	)

	stop := make(chan struct{})
	controller.Run(stop)
}

// getAddedConsulServices returns the list of added Consul Services
func (c *Controller) getAddedConsulServices() (map[string]string, map[string][]string, error) {
	var addedServices = make(map[string]string)
	var registeredConsulServices = make(map[string][]string)

	// Make list of Consul's services
	for consulAgentID, consulAgent := range consulAgents {
		services, err := consulAgent.Services()
		if err != nil {
			glog.Errorf("Can't get services from Consul Agent, register mode=%s: %s", c.cfg.Controller.RegisterMode, err)
		} else {
			glog.V(3).Infof("agent: %#v, services: %#v", consulAgentID, services)
			for _, service := range services {
				if utils.CheckK8sTag(service.Tags, c.cfg.Controller.K8sTag) {
					addedServices[service.ID] = consulAgentID

					uid := utils.GetConsulServiceTag(service.Tags, "uid")
					if value, ok := registeredConsulServices[uid]; ok {
						registeredConsulServices[uid] = append(value, service.ID)
					} else {
						registeredConsulServices[uid] = []string{service.ID}
					}
				}
			}
		}
	}
	return addedServices, registeredConsulServices, nil
}

func (c *Controller) eventAddFunc(obj interface{}) error {
	if !isRegisterEnabled(obj) {
		return nil
	}

	var nodesIPs []string
	var ports []int32
	var err error

	switch serviceType := obj.(*v1.Service).Spec.Type; serviceType {
	case v1.ServiceTypeNodePort:
		// Check if ExternalIPs is empty
		if len(obj.(*v1.Service).Spec.ExternalIPs) > 0 {
			nodesIPs = obj.(*v1.Service).Spec.ExternalIPs
		} else {
			nodesIPs, err = c.getNodesIPs()
			if err != nil {
				return err
			}
		}
		for _, port := range obj.(*v1.Service).Spec.Ports {
			if port.Protocol == v1.ProtocolTCP {
				ports = append(ports, port.NodePort)
			}
		}

		// Now is time to add service to Consul
		for _, nodeAddress := range nodesIPs {
			for _, port := range ports {
				// Add to Consul
				service, err := c.createConsulService(obj.(*v1.Service), nodeAddress, port)
				if err != nil {
					glog.Errorf("Cannot create Consul service: %s", err)
					continue
				}
				// Check if service's already added
				if _, ok := allAddedServices[service.ID]; ok {
					glog.V(3).Infof("Service %s has already registered in Consul", service.ID)
					continue
				}

				consulAgent := c.consulInstance.New(c.cfg, nodeAddress, "")
				err = consulAgent.Register(service)
				if err != nil {
					glog.Errorf("Cannot register service in Consul: %s", err)
					metrics.ConsulFailure.WithLabelValues("register", consulAgent.Config.Address).Inc()
				} else {
					allAddedServices[service.ID] = true
					glog.Infof("Service %s has been registered in Consul with ID: %s", obj.(*v1.Service).ObjectMeta.Name, service.ID)
					metrics.ConsulSuccess.WithLabelValues("register", consulAgent.Config.Address).Inc()
				}
			}
		}
	case v1.ServiceTypeClusterIP:
		// Check if ExternalIPs is empty
		if len(obj.(*v1.Service).Spec.ExternalIPs) > 0 {
			nodesIPs = obj.(*v1.Service).Spec.ExternalIPs
		} else {
			return nil
		}
		for _, port := range obj.(*v1.Service).Spec.Ports {
			if port.Protocol == v1.ProtocolTCP {
				ports = append(ports, port.NodePort)
			}
		}

		// Now is time to add service to Consul
		for _, nodeAddress := range nodesIPs {
			for _, port := range ports {
				// Add to Consul
				service, err := c.createConsulService(obj.(*v1.Service), nodeAddress, port)
				if err != nil {
					glog.Errorf("Cannot create Consul service: %s", err)
					continue
				}
				// Check if service's already added
				if _, ok := allAddedServices[service.ID]; ok {
					glog.V(3).Infof("Service %s has already registered in Consul", service.ID)
					continue
				}

				consulAgent := c.consulInstance.New(c.cfg, nodeAddress, "")
				err = consulAgent.Register(service)
				if err != nil {
					glog.Errorf("Cannot register service in Consul: %s", err)
					metrics.ConsulFailure.WithLabelValues("register", consulAgent.Config.Address).Inc()
				} else {
					allAddedServices[service.ID] = true
					glog.Infof("Service %s has been registered in Consul with ID: %s", obj.(*v1.Service).ObjectMeta.Name, service.ID)
					metrics.ConsulSuccess.WithLabelValues("register", consulAgent.Config.Address).Inc()
				}
			}
		}
	}
	return nil
}

func (c *Controller) getNodesIPs() ([]string, error) {
	nodes, err := c.clientset.Core().Nodes().List(v1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var addresses []string
	for _, node := range nodes.Items {
		for _, address := range node.Status.Addresses {
			switch addressType := address.Type; addressType {
			case v1.NodeExternalIP:
				addresses = append(addresses, address.Address)
			case v1.NodeInternalIP:
				addresses = append(addresses, address.Address)
			}
		}
	}
	return addresses, nil
}

func (c *Controller) eventDeleteFunc(obj interface{}) error {
	var nodesIPs []string
	var ports []int32
	var err error

	switch serviceType := obj.(*v1.Service).Spec.Type; serviceType {
	case v1.ServiceTypeNodePort:
		// Check if ExternalIPs is empty
		if len(obj.(*v1.Service).Spec.ExternalIPs) > 0 {
			nodesIPs = obj.(*v1.Service).Spec.ExternalIPs
		} else {
			nodesIPs, err = c.getNodesIPs()
			if err != nil {
				return err
			}
		}
		for _, port := range obj.(*v1.Service).Spec.Ports {
			if port.Protocol == v1.ProtocolTCP {
				ports = append(ports, port.NodePort)
			}
		}

		// Now is time to deregister services from Consul
		for _, nodeAddress := range nodesIPs {
			for _, port := range ports {
				// Add to Consul
				service, err := c.createConsulService(obj.(*v1.Service), nodeAddress, port)
				if err != nil {
					glog.Errorf("Cannot create Consul service: %s", err)
					continue
				}
				// Check if service's already added
				if _, ok := allAddedServices[service.ID]; !ok {
					glog.V(3).Infof("Service %s has already been deleted in Consul", service.ID)
					continue
				}
				consulAgent := c.consulInstance.New(c.cfg, nodeAddress, "")
				err = consulAgent.Deregister(service)
				if err != nil {
					glog.Errorf("Cannot deregister service in Consul: %s", err)
					metrics.ConsulFailure.WithLabelValues("deregister", consulAgent.Config.Address).Inc()
				} else {
					glog.Infof("Service %s has been deregistered in Consul with ID: %s", obj.(*v1.Service).ObjectMeta.Name, service.ID)
					metrics.ConsulSuccess.WithLabelValues("deregister", consulAgent.Config.Address).Inc()
					delete(allAddedServices, service.ID)
				}
			}
		}
	}
	return nil
}

func (c *Controller) getPod(namespace string, podName string) (*v1.Pod, error) {
	pod, err := c.clientset.Core().Pods(namespace).Get(podName)
	if err != nil {
		return nil, err
	}

	return pod, nil
}

func (c *Controller) createConsulService(svc *v1.Service, address string, port int32) (*consulapi.AgentServiceRegistration, error) {
	service := &consulapi.AgentServiceRegistration{}

	service.ID = fmt.Sprintf("%s-%s-%s-%d", svc.ObjectMeta.Name, svc.ObjectMeta.UID, address, port)
	service.Name = svc.ObjectMeta.Name

	//Add K8sTag from configuration
	service.Tags = []string{c.cfg.Controller.K8sTag}
	service.Tags = append(service.Tags, fmt.Sprintf("uid:%s", svc.ObjectMeta.UID))
	service.Tags = append(service.Tags, labelsToTags(svc.ObjectMeta.Labels)...)

	service.Port = int(port)
	service.Address = address

	return service, nil
}

func labelsToTags(labels map[string]string) []string {
	var tags []string

	for key, value := range labels {
		// if value is equal to "tag" then set only key as tag
		if value == "tag" {
			tags = append(tags, key)
		} else {
			tags = append(tags, fmt.Sprintf("%s:%s", key, value))
		}
	}
	return tags

}

func isRegisterEnabled(obj interface{}) bool {
	if value, ok := obj.(*v1.Service).ObjectMeta.Annotations[ConsulRegisterEnabledAnnotation]; ok {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			glog.Errorf("Can't convert value of %s annotation: %s", ConsulRegisterEnabledAnnotation, err)
			return false
		}

		if !enabled {
			glog.Infof("Service %s in %s namespace is disabled by annotation. Value: %s", obj.(*v1.Service).ObjectMeta.Name, obj.(*v1.Service).ObjectMeta.Namespace, value)
			return false
		}
	} else {
		glog.V(1).Infof("Service %s in %s namespace will not be registered in Consul. Lack of annotation %s", obj.(*v1.Service).ObjectMeta.Name, obj.(*v1.Service).ObjectMeta.Namespace, ConsulRegisterEnabledAnnotation)
		return false
	}
	return true
}
