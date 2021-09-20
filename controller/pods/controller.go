package pods

import (
	"encoding/json"
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
	"k8s.io/client-go/pkg/types"
	"k8s.io/client-go/tools/cache"

	consulapi "github.com/hashicorp/consul/api"
)

// These are valid annotations names which are take into account.
// "ConsulRegisterEnabledAnnotation" is a name of annotation key for `enabled` option.
// "ConsulRegisterServiceNameAnnotation" is a name of annotation key for `service.name` option.
// "ConsulRegisterServiceMetaPrefixAnnotation" is a prefix name of annotation key for `service.meta` option.
// "CreatedByAnnotation" represents the key used to store the spec(json)
// used to create the resource
// "ExpectedContainerNamesAnnotation" is a name of container or list of names (separated by comma)
// which are take into account during register process.
const (
	ConsulRegisterEnabledAnnotation           string = "consul.register/enabled"
	ConsulRegisterServiceNameAnnotation       string = "consul.register/service.name"
	ConsulRegisterServiceMetaPrefixAnnotation string = "consul.register/service.meta."
	CreatedByAnnotation                       string = "kubernetes.io/created-by"
	ExpectedContainerNamesAnnotation          string = "consul.register/pod.container.name"
)

var (
	addedPods       = make(map[types.UID]bool)
	addedContainers = make(map[string]bool)
	addedServices   = make(map[string]bool)

	consulAgents map[string]*consul.Adapter
)

