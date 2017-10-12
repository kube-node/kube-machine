package node

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/kube-node/kube-machine/pkg/controller"
	"github.com/kube-node/kube-machine/pkg/nodeclass"
	"github.com/kube-node/nodeset/pkg/nodeset/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type Controller struct {
	nodeInformer         cache.Controller
	nodeIndexer          cache.Indexer
	nodeQueue            workqueue.RateLimitingInterface
	nodeClassStore       cache.Store
	nodeClassInformer    cache.Controller
	client               *kubernetes.Clientset
	nodeCreateLock       *sync.Mutex
	maxMigrationWaitTime time.Duration
	metrics              *ControllerMetrics
}

const (
	classAnnotationKey      = "node.k8s.io/node-class"
	phaseAnnotationKey      = "node.k8s.io/state"
	driverDataAnnotationKey = "node.k8s.io/driver-data"
	deleteFinalizerName     = "node.k8s.io/delete"
	publicIPAnnotationKey   = "node.k8s.io/public-ip"
	hostnameAnnotationKey   = "node.k8s.io/hostname"

	controllerLabelKey = "node.k8s.io/controller"
	controllerName     = "kube-machine"

	phasePending      = "pending"
	phaseProvisioning = "provisioning"
	phaseLaunching    = "launching"
	phaseRunning      = "running"
	phaseDeleting     = "deleting"

	conditionUpdatePeriod = 5 * time.Second
	migrationWorkerPeriod = 5 * time.Second
)

var nodeClassNotFoundErr = errors.New("node class not found")
var nodeNotFoundErr = errors.New("node not found")

func New(
	client *kubernetes.Clientset,
	queue workqueue.RateLimitingInterface,
	nodeIndexer cache.Indexer,
	nodeInformer cache.Controller,
	nodeClassStore cache.Store,
	nodeClassController cache.Controller,
	maxMigrationWaitTime time.Duration,
	metrics *ControllerMetrics,
) controller.Interface {
	return &Controller{
		nodeInformer:         nodeInformer,
		nodeIndexer:          nodeIndexer,
		nodeQueue:            queue,
		nodeClassInformer:    nodeClassController,
		nodeClassStore:       nodeClassStore,
		client:               client,
		nodeCreateLock:       &sync.Mutex{},
		maxMigrationWaitTime: maxMigrationWaitTime,
		metrics:              metrics,
	}
}

func (c *Controller) processNextItem() bool {
	// Wait until there is a new item in the working nodeQueue
	key, quit := c.nodeQueue.Get()
	if quit {
		return false
	}

	defer c.nodeQueue.Done(key)

	err := c.syncNode(key.(string))
	if err != nil {
		c.metrics.SyncErrors.Inc()
	}

	c.handleErr(err, key)
	return true
}

func (c *Controller) getNode(key string) (*corev1.Node, error) {
	node, err := c.client.CoreV1().Nodes().Get(key, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (c *Controller) syncNode(key string) error {
	node, err := c.getNode(key)
	if err != nil {
		glog.V(0).Infof("Failed to fetch node %s: %v", key, err)
		return nil
	}

	if node.Labels[controllerLabelKey] != controllerName {
		return nil
	}

	originalData, err := json.Marshal(node)
	if err != nil {
		glog.V(0).Infof("Failed marshal node %s: %v", key, err)
		return nil
	}

	glog.V(6).Infof("Processing Node %s\n", node.GetName())

	// Get phase of node. In case we have not touched it set phase to `pending`
	phase := node.Annotations[phaseAnnotationKey]
	if phase == "" {
		phase = phasePending
	}

	if node.DeletionTimestamp != nil {
		phase = phaseDeleting
	}
	node.Annotations[phaseAnnotationKey] = phase

	start := time.Now()

	switch phase {
	case phasePending:
		node, err = c.syncPendingNode(node)
	case phaseProvisioning:
		node, err = c.syncProvisioningNode(node)
	case phaseLaunching:
		node, err = c.syncLaunchingNode(node)
	case phaseDeleting:
		node, err = c.syncDeletingNode(node)
	}

	if phase != phaseRunning {
		c.metrics.SyncSeconds.WithLabelValues(phase).Add(time.Since(start).Seconds())
	}

	if err != nil {
		return err
	}

	if node != nil {
		return c.updateNode(originalData, node)
	}

	c.nodeQueue.AddAfter(key, 30*time.Second)
	return nil
}

func (c *Controller) getNodeClassConfig(nc *v1alpha1.NodeClass) (*nodeclass.NodeClassConfig, error) {
	var config nodeclass.NodeClassConfig
	err := json.Unmarshal(nc.Config.Raw, &config)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal config from nodeclass: %v", err)
	}
	return &config, nil
}

func (c *Controller) getNodeClassFromAnnotationContent(node *corev1.Node) (*v1alpha1.NodeClass, *nodeclass.NodeClassConfig, error) {
	content, err := base64.StdEncoding.DecodeString(node.Annotations[v1alpha1.NodeClassContentAnnotationKey])
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load nodeclass content from annotation %s: %v", v1alpha1.NodeClassContentAnnotationKey, err)
	}
	class := &v1alpha1.NodeClass{}
	err = json.Unmarshal(content, class)
	if err != nil {
		return nil, nil, fmt.Errorf("could not unmarshal nodeclass from annotation %s content: %v", v1alpha1.NodeClassContentAnnotationKey, err)
	}
	config, err := c.getNodeClassConfig(class)
	return class, config, err
}

