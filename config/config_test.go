package config

import (
	"testing"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/assert"
)

func TestFillConfigDefaults(t *testing.T) {
	t.Parallel()

	var data = make(map[string]string)

	a := assert.New(t)

	cfg := &Config{}
	obj, err := cfg.fillConfig(data)

	if !a.IsType(&Config{}, obj) {
		t.Errorf("fillConfig returns wrong type. Expected *Config, got: %v", obj)
	}

	if !a.IsType(&consulapi.Config{}, cfg.Consul) {
		t.Errorf("fillConfig returns wrong type. Expected *consulapi.Config, got: %v", obj)
	}

	if !a.IsType(&ControllerConfig{}, cfg.Controller) {
		t.Errorf("fillConfig returns wrong type. Expected *ControllerConfig, got: %v", obj)
	}

	assert.Nil(t, err, "err should be nothing")

	assert.Equal(t, cfg.Controller.ConsulAddress, "localhost", "wrong default value for `consul_address` option")
	assert.Equal(t, cfg.Controller.ConsulPort, "8500", "wrong default value for `consul_port` option")
	assert.Equal(t, cfg.Controller.ConsulScheme, "http", "wrong default value for `consul_scheme` option")
	assert.Equal(t, cfg.Controller.ConsulCAFile, "", "wrong default value for `consul_ca_file` option")
	assert.Equal(t, cfg.Controller.ConsulKeyFile, "", "wrong default value for `consul_key_file` option")
	assert.Equal(t, cfg.Controller.ConsulCertFile, "", "wrong default value for `consul_cert_file` option")
	assert.Equal(t, cfg.Controller.ConsulInsecureSkipVerify, false, "wrong default value for `consul_insecure_skip_verify` option")
	assert.Equal(t, cfg.Controller.ConsulContainerName, "consul", "wrong default value for `consul_container_name` option")
	assert.Equal(t, cfg.Controller.K8sTag, "kubernetes", "wrong default value for `k8s_tag` option")
	assert.Equal(t, cfg.Controller.RegisterMode, RegisterSingleMode, "wrong default value for `register_mode` option")
}

func TestFillConfig(t *testing.T) {
	t.Parallel()

	var data = make(map[string]string)
	cfg := &Config{}

	//fill data
	data["consul_address"] = "domain"
	data["consul_port"] = "8000"
	data["consul_scheme"] = "https"
	data["consul_ca_file"] = "ca.pem"
	data["consul_key_file"] = "key.pem"
	data["consul_cert_file"] = "cert.pem"
	data["consul_insecure_skip_verify"] = "true"
	data["consul_container_name"] = "name"
	data["k8s_tag"] = "k8s"
	data["register_mode"] = "node"

	cfg.fillConfig(data)

	assert.Equal(t, cfg.Controller.ConsulAddress, "domain", "they should be equal")
	assert.Equal(t, cfg.Controller.ConsulPort, "8000", "they should be equal")
	assert.Equal(t, cfg.Controller.ConsulScheme, "https", "they should be equal")
	assert.Equal(t, cfg.Controller.ConsulCAFile, "ca.pem", "they should be equal")
	assert.Equal(t, cfg.Controller.ConsulKeyFile, "key.pem", "they should be equal")
	assert.Equal(t, cfg.Controller.ConsulCertFile, "cert.pem", "they should be equal")
	assert.Equal(t, cfg.Controller.ConsulInsecureSkipVerify, true, "they should be equal")
	assert.Equal(t, cfg.Controller.ConsulContainerName, "name", "they should be equal")
	assert.Equal(t, cfg.Controller.K8sTag, "k8s", "they should be equal")
	assert.Equal(t, cfg.Controller.RegisterMode, RegisterNodeMode, "they should be equal")

	data["register_mode"] = "pod"
	cfg.fillConfig(data)
	assert.Equal(t, cfg.Controller.RegisterMode, RegisterPodMode, "they should be equal")

	data["register_mode"] = "single"
	cfg.fillConfig(data)
	assert.Equal(t, cfg.Controller.RegisterMode, RegisterSingleMode, "they should be equal")

	data["register_mode"] = "node"
	cfg.fillConfig(data)
	assert.Equal(t, cfg.Controller.RegisterMode, RegisterNodeMode, "they should be equal")

	data["consul_insecure_skip_verify"] = "not_bool"
	_, err := cfg.fillConfig(data)
	assert.Error(t, err, "An error was expected")
}