// Factory has a method to return a FactoryAdapter
//type Factory struct{}

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
		pods, err := c.clientset.CoreV1().Pods(c.namespace).List(v1.ListOptions{
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

	var podsInCluster []*PodInfo
	var err error

	c.mutex.Lock()

	consulAgents, err = c.cacheConsulAgent()
	if err != nil {
		c.mutex.Unlock()
		return fmt.Errorf("Can't cache Consul' Agents: %s", err)
	}

	// Get list of added Consul' services
	addedConsulServices, err := c.getAddedConsulServices()
	if err != nil {
		c.mutex.Unlock()
		return err
	}

	// Make list of Kubernetes PODs
	pods, err := c.clientset.CoreV1().Pods(c.namespace).List(v1.ListOptions{
		LabelSelector: c.cfg.Controller.PodLabelSelector,
	})
	if err != nil {
		c.mutex.Unlock()
		return err
	}

	for _, pod := range pods.Items {
		podInfo := &PodInfo{}
		podInfo.save(&pod)

		// If miss or consul.register/enabled annotation is set on `false` then skip pod
		if !podInfo.isRegisterEnabled() {
			continue
		}

		for _, container := range podInfo.ContainerStatuses {
			serviceID := fmt.Sprintf("%s-%s", podInfo.Name, container.Name)
			addedServices[serviceID] = true
		}

		podsInCluster = append(podsInCluster, podInfo) // nolint: megacheck
	}
	//Deletion of inactive services
	//Delete all services which doesn't exists in Consul
	//If service doesn't exists in addedService map then delete them
	for serviceID, consulAgentID := range addedConsulServices {
		if _, ok := addedServices[serviceID]; !ok {
			service := &consulapi.AgentServiceRegistration{ID: serviceID}
			err := consulAgents[consulAgentID].Deregister(service)
			if err != nil {
				glog.Errorf("Can't deregister service: %s", err)
				continue
			}
			glog.Infof("Service's been deregistered, ID: %s", service.ID)
			glog.V(2).Infof("%#v", service)
			delete(addedConsulServices, service.ID)
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
	addedConsulServices, err := c.getAddedConsulServices()
	if err != nil {
		c.mutex.Unlock()
		return err
	}
	glog.V(3).Infof("Added services: %#v", addedConsulServices)

	pods, err := c.clientset.CoreV1().Pods(c.namespace).List(v1.ListOptions{
		LabelSelector: c.cfg.Controller.PodLabelSelector,
	})
	if err != nil {
		c.mutex.Unlock()
		return err
	}

	for _, pod := range pods.Items {
		podInfo := &PodInfo{}
		podInfo.save(&pod)

		// If miss or consul.register/enabled annotation is set on `false` then skip pod
		if !podInfo.isRegisterEnabled() {
			continue
		}

		for _, container := range podInfo.ContainerStatuses {
			serviceID := fmt.Sprintf("%s-%s", podInfo.Name, container.Name)
			// If service does not appears in Consul's services then remove
			// container from addedContainers map and call update.
			if _, ok := addedConsulServices[serviceID]; !ok {
				delete(addedContainers, container.ContainerID)
				if err := eventUpdateFunc(&pod, c.consulInstance, c.cfg); err != nil {
					glog.Errorf("Failed to sync pod: %s: %s", podInfo.Name, err)
				}
			}
		}
	}
	c.mutex.Unlock()
	return nil
}

// Watch watches events in K8S cluster
func (c *Controller) Watch() {
	watchlist := cache.NewListWatchFromClient(c.clientset.CoreV1().RESTClient(), "pods", c.namespace,
		fields.Everything())
	_, controller := cache.NewInformer(
		watchlist,
		&v1.Pod{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				timer := prometheus.NewTimer(metrics.FuncDuration.WithLabelValues("add"))
				defer timer.ObserveDuration()

				podInfo := &PodInfo{}
				podInfo.save(obj)

				glog.V(1).Infof("POD ADD: Name: %s, Namespace: %s, Phase: %s", podInfo.Name, podInfo.Namespace, podInfo.Phase)
				metrics.PodSuccess.WithLabelValues("add").Inc()
			},
			DeleteFunc: func(obj interface{}) {
				timer := prometheus.NewTimer(metrics.FuncDuration.WithLabelValues("delete"))
				defer timer.ObserveDuration()

				if !utils.HasLabel(obj.(*v1.Pod).ObjectMeta.Labels, c.cfg.Controller.PodLabelSelector) && c.cfg.Controller.PodLabelSelector != "" {
					glog.Infof("Skip pod %s. Label selector is %s, pod's labels: %#v",
						obj.(*v1.Pod).ObjectMeta.Name, c.cfg.Controller.PodLabelSelector, obj.(*v1.Pod).ObjectMeta.Labels)
					return
				}
				c.mutex.Lock()
				if err := eventDeleteFunc(obj, c.consulInstance, c.cfg); err != nil {
					glog.Errorf("Failed to delete pods: %s", err)
				}
				c.mutex.Unlock()
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				timer := prometheus.NewTimer(metrics.FuncDuration.WithLabelValues("update"))
				defer timer.ObserveDuration()

				if !utils.HasLabel(newObj.(*v1.Pod).ObjectMeta.Labels, c.cfg.Controller.PodLabelSelector) && c.cfg.Controller.PodLabelSelector != "" {
					glog.Infof("Skip pod %s. Label selector is %s, pod's labels: %#v",
						newObj.(*v1.Pod).ObjectMeta.Name, c.cfg.Controller.PodLabelSelector, newObj.(*v1.Pod).ObjectMeta.Labels)
					return
				}
				c.mutex.Lock()
				if err := eventUpdateFunc(newObj, c.consulInstance, c.cfg); err != nil {
					glog.Errorf("Failed to update pods: %s", err)
				}
				c.mutex.Unlock()
			},
		},
	)

	stop := make(chan struct{})
	controller.Run(stop)
}

// getAddedConsulServices returns the list of added Consul Services
func (c *Controller) getAddedConsulServices() (map[string]string, error) {
	var addedServices = make(map[string]string)

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
				}
			}
		}
	}
	return addedServices, nil
}

func eventDeleteFunc(obj interface{}, consulInstance consul.Adapter, cfg *config.Config) error {
	podInfo := &PodInfo{}
	podInfo.save(obj)

	if !podInfo.isRegisterEnabled() {
		return nil
	}
	glog.Infof("POD DELETE: Name: %s, Namespace: %s, Phase: %s, Ready: %s", podInfo.Name, podInfo.Namespace, podInfo.Phase, podInfo.Ready)
	delete(addedPods, podInfo.UID)

	for _, container := range podInfo.ContainerStatuses {
		glog.Infof("Container %s in POD %s has status: Ready:%t", container.Name, podInfo.Name, container.Ready)

		//Deletion service from consul
		glog.Infof("Deleting service for container %s in POD %s to consul", container.Name, podInfo.Name)

		// Consul Agent
		consulAgent := consulInstance.New(cfg, podInfo.NodeName, podInfo.IP)
		serviceID := fmt.Sprintf("%s-%s", podInfo.Name, container.Name)
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

		delete(addedContainers, container.ContainerID)
	}

	metrics.PodSuccess.WithLabelValues("delete").Inc()
	return nil
}

