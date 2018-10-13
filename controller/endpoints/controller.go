package endpoints

import (
	"fmt"
	"strconv"
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
	"k8s.io/client-go/pkg/types"
	"k8s.io/client-go/tools/cache"

	consulapi "github.com/hashicorp/consul/api"
)

// These are valid annotations names which are take into account.
// "ConsulRegisterEnabledAnnotation" is a name of annotation key for `enabled` option.
const (
	ConsulRegisterEnabledAnnotation string = "consul.register/enabled"
)

var (
	addedEndpoints = make(map[types.UID]bool)

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
		nodes, err := c.clientset.CoreV1().Nodes().List(v1.ListOptions{
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
		pods, err := c.clientset.CoreV1().Pods("").List(v1.ListOptions{
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
	addedConsulServices, registeredEndpoints, err := c.getAddedConsulServices()
	if err != nil {
		c.mutex.Unlock()
		return err
	}

	endpoints, err := c.clientset.CoreV1().Endpoints("").List(v1.ListOptions{})
	if err != nil {
		return err
	}

	for _, endpoint := range endpoints.Items {
		if !isRegisterEnabled(&endpoint) {
			continue
		}

		for _, subset := range endpoint.Subsets {
			for _, address := range subset.Addresses {
				addedEndpoints[address.TargetRef.UID] = true
			}
		}
	}

	// Remove useless services
	for uid, services := range registeredEndpoints {
		if _, ok := addedEndpoints[types.UID(uid)]; !ok {
			for _, serviceID := range services {
				glog.Infof("Deletion of endpoint with UID %s (POD: %s)", uid, serviceID)
				// check if there consul agent instance
				if _, ok := addedConsulServices[serviceID]; !ok {
					glog.Warningf("Cannot find Consul Agent Instance for service with ID: %s", serviceID)
					continue
				}
				service := &consulapi.AgentServiceRegistration{ID: serviceID}
				err := consulAgents[addedConsulServices[serviceID]].Deregister(service)
				if err != nil {
					glog.Errorf("Can't deregister service: %s", err)
					continue
				}
				glog.Infof("Service's been deregistered, ID: %s", service.ID)
				glog.V(2).Infof("%#v", service)
				delete(addedConsulServices, service.ID)

			}
			delete(addedEndpoints, types.UID(uid))
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
	addedConsulServices, _, err := c.getAddedConsulServices()
	if err != nil {
		c.mutex.Unlock()
		return err
	}
	glog.V(3).Infof("Added services: %#v", addedConsulServices)

	endpoints, err := c.clientset.CoreV1().Endpoints("").List(v1.ListOptions{})
	if err != nil {
		return err
	}

	for _, endpoint := range endpoints.Items {
		if !isRegisterEnabled(&endpoint) {
			continue
		}

		if err := c.eventUpdateFunc(&endpoint, &endpoint); err != nil {
			glog.Errorf("Failed to sync endpoint %s: %s", endpoint.GetName(), err)
		}
	}

	c.mutex.Unlock()
	return nil
}

// Watch watches events in K8S cluster
func (c *Controller) Watch() {
	watchlist := cache.NewListWatchFromClient(c.clientset.CoreV1().RESTClient(), "endpoints", c.namespace,
		fields.Everything())
	_, controller := cache.NewInformer(
		watchlist,
		&v1.Endpoints{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			DeleteFunc: func(obj interface{}) {
				timer := prometheus.NewTimer(metrics.FuncDuration.WithLabelValues("delete"))
				defer timer.ObserveDuration()

				if !isRegisterEnabled(obj) {
					return
				}

				c.mutex.Lock()
				glog.Info("Endpoint deletion")
				if err := c.eventDeleteFunc(obj); err != nil {
					glog.Errorf("Failed to delete endpoints: %s", err)
				}
				c.mutex.Unlock()
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				timer := prometheus.NewTimer(metrics.FuncDuration.WithLabelValues("update"))
				defer timer.ObserveDuration()

				if !isRegisterEnabled(newObj) {
					return
				}

				c.mutex.Lock()
				glog.Info("Endpoint updation")
				if err := c.eventUpdateFunc(oldObj, newObj); err != nil {
					glog.Errorf("Failed to update endpoints: %s", err)
				}
				c.mutex.Unlock()

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

func (c *Controller) deleteEndpoint(nodeName, podIP, serviceID string) {
	consulAgent := c.consulInstance.New(c.cfg, nodeName, podIP)
	service := &consulapi.AgentServiceRegistration{ID: serviceID}
	err := consulAgent.Deregister(service)
	if err != nil {
		glog.Errorf("Can't deregister service: %s", err)
		metrics.ConsulFailure.WithLabelValues("deregister", consulAgent.Config.Address).Inc()
	} else {
		metrics.ConsulSuccess.WithLabelValues("deregister", consulAgent.Config.Address).Inc()
		glog.Infof("Service's been deregistered, ID: %s", service.ID)
		glog.V(2).Infof("%#v", service)
	}
}

func (c *Controller) eventDeleteFunc(obj interface{}) error {
	for _, subset := range obj.(*v1.Endpoints).Subsets {
		for _, address := range subset.Addresses {
			glog.Infof("Deletion of endpoint with UID %s (POD: %s)", address.TargetRef.UID, address.TargetRef.Name)

			// Get NodeName of endpoint
			pod, err := c.getPod(address.TargetRef.Namespace, address.TargetRef.Name)
			if err != nil {
				return err
			}
			ports := subset.Ports
			for _, port := range ports {
				serviceID := fmt.Sprintf("%s-%d", address.TargetRef.Name, port.Port)
				c.deleteEndpoint(pod.Spec.NodeName, pod.Status.PodIP, serviceID)
			}
			delete(addedEndpoints, address.TargetRef.UID)
		}
	}
	metrics.PodSuccess.WithLabelValues("delete").Inc()
	return nil
}

func (c *Controller) eventUpdateFunc(oldObj interface{}, newObj interface{}) error {
	var addedAddresses = make(map[types.UID]bool)

	// Check if any address has been deleted
	for _, subsetNew := range newObj.(*v1.Endpoints).Subsets {
		for _, addressNew := range subsetNew.Addresses {
			addedAddresses[addressNew.TargetRef.UID] = true
		}
	}

	for _, subsetOld := range oldObj.(*v1.Endpoints).Subsets {
		for _, addressOld := range subsetOld.Addresses {
			if _, ok := addedAddresses[addressOld.TargetRef.UID]; !ok {
				glog.Infof("Deletion of endpoint with UID %s (POD: %s)", addressOld.TargetRef.UID, addressOld.TargetRef.Name)

				// Get NodeName of endpoint
				pod, err := c.getPod(addressOld.TargetRef.Namespace, addressOld.TargetRef.Name)
				if err != nil {
					return err
				}
				ports := subsetOld.Ports
				for _, port := range ports {
					serviceID := fmt.Sprintf("%s-%d", addressOld.TargetRef.Name, port.Port)
					c.deleteEndpoint(pod.Spec.NodeName, pod.Status.PodIP, serviceID)
				}
				delete(addedAddresses, addressOld.TargetRef.UID)
			}
		}
	}

	// Register new endpoint
	for _, subset := range newObj.(*v1.Endpoints).Subsets {
		for _, address := range subset.Addresses {
			if _, ok := addedEndpoints[address.TargetRef.UID]; !ok {
				// Get NodeName of endpoint
				pod, err := c.getPod(address.TargetRef.Namespace, address.TargetRef.Name)
				if err != nil {
					return err
				}

				// Add service for each port
				ports := subset.Ports
				for _, port := range ports {
					// Convert endpoint to Consul's service
					service, err := c.createConsulService(newObj.(*v1.Endpoints), address, port)
					if err != nil {
						glog.Errorf("Can't convert endpoint to Consul's service: %s", err)
						metrics.PodFailure.WithLabelValues("update").Inc()
						continue
					}
					// Consul Agent
					consulAgent := c.consulInstance.New(c.cfg, pod.Spec.NodeName, pod.Status.PodIP)
					err = consulAgent.Register(service)
					if err != nil {
						glog.Errorf("Can't register service: %s", err)
						metrics.ConsulFailure.WithLabelValues("register", consulAgent.Config.Address).Inc()
					} else {
						glog.Infof("Service's been registered, Name: %s, ID: %s", service.Name, service.ID)
						glog.V(2).Infof("%#v", service)
						addedEndpoints[address.TargetRef.UID] = true
						metrics.ConsulSuccess.WithLabelValues("register", consulAgent.Config.Address).Inc()
					}
				}
			}
		}
	}

	metrics.PodSuccess.WithLabelValues("update").Inc()
	return nil
}

func (c *Controller) getPod(namespace string, podName string) (*v1.Pod, error) {
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(podName)
	if err != nil {
		return nil, err
	}

	return pod, nil
}

func (c *Controller) createConsulService(endpoint *v1.Endpoints, address v1.EndpointAddress, port v1.EndpointPort) (*consulapi.AgentServiceRegistration, error) {
	service := &consulapi.AgentServiceRegistration{}

	service.ID = fmt.Sprintf("%s-%d", address.TargetRef.Name, port.Port)
	service.Name = endpoint.ObjectMeta.Name

	//Add K8sTag from configuration
	service.Tags = []string{c.cfg.Controller.K8sTag}
	service.Tags = append(service.Tags, fmt.Sprintf("uid:%s", address.TargetRef.UID))
	service.Tags = append(service.Tags, labelsToTags(endpoint.ObjectMeta.Labels)...)

	service.Port = int(port.Port)
	service.Address = address.IP

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
	if value, ok := obj.(*v1.Endpoints).ObjectMeta.Annotations[ConsulRegisterEnabledAnnotation]; ok {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			glog.Errorf("Can't convert value of %s annotation: %s", ConsulRegisterEnabledAnnotation, err)
			return false
		}

		if !enabled {
			glog.Infof("Endpoint %s in %s namespace is disabled by annotation. Value: %s", obj.(*v1.Endpoints).ObjectMeta.Name, obj.(*v1.Endpoints).ObjectMeta.Namespace, value)
			return false
		}
	} else {
		glog.V(1).Infof("Endpoint %s in %s namespace will not be registered in Consul. Lack of annotation %s", obj.(*v1.Endpoints).ObjectMeta.Name, obj.(*v1.Endpoints).ObjectMeta.Namespace, ConsulRegisterEnabledAnnotation)
		return false
	}
	return true
}