func (c *Controller) getNodeClassFromAnnotation(node *corev1.Node) (*v1alpha1.NodeClass, *nodeclass.NodeClassConfig, error) {
	ncobj, exists, err := c.nodeClassStore.GetByKey(node.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("could not fetch nodeclass from store: %v", err)
	}
	if !exists {
		return nil, nil, nodeClassNotFoundErr
	}

	class := ncobj.(*v1alpha1.NodeClass)
	config, err := c.getNodeClassConfig(class)
	return class, config, err
}

func (c *Controller) getNodeClass(node *corev1.Node) (*v1alpha1.NodeClass, *nodeclass.NodeClassConfig, error) {
	//First try to load it via annotation
	if node.Annotations[v1alpha1.NodeClassContentAnnotationKey] != "" {
		return c.getNodeClassFromAnnotationContent(node)
	}
	return c.getNodeClassFromAnnotation(node)
}

func (c *Controller) updateNode(originalData []byte, node *corev1.Node) error {
	modifiedData, err := json.Marshal(node)
	if err != nil {
		return err
	}

	b, err := strategicpatch.CreateTwoWayMergePatch(originalData, modifiedData, corev1.Node{})
	if err != nil {
		return err
	}
	//Avoid empty patch calls
	if string(b) == "{}" {
		return nil
	}

	_, err = c.client.CoreV1().Nodes().Patch(node.Name, types.StrategicMergePatchType, b)
	return err
}

// handleErr checks if an error happened and makes sure we will retry later.
func (c *Controller) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.nodeQueue.Forget(key)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if c.nodeQueue.NumRequeues(key) < 5 {
		glog.V(0).Infof("Error syncing node %v: %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// nodeQueue and the re-enqueue history, the key will be processed later again.
		c.nodeQueue.AddRateLimited(key)
		return
	}

	c.nodeQueue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	runtime.HandleError(err)
	glog.V(0).Infof("Dropping node %q out of the queue: %v", key, err)
}

func (c *Controller) Run(workerCount int, stopCh chan struct{}) {
	defer runtime.HandleCrash()

	// Let the workers stop when we are done
	defer c.nodeQueue.ShutDown()
	glog.V(0).Info("Starting Node controller")

	go c.nodeInformer.Run(stopCh)
	go c.nodeClassInformer.Run(stopCh)

	// Wait for all involved caches to be synced, before processing items from the nodeQueue is started
	if !cache.WaitForCacheSync(stopCh, c.nodeInformer.HasSynced, c.nodeClassInformer.HasSynced) {
		runtime.HandleError(errors.New("timed out waiting for caches to sync"))
		return
	}

	go wait.Forever(func() {
		c.metrics.Nodes.Set(float64(len(c.nodeIndexer.List())))
	}, time.Second)

	for i := 0; i < workerCount; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}
	go wait.Forever(c.readyConditionWorker, conditionUpdatePeriod)
	go wait.Forever(c.migrationWorker, migrationWorkerPeriod)

	<-stopCh
	glog.V(0).Info("Stopping Node controller")
	glog.V(0).Info("Waiting until all pending migrations are done...")
	c.waitUntilMigrationDone()
	glog.V(0).Info("Done")
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

func (c *Controller) IsReady() bool {
	return c.nodeInformer.HasSynced() && c.nodeClassInformer.HasSynced()
}