func eventUpdateFunc(obj interface{}, consulInstance consul.Adapter, cfg *config.Config) error {
	podInfo := &PodInfo{}
	podInfo.save(obj)

	message := fmt.Sprintf("POD UPDATE: Name: %s, Namespace: %s, Phase: %s, Ready: %s", podInfo.Name, podInfo.Namespace, podInfo.Phase, podInfo.Ready)

	if !podInfo.isRegisterEnabled() {
		return nil
	}

	//Add service if POD has 'Running' status
	if podInfo.Phase == v1.PodRunning {
		glog.Info(message)

		for _, container := range podInfo.ContainerStatuses {
			if container.Name == cfg.Controller.ConsulContainerName {
				glog.Infof("Container %s name's equal to `consul_container_name` value. Skipping registering.", container.Name)
				continue
			}

			if !podInfo.expectedContainerNames(container.Name) {
				glog.Infof("Container %s is not on list of allowed containers. Use %s annotation. Omitted.", container.Name, ExpectedContainerNamesAnnotation)
				continue
			}

			glog.Infof("Container %s in POD %s has status: Ready:%t", container.Name, podInfo.Name, container.Ready)

			//Add service to consul
			if _, ok := addedContainers[container.ContainerID]; !ok && container.Ready {
				glog.Infof("Adding service for container %s in POD %s to consul", container.Name, podInfo.Name)
				// Convert POD to Consul's service
				service, err := podInfo.PodToConsulService(container, cfg)
				if err != nil {
					glog.Errorf("Can't convert POD to Consul's service: %s", err)
					metrics.PodFailure.WithLabelValues("update").Inc()
					continue
				}

				// Consul Agent
				consulAgent := consulInstance.New(cfg, podInfo.NodeName, podInfo.IP)
				err = consulAgent.Register(service)
				if err != nil {
					glog.Errorf("Can't register service: %s", err)
					metrics.ConsulFailure.WithLabelValues("register", consulAgent.Config.Address).Inc()
				} else {
					glog.Infof("Service's been registered, Name: %s, ID: %s", service.Name, service.ID)
					glog.V(2).Infof("%#v", service)
					addedContainers[container.ContainerID] = true
					metrics.ConsulSuccess.WithLabelValues("register", consulAgent.Config.Address).Inc()
				}
			} else if _, ok := addedContainers[container.ContainerID]; ok && !container.Ready {
				glog.Warningf("Container %s in POD %s has status: Ready:%t, RestartCount:%d", container.Name, podInfo.Name, container.Ready, container.RestartCount)
				glog.Warningf("Removing service for container %s in POD %s from consul", container.Name, podInfo.Name)

				delete(addedContainers, container.ContainerID)
			}
		}
	} else if podInfo.Phase == v1.PodRunning && podInfo.Ready == v1.ConditionTrue {
		if _, ok := addedPods[podInfo.UID]; !ok {
			addedPods[podInfo.UID] = true
		}
	} else {
		glog.V(1).Info(message)

		for _, container := range podInfo.ContainerStatuses {
			glog.V(1).Infof("Container %s in POD %s has status: Ready:%t", container.Name, podInfo.Name, container.Ready)
			glog.V(3).Infof("%#v", container)
		}
	}

	metrics.PodSuccess.WithLabelValues("update").Inc()
	return nil
}

// PodToConsulService converts POD data to Consul service structure
func (p *PodInfo) PodToConsulService(containerStatus v1.ContainerStatus, cfg *config.Config) (*consulapi.AgentServiceRegistration, error) {
	service := &consulapi.AgentServiceRegistration{}

	if value, ok := p.Annotations[ConsulRegisterServiceNameAnnotation]; ok {
		service.Name = value
	} else {
		reference, found := p.getReference()
		if found {
			service.Name = reference.Reference.Name
		} else {
			service.Name = p.Name
		}
	}

	service.ID = fmt.Sprintf("%s-%s", p.Name, containerStatus.Name)
	service.Tags = p.labelsToTags(containerStatus.Name)
	service.Meta = p.annotationsToMeta()

	//Add K8sTag from configuration
	service.Tags = append(service.Tags, cfg.Controller.K8sTag)

	port := p.getContainerPort(containerStatus.Name)
	if port == 0 {
		return service, fmt.Errorf("Port's equal to 0")
	}
	service.Port = port
	service.Check = p.livenessProbeToConsulCheck(p.getContainerLivenessProbe(containerStatus.Name))
	service.Address = p.IP

	return service, nil
}

