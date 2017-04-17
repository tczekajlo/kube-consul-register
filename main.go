package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tczekajlo/kube-consul-register/consul"
	"github.com/tczekajlo/kube-consul-register/metrics"
	"github.com/tczekajlo/kube-consul-register/utils"

	"github.com/golang/glog"

	"github.com/tczekajlo/kube-consul-register/config"
	"github.com/tczekajlo/kube-consul-register/controller"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	// VERSION is filled out during the build process (using git describe output)
	VERSION string

	cfg   *config.Config
	mutex = &sync.Mutex{}

	watchNamespace       = flag.String("watch-namespace", v1.NamespaceAll, "namespace to watch for Pods. Default is to watch all namespaces")
	kubeconfig           = flag.String("kubeconfig", "./kubeconfig", "absolute path to the kubeconfig file")
	configMap            = flag.String("configmap", "default/kube-consul-register-config", "name of the ConfigMap that containes the custom configuration to use")
	inClusterConfig      = flag.Bool("in-cluster", true, "use in-cluster config. Use always in case when controller is running on Kubernetes cluster")
	syncInterval         = flag.Duration("sync-interval", 120*time.Second, "time in seconds, what period of time will be done synchronization")
	cleanInterval        = flag.Duration("clean-interval", 1800*time.Second, "time in seconds, what period of time will be done cleaning of inactive services")
	metricsListenAddress = flag.String("metrics-listen-address", ":8080", "the address to listen on for HTTP requests.")
	versionFlag          = flag.Bool("version", false, "print version end exit")
)

func init() {
	// Metrics have to be registered to be exposed
	prometheus.MustRegister(metrics.ConsulFailure)
	prometheus.MustRegister(metrics.ConsulSuccess)
	prometheus.MustRegister(metrics.PodFailure)
	prometheus.MustRegister(metrics.PodSuccess)
	prometheus.MustRegister(metrics.FuncDuration)
}

func main() {
	flag.Parse()

	if *versionFlag {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	glog.Infof("Using build: %v", VERSION)

	var err error
	var kubeClientConfig *rest.Config
	if *inClusterConfig {
		// creates the in-cluster config
		kubeClientConfig, err = rest.InClusterConfig()
	} else {
		// uses the current context in kubeconfig
		kubeClientConfig, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	}
	if err != nil {
		glog.Fatalf("Error configuring the client: %v", err.Error())
	}
	// creates the clientset for Kubernetes
	clientset, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		glog.Fatalf("Failed to create Kubernetes client: %v", err.Error())
	}

	if *configMap != "" {
		namespace, name, err := utils.ParseNsName(*configMap)
		if err != nil {
			glog.Fatalf("ConfigMap: %v", err)
		}

	load_config:
		cfg, err = config.Load(clientset, namespace, name)
		if err != nil {
			glog.Errorf("Unable to load configuration: %v", err)
			time.Sleep(10 * time.Second)
			goto load_config
		}

		glog.Infof("Current configuration: Controller: %#v, Consul: %#v", cfg.Controller, cfg.Consul)
	}

	//Consul instance
	consulInstance := consul.Adapter{}

	//Controller instance
	ctrInstance := controller.Factory{}
	ctr := ctrInstance.New(clientset, consulInstance, cfg, *watchNamespace)

	//Cleaning
	go func() {
		for {
			mutex.Lock()
			glog.Info("Start cleaning...")
			err := ctr.Clean()
			if err != nil {
				glog.Errorf("Unable to cleaning to inactive services: %s", err)
			} else {
				glog.Info("Cleaning has been ended")
			}
			mutex.Unlock()
			time.Sleep(*cleanInterval)
		}
	}()

	//Syncing
	go func() {
		for {
			mutex.Lock()
			glog.Info("Start syncing...")
			err := ctr.Sync()
			if err != nil {
				glog.Errorf("Unable to syncing: %s", err)
			} else {
				glog.Info("Synchronization's been ended")
			}
			mutex.Unlock()
			time.Sleep(*syncInterval)
		}
	}()

	go func() {
		ctr.Watch()
	}()

	go handleSigterm()

	// Metrics
	http.Handle("/metrics", promhttp.Handler())
	glog.Fatal(http.ListenAndServe(*metricsListenAddress, nil))
}

func handleSigterm() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM)
	<-signalChan
	glog.Infof("Received SIGTERM, shutting down")

	exitCode := 0

	glog.Infof("Exiting with %v", exitCode)
	os.Exit(exitCode)
}
