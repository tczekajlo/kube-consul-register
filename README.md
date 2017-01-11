# Kubernetes Consul Register
The kube-consul-register is a tool to register Kubernetes PODs as Consul Services.

kube-consul-register watches Kubernetes events and converts information about PODs to Consul Agent.

## Usage
```
  -alsologtostderr
        log to standard error as well as files
  -clean-interval duration
        time in seconds, what period of time will be done cleaning of inactive services (default 30m0s)
  -configmap string
        name of the ConfigMap that containes the custom configuration to use
  -in-cluster
        use in-cluster config. Use always in case when controller is running on Kubernetes cluster (default true)
  -kubeconfig string
        absolute path to the kubeconfig file (default "./kubeconfig")
  -log_backtrace_at value
        when logging hits line file:N, emit a stack trace
  -log_dir string
        If non-empty, write log files in this directory
  -logtostderr
        log to standard error instead of files
  -stderrthreshold value
        logs at or above this threshold go to stderr
  -sync-interval duration
        time in seconds, what period of time will be done synchronization (default 2m0s)
  -v value
        log level for V logs
  -version
        print version end exit
  -vmodule value
        comma-separated list of pattern=N settings for file-filtered logging
  -watch-namespace string
        namespace to watch for Pods. Default is to watch all namespaces
```

## Configuration
The store configuration is uses [ConfigMap](https://github.com/kubernetes/kubernetes/blob/master/docs/design/configmap.md).
You can find [example of configuration](https://github.com/tczekajlo/kube-consul-register/blob/master/examples/config.yaml) with default values in examples directory.
In order to use ConfigMap configuration you've to use `configmap` flag. Value of this flag has format `namespace/configmap_nam`, e.g. `-configmap="default/kube-consul-register-config"`.

| Option name | Default value | Description |
|-------------|---------------|-------------|
|`consul_address`|`localhost`| The address of Consul Agent. This option is taken into account only in case where `register_mode` is set to `single`|
|`consul_port`|`8500`| The port number of Consul Agent|
|`consul_scheme`|`http`| Connection scheme. Available options: `http`, `https`, `consul-unix`|
|`consul_ca_file`|| Path to a CA certificate file|
|`consul_cert_file`|| Path to an SSL client certificate to use to authenticate to the Consul server|
|`consul_key_file`|| Path to an SSL client certificate key to use to authenticate to the Consul server|
|`consul_insecure_skip_verify`|`false`| Skip verifying certificates when connecting via SSL|
|`consul_container_name`|`consul`| The name of container in POD with Consul Agent. The container with given name will be skip and not registered in Consul. This options is taken into account only if `register_mode` is set to `pod`|
|`k8s_tag`|`kubernetes`| The name of tag which is added to every Consul Service. This tag idetifies all Consul Serivces which has been registered by kube-consul-register|
|`register_mode`|`single`| The mode of register. Available options: `single`, `pod`, `node`|

### Register mode
The `register_mode` option determine to which Consul Agent a services should be registered.
- `single` - registers all services in one agent. The address of agent is taken from `consul_address` option.
- `pod` - registers service in agent which is running as container is the same pod, as Consul Agent address is taken a IP address of pod.
- `node` - register service in agent which is running on the same node where service, as Consul Agent address is taken a name of node.

### Annotations
There are available annotations which can be used as pod's annotations.
|Name|Value|Description|
|----|-----|-----------|
|`consul.regiser/enabled`|`true`\|`false`|Determine if pod should be registered in Consul. This annotation is require in order to register pod as Consul service|
|`consul.regiser/service.name`|`service_name`|Determine name of service in Consul. If not given then is used the name of resource which created the POD|

The example of how to use annotation you can see [here](https://github.com/tczekajlo/kube-consul-register/blob/master/examples/nginx.yaml).

## Examples of usage
Run out-of-cluster

```
$ kube-consul-register -logtostderr=true -kubeconfig=/my/kubeconfig -configmap="default/kube-consul-register" -in-cluster=false
```