func (p *PodInfo) isRegisterEnabled() bool {
	if value, ok := p.Annotations[ConsulRegisterEnabledAnnotation]; ok {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			glog.Errorf("Can't convert value of %s annotation: %s", ConsulRegisterEnabledAnnotation, err)
			return false
		}

		if !enabled {
			glog.Infof("Pod %s in %s namespace is disabled by annotation. Value: %s", p.Name, p.Namespace, value)
			return false
		}
	} else {
		glog.V(1).Infof("Pod %s in %s namespace will not be registered in Consul. Lack of annotation %s", p.Name, p.Namespace, ConsulRegisterEnabledAnnotation)
		return false
	}
	return true
}

func (p *PodInfo) expectedContainerNames(containerName string) bool {
	if value, ok := p.Annotations[ExpectedContainerNamesAnnotation]; ok {
		for _, name := range strings.Split(value, ",") {
			if containerName == name {
				return true
			}
		}
	} else {
		return true
	}

	return false
}

func (p *PodInfo) livenessProbeToConsulCheck(probe *v1.Probe) *consulapi.AgentServiceCheck {
	check := &consulapi.AgentServiceCheck{}

	if probe == nil {
		return check
	}

	if probe.Handler.Exec != nil {
		return check
	}

	check.Status = "passing"
	check.Interval = fmt.Sprintf("%ds", probe.PeriodSeconds)
	check.Timeout = fmt.Sprintf("%ds", probe.TimeoutSeconds)

	host := p.IP

	if probe.Handler.HTTPGet != nil {
		if probe.Handler.HTTPGet.Host != "" {
			host = probe.Handler.HTTPGet.Host
		}
		check.HTTP = fmt.Sprintf("%s://%s:%d%s", probe.Handler.HTTPGet.Scheme, host, probe.Handler.HTTPGet.Port.IntVal, probe.Handler.HTTPGet.Path)
	} else if probe.Handler.TCPSocket != nil {
		check.TCP = fmt.Sprintf("%s:%d", host, probe.Handler.TCPSocket.Port.IntVal)
	}
	glog.V(3).Infof("Consul check: %#v", check)
	return check
}

func (p *PodInfo) getContainerLivenessProbe(searchContainer string) *v1.Probe {
	for _, container := range p.Containers {
		if container.Name == searchContainer {
			return container.LivenessProbe
		}
	}
	return nil
}

func (p *PodInfo) labelsToTags(containerName string) []string {
	var tags []string
	tags = append(tags, p.Name)
	tags = append(tags, fmt.Sprintf("pod:%s", p.Name))
	tags = append(tags, fmt.Sprintf("node:%s", p.NodeName))
	tags = append(tags, fmt.Sprintf("container:%s", containerName))

	for key, value := range p.Labels {
		// if value is equal to "tag" then set only key as tag
		if value == "tag" {
			tags = append(tags, key)
		} else {
			tags = append(tags, fmt.Sprintf("%s:%s", key, value))
		}
	}
	return tags
}

func (p *PodInfo) annotationsToMeta() map[string]string {
	meta := make(map[string]string)
	for key, value := range p.Annotations {
		if strings.HasPrefix(key, ConsulRegisterServiceMetaPrefixAnnotation) {
			meta[strings.TrimPrefix(key, ConsulRegisterServiceMetaPrefixAnnotation)] = value
		}
	}
	return meta
}

func (p *PodInfo) getContainerPort(searchContainer string) int {
	for _, container := range p.Containers {
		if container.Name == searchContainer {
			if len(container.Ports) > 0 {
				return int(container.Ports[0].ContainerPort)
			}
		}
	}
	glog.Warningf("Container hasn't set ContainerPort")
	return 0
}

func (p *PodInfo) getReference() (v1.SerializedReference, bool) {
	var sr v1.SerializedReference

	creatorRefJSON, found := p.Annotations[CreatedByAnnotation]
	if !found {
		glog.V(4).Infof("Pod with no created-by annotation")
		return sr, false
	}

	err := json.Unmarshal([]byte(creatorRefJSON), &sr)
	if err != nil {
		glog.V(4).Infof("Pod with unparsable created-by annotation: %v", err)
		return sr, false
	}
	return sr, true
}
