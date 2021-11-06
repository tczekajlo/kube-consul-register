# Kubernetes Consul Register [![Build Status](https://travis-ci.org/tczekajlo/kube-consul-register.svg?branch=master)](https://travis-ci.org/tczekajlo/kube-consul-register)
The kube-consul-register is a tool to register Kubernetes PODs as Consul Services.

kube-consul-register watches Kubernetes events and converts information about PODs to Consul Agent.

## Usage
```
  -alsologtostderr
        log to standard error as well as files
  -clean-interval duration
        time in seconds, what period of time will be done cleaning of inactive services (default 30m0s)
  -configmap string
        name of the ConfigMap that containes the custom configuration to use (default "default/kube-consul-register-config")
  -consul-secret string
        name of the secret containing the consul token, e.g. default/consul. Key must be consul_token
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
To store configuration is used [ConfigMap](https://github.com/kubernetes/kubernetes/blob/master/docs/design/configmap.md).
You can find [example of configuration](https://github.com/tczekajlo/kube-consul-register/blob/master/examples/config.yaml) with default values in examples directory.
In order to use ConfigMap configuration you've to use `configmap` flag. Value of this flag has format `namespace/configmap_name`, e.g. `-configmap="default/kube-consul-register-config"`.

| Option name | Default value | Description |
|-------------|---------------|-------------|
|`consul_address`|`localhost`| The address of Consul Agent. This option is taken into account only in case where `register_mode` is set to `single`|
|`consul_port`|`8500`| The port number of Consul Agent|
|`consul_scheme`|`http`| Connection scheme. Available options: `http`, `https`, `consul-unix`|
|`consul_ca_file`|| Path to a CA certificate file|
|`consul_cert_file`|| Path to an SSL client certificate to use to authenticate to the Consul server|
|`consul_key_file`|| Path to an SSL client certificate key to use to authenticate to the Consul server|
|`consul_insecure_skip_verify`|`false`| Skip verifying certificates when connecting via SSL|
|`consul_token`|| The Consul ACL token. Token is used to provide a per-request ACL token which overrides the agent's default token|
|`consul_timeout`|`2s`| Time limit for requests made by the Consul HTTP client. A Timeout of zero means no timeout|
|`consul_container_name`|`consul`| The name of container in POD with Consul Agent. The container with given name will be skip and not registered in Consul. This options is taken into account only if `register_mode` is set to `pod`|
|`consul_node_selector`|`consul=enabled`| Node label which is used to select nodes with Consul agent. This option is taken into account only if `register_mode` is equal to `node`|
|`pod_label_selector`|| Pay heed only to PODs with the given label |
|`k8s_tag`|`kubernetes`| The name of tag which is added to every Consul Service. This tag identifies all Consul Services which has been registered by kube-consul-register|
|`register_mode`|`single`| The mode of register. Available options: `single`, `pod`, `node`|
|`register_source`|`pod`| Source name which is watching in order to add services to Consul. Available options: `pod`, `service`, `endpoint`|

### Register mode
The `register_mode` option determine to which Consul Agent a services should be registered.
- `single` - registers all services in one agent. The address of agent is taken from `consul_address` option.
- `pod` - registers service in agent which is running as container is the same pod, as Consul Agent address is taken a IP address of pod.
- `node` - register service in agent which is running on the same node where service, as Consul Agent address is taken a name of node.

### Register source
`kube-consul-register` as default watches PODs and converts information about them into Consul Services, as alternative you can use Kubernetes Services or Endpoints.

In order to use Kubernetes Endpoints as source of information you have to set value of `register_source` option on `endpoints`, additionally you have to add annotation into specific endpoint.

```
# create service
kubectl expose deployment my-nginx --port=80 --type=LoadBalancer

# add annotation
kubectl annotate endpoints my-nginx consul.register/enabled=true
```

If you want to use Kubernetes Services you have to set value of `register_source` on `service`, only service with type `NodePort` is take into account. 

### Annotations
There are available annotations which can be used as pod's annotations.


|Name|Value|Description|
|----|-----|-----------|
|`consul.register/enabled`|`true`\|`false`|Determine if pod should be registered in Consul. This annotation is require in order to register pod as Consul service|
|`consul.register/service.name`|`service_name`|Determine name of service in Consul. If not given then is used the name of resource which created the POD. Only available if `register_source` is set on `pod`|
|`consul.register/service.meta.<key>`|`<value>`|Adds `key`/`value` service meta. Eg. `"consul.register/service.meta.redis_version"`=`"4.0"` results in meta `redis_version=4.0`|
|`consul.register/pod.container.name`|`container_name`|Container name or list of names (next name should be separated by comma) which will be taken into account. If omitted, all containers in POD will be registered|
|`consul.register/pod.container.probe.liveness`|`true`\|`false`|Use container `Liveness probe` for checks. Default is `true`.
|`consul.register/pod.container.probe.readiness`|`true`\|`false`|Use container `Readiness probe` for checks. Default is `false`|


The example of how to use annotation you can see [here](https://github.com/tczekajlo/kube-consul-register/blob/master/examples/nginx.yaml).

## Examples of usage
### Run out-of-cluster

```
$ kube-consul-register -logtostderr=true -kubeconfig=/my/kubeconfig -configmap="default/kube-consul-register" -in-cluster=false
```

### Run in-cluster

Example of usage in-cluster you can find [here](https://github.com/tczekajlo/kube-consul-register/blob/master/examples/rs.yaml). `kube-consul-register` is run as ReplicaSet.

## Metrics
Prometheus metrics are available by `/metrics` endpoint on `:8080` address.
