package pods

import (
	"fmt"
	"testing"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/assert"
	"github.com/tczekajlo/kube-consul-register/config"
	v1 "k8s.io/client-go/pkg/api/v1"
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
	// t.Parallel()

	testCases := []struct {
		desc  string
		probe v1.Probe
		pod   *PodInfo
	}{
		{
			"http probe",
			v1.Probe{
				Handler: v1.Handler{
					HTTPGet: &v1.HTTPGetAction{
						Scheme: "http",
						Path:   "/ping",
						Port:   intstr.IntOrString{IntVal: 8080},
					},
				},
			},
			&PodInfo{IP: "192.168.8.8"},
		},
		{
			"tcp probe",
			v1.Probe{
				Handler: v1.Handler{
					TCPSocket: &v1.TCPSocketAction{
						Port: intstr.IntOrString{IntVal: 5432},
					},
				},
			},
			&PodInfo{IP: "192.168.8.8"},
		},
		// apparently these are stripped
		{
			"exec probe",
			v1.Probe{
				Handler: v1.Handler{
					Exec: &v1.ExecAction{
						Command: []string{"some-command-to-check"},
					},
				},
			},
			&PodInfo{IP: "192.168.8.8"},
		},
	}

	for _, tc := range testCases {
		check := tc.pod.probeToConsulCheck(&tc.probe, "Liveness Probe")

		if check.Name != "Liveness Probe" {
			if tc.probe.Exec == nil {
				t.Errorf("[%s] wrong name: %s", tc.desc, check.Name)
			}
		}

		if tc.probe.HTTPGet != nil {
			assert.Equal(t, "http://192.168.8.8:8080/ping", check.HTTP)
		}

		if tc.probe.TCPSocket != nil {
			assert.Equal(t, "192.168.8.8:5432", check.TCP)
		}

		if tc.probe.Exec != nil {
			// emptyCheck := consulapi.AgentServiceCheck{}
			fmt.Println(check.Shell)
			// assert.Equal(t, emptyCheck, consulapi.AgentServiceCheck{})
		}
	}
}

func TestEmptyNoChecks(t *testing.T) {
	emptyCheck := consulapi.AgentServiceCheck{}
	podInfo := &PodInfo{IP: "192.168.8.8"}

	noProbeCheck := podInfo.probeToConsulCheck(nil, "Liveness Probe")

	assert.Equal(t, emptyCheck, *noProbeCheck)
}
