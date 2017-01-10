package utils

import (
	"fmt"
	"strings"
)

// ParseNsName parses input and returns namespace name and ConfigMap name.
func ParseNsName(input string) (string, string, error) {
	nsName := strings.Split(input, "/")
	if len(nsName) != 2 {
		return "", "", fmt.Errorf("invalid format (namespace/name) found in '%v'", input)
	}

	return nsName[0], nsName[1], nil
}

// CheckK8sTag checks whether Consul service' tags contains
// tag which is given as value in `k8s_tag` option.
func CheckK8sTag(tags []string, k8sTag string) bool {
	for _, tag := range tags {
		if tag == k8sTag {
			return true
		}
	}
	return false
}
