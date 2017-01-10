package controller

import (
	"github.com/golang/glog"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/types"
)

// FactoryAdapter has a method to work with Controller resources.
type FactoryAdapter interface {
	Watch()
	Sync() error
	Clean() error
}

// PodInfo represents information about a pod.
type PodInfo struct {
	UID               types.UID
	Name              string
	Namespace         string
	Phase             v1.PodPhase
	IP                string
	NodeName          string
	Containers        []v1.Container
	ContainerStatuses []v1.ContainerStatus
	Ready             v1.ConditionStatus
	Labels            map[string]string
	Annotations       map[string]string
}

func (p *PodInfo) save(obj interface{}) {
	objectMeta := obj.(*v1.Pod).ObjectMeta
	spec := obj.(*v1.Pod).Spec
	status := obj.(*v1.Pod).Status

	p.UID = objectMeta.UID
	p.Name = objectMeta.Name
	p.Namespace = objectMeta.Namespace
	p.Labels = objectMeta.Labels
	p.Annotations = objectMeta.Annotations

	p.NodeName = spec.NodeName
	p.Containers = spec.Containers

	p.Phase = status.Phase
	p.IP = status.PodIP
	p.ContainerStatuses = status.ContainerStatuses

	for _, condition := range status.Conditions {
		if condition.Type == "Ready" {
			p.Ready = condition.Status
		}
	}

	glog.V(4).Infof("Save PodInfo: %#v", p)
}
