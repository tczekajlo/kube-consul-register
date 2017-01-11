package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tczekajlo/kube-consul-register/config"
	"k8s.io/client-go/pkg/api/v1"
)

func TestPodInfoMethods(t *testing.T) {
	t.Parallel()

	var labels = make(map[string]string)
	var annotations = make(map[string]string)

	labels["app"] = "kubernetes"
	annotations["consul.register/enabled"] = "true"
	annotations["consul.register/service.name"] = "servicename"

	objPod := &v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			UID:         "01234567-89ab-cdef-0123-456789abcdef",
			Name:        "podname",
			Namespace:   "default",
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: v1.PodSpec{
			NodeName: "nodename",
		},
		Status: v1.PodStatus{
			Phase: "Running",
			PodIP: "127.0.0.1",
		},
	}

	var containerStatuses []v1.ContainerStatus
	containerStatus := v1.ContainerStatus{
		Name: "containername",
	}
	containerStatuses = append(containerStatuses, containerStatus)
	objPod.Status.ContainerStatuses = containerStatuses

	cfg := &config.Config{
		Controller: &config.ControllerConfig{
			K8sTag: "kubernetes",
		},
	}

	podInfo := &PodInfo{}
	podInfo.save(objPod)

	service, err := podInfo.PodToConsulService(containerStatus, cfg)
	assert.Error(t, err, "An error was expected")
	assert.Equal(t, "podname-containername", service.ID)
	assert.Contains(t, service.Tags, "kubernetes")

	isEnabledByAnnotation := podInfo.isRegisterEnabled()
	assert.Equal(t, true, isEnabledByAnnotation)
}
