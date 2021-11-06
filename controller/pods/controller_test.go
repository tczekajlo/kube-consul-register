package pods

import (
	"testing"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/assert"
	"github.com/tczekajlo/kube-consul-register/config"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/util/intstr"
)

func TestPodInfoMethods(t *testing.T) {
	t.Parallel()

	var labels = make(map[string]string)
	var annotations = make(map[string]string)

	labels["app"] = "kubernetes"
	labels["production"] = "tag"
	annotations["consul.register/enabled"] = "true"
	annotations["consul.register/service.name"] = "servicename"
	annotations["consul.register/service.meta.abc"] = "123"
	annotations["consul.register/service.meta.XYZ"] = "790_0"

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
	assert.Contains(t, service.Tags, "production")
	assert.Contains(t, service.Tags, "pod:podname")
	assert.Contains(t, service.Tags, "podname")
	assert.Equal(t, service.Meta["abc"], "123")
	assert.Equal(t, service.Meta["XYZ"], "790_0")

	isEnabledByAnnotation := podInfo.isRegisterEnabled()
	assert.Equal(t, true, isEnabledByAnnotation)
}

func TestProbeToConsulCheck(t *testing.T) {
	t.Parallel()
	emptyCheck := consulapi.AgentServiceCheck{}

	podInfo := &PodInfo{IP: "192.168.8.8"}

	httpProbe := &v1.Probe{
		Handler: v1.Handler{
			HTTPGet: &v1.HTTPGetAction{
				Scheme: "http",
				Path:   "/ping",
				Port:   intstr.IntOrString{IntVal: 8080},
			},
		},
	}

	tcpProbe := &v1.Probe{
		Handler: v1.Handler{
			TCPSocket: &v1.TCPSocketAction{
				Port: intstr.IntOrString{IntVal: 5432},
			},
		},
	}

	execProbe := &v1.Probe{
		Handler: v1.Handler{
			Exec: &v1.ExecAction{
				Command: []string{"some-command-to-check"},
			},
		},
	}

	httpCheck := podInfo.probeToConsulCheck(httpProbe, "Liveness Probe")
	tcpCheck := podInfo.probeToConsulCheck(tcpProbe, "Liveness Probe")
	noProbeCheck := podInfo.probeToConsulCheck(nil, "Liveness Probe")
	execCheck := podInfo.probeToConsulCheck(execProbe, "Liveness Probe")

	assert.Equal(t, "Liveness Probe", httpCheck.Name)
	assert.Equal(t, "http://192.168.8.8:8080/ping", httpCheck.HTTP)
	assert.Equal(t, "192.168.8.8:5432", tcpCheck.TCP)
	assert.Equal(t, emptyCheck, *noProbeCheck)
	assert.Equal(t, emptyCheck, *execCheck)
}
