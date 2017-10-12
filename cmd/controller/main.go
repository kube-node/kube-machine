package main

import (
	goflag "flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	dlog "github.com/docker/machine/libmachine/log"
	"github.com/docker/machine/libmachine/ssh"
	"github.com/golang/glog"
	"github.com/kube-node/kube-machine/pkg/controller"
	"github.com/kube-node/kube-machine/pkg/controller/node"
	"github.com/kube-node/kube-machine/pkg/nodeclass"
	nodesetclient "github.com/kube-node/nodeset/pkg/client/clientset/versioned"
	"github.com/kube-node/nodeset/pkg/nodeset/v1alpha1"
	flag "github.com/spf13/pflag"

	"k8s.io/api/core/v1"
	extapiclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
)

var kubeconfig *string = flag.String("kubeconfig", "", "Path to kubeconfig file with authorization and master location information.")
var master *string = flag.String("master", "", "The address of the Kubernetes API server (overrides any value in kubeconfig)")
var healthListenAddress *string = flag.String("health-listen-address", ":8081", "The listen address for health checking")
var maxMigrationWaitSeconds *int = flag.Int("max-migration-wait-seconds", 20, "Maximum time to wait for a migration until a deleted node gets deleted at cloud-provider. A migration happens if the actual kubelet registers with a different name than specified in the node resource OR when the kubelet deletes the existing node and recreates it(happens on every cloud-provider)")
var promAddr *string = flag.String("prometheus", ":8082", "The address for Prometheus")

const (
	workerCount = 25
)

func main() {
	flag.CommandLine.AddGoFlagSet(goflag.CommandLine)
	flag.Parse()
	dlog.SetDebug(true)

	metrics := node.NewControllerMetrics()
	go metrics.Serve(*promAddr)

	var config *rest.Config
	var err error

	glog.V(6).Infof("Using local kubeconfig located at %q", *kubeconfig)
	config, err = clientcmd.BuildConfigFromFlags(*master, *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	kubeClient := kubernetes.NewForConfigOrDie(config)
	apiextensionsclientset := extapiclient.NewForConfigOrDie(config)

	err = nodeclass.EnsureCustomResourceDefinitions(apiextensionsclientset)
	if err != nil {
		panic(err)
	}

	nodesetClient := nodesetclient.NewForConfigOrDie(config)

	nodeQueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	nodeIndexer, nodeInformer := cache.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kubeClient.CoreV1().Nodes().List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kubeClient.CoreV1().Nodes().Watch(options)
			},
		},
		&v1.Node{},
		5*time.Minute,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				key, err := cache.MetaNamespaceKeyFunc(obj)
				if err == nil {
					nodeQueue.Add(key)
				}
			},
			UpdateFunc: func(old interface{}, new interface{}) {
				key, err := cache.MetaNamespaceKeyFunc(new)
				if err == nil {
					nodeQueue.Add(key)
				}
			},
			DeleteFunc: func(obj interface{}) {
				// IndexerInformer uses a delta nodeQueue, therefore for deletes we have to use this
				// key function.
				key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
				if err == nil {
					nodeQueue.Add(key)
				}
			},
		},
		cache.Indexers{},
	)

	nodeClassStore, nodeClassController := cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return nodesetClient.NodesetV1alpha1().NodeClasses().List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return nodesetClient.NodesetV1alpha1().NodeClasses().Watch(options)
			},
		},
		&v1alpha1.NodeClass{},
		5*time.Minute,
		cache.ResourceEventHandlerFuncs{},
	)

	//Is default on docker-machine. Lets stick to defaults.
	ssh.SetDefaultClient(ssh.External)

	c := node.New(
		kubeClient,
		nodeQueue,
		nodeIndexer,
		nodeInformer,
		nodeClassStore,
		nodeClassController,
		time.Duration(*maxMigrationWaitSeconds)*time.Second,
		metrics)

	stop := make(chan struct{})
	osc := make(chan os.Signal, 2)
	signal.Notify(osc, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-osc
		close(stop)
	}()

	go startHealth(c)
	c.Run(workerCount, stop)
}

func startHealth(c controller.Interface) {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if c.IsReady() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Not ready"))
		}
	})
	log.Fatal(http.ListenAndServe(*healthListenAddress, nil))
}
