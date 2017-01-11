package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/types"
)

func TestPodInfoSave(t *testing.T) {
	t.Parallel()

	var labels = make(map[string]string)
	var annotations = make(map[string]string)

	labels["app"] = "kubernetes"
	annotations["consul.register/enabled"] = "true"

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

	podInfo := &PodInfo{}
	podInfo.save(objPod)

	assert.Equal(t, types.UID("01234567-89ab-cdef-0123-456789abcdef"), podInfo.UID)
	assert.Equal(t, "podname", podInfo.Name)
	assert.Equal(t, "default", podInfo.Namespace)
	assert.Equal(t, "kubernetes", podInfo.Labels["app"])
	assert.Equal(t, "true", podInfo.Annotations["consul.register/enabled"])
	assert.Equal(t, "nodename", podInfo.NodeName)
	assert.Equal(t, v1.PodPhase("Running"), podInfo.Phase)
	assert.Equal(t, "127.0.0.1", podInfo.IP)
}
