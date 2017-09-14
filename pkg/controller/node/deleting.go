package node

import (
	"time"
	"encoding/json"

	"github.com/golang/glog"
	"github.com/kube-node/kube-machine/pkg/libmachine"
	nodehelper "github.com/kube-node/kube-machine/pkg/node"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	migrationCheckCount    = 100
	migrationCheckInterval = 100 * time.Millisecond
)

var pendingMigrationNodes = map[string]bool{}

func (c *Controller) syncDeletingNode(node *v1.Node) (changedN *v1.Node, err error) {
	if !nodehelper.HasFinalizer(node, deleteFinalizerName) {
		return nil, nil
	}

	changedN, err = c.deleteInstance(node)
	if err != nil || changedN != nil {
		return changedN, err
	}

	return nil, nil
}

func (c *Controller) deleteInstance(node *v1.Node) (*v1.Node, error) {
	for i, f := range node.Finalizers {
		if f == deleteFinalizerName {
			node.Finalizers = append(node.Finalizers[:i], node.Finalizers[i+1:]...)
			break
		}
	}

	if node.Annotations[driverDataAnnotationKey] == "" {
		return node, nil
	}

	go func() {
		deletedNode := &v1.Node{}
		*deletedNode = *node

		pendingMigrationNodes[deletedNode.Name] = true
		defer func() { delete(pendingMigrationNodes, deletedNode.Name) }()
		// Check for 30 seconds if a new node with the same name appeared.
		// In this case, migrate the node-controller labels&annotation to the new node
		// If a migration happened do not delete the instance at the cloud-provider
		for i := 0; i < migrationCheckCount; i++ {
			glog.Infof("Checking if a new node appeared after node %s got deleted", deletedNode.Name)
			time.Sleep(migrationCheckInterval)

			newNode, err := c.findRecreatedNode(deletedNode)
			if err != nil {
				if err != nodeNotFoundErr {
					glog.Errorf("Failed to fetch node %s during migration check: %v", deletedNode.Name, err)
				}
				continue
			}
			err = c.migrateNode(deletedNode, newNode)
			if err != nil {
				glog.Errorf("Failed to migrate node %s: %v", deletedNode.Name, err)
				continue
			}
			glog.Infof("Migrated node %s to %s", deletedNode.Name, newNode.Name)
			return
		}

		glog.Infof("No new node found for deleted node %s. Will delete it at cloud-provider", deletedNode.Name)

		mapi := libmachine.New()
		defer mapi.Close()

		h, err := mapi.Load(deletedNode)
		if err != nil {
			glog.Error(err)
		}

		err = h.Driver.Remove()
		if err != nil {
			glog.Error(err)
		}
	}()

	return node, nil
}

func (c *Controller) findRecreatedNode(deletedNode *v1.Node) (*v1.Node, error) {
	nlist := c.nodeIndexer.List()
	for _, obj := range nlist {
		node := obj.(*v1.Node)
		if node.UID == deletedNode.UID {
			continue
		}
		//Either if the name or the hostname label key is the same
		if node.Name == deletedNode.Name {
			glog.Infof("Found a matching new node via name comparison. Deleted: %q New: %q", deletedNode.Name, node.Name)
			return node, nil
		}
		if node.ObjectMeta.Labels[metav1.LabelHostname] != "" && node.ObjectMeta.Labels[metav1.LabelHostname] == deletedNode.ObjectMeta.Labels[metav1.LabelHostname] {
			glog.Infof("Found a matching new node via hostname-label comparison. Deleted: %q New: %q", deletedNode.ObjectMeta.Labels[metav1.LabelHostname], node.ObjectMeta.Labels[metav1.LabelHostname])
			return node, nil
		}
	}
	return nil, nodeNotFoundErr
}

func (c *Controller) migrateNode(deletedNode, currentNode *v1.Node) error {
	originalData, err := json.Marshal(currentNode)
	if err != nil {
		glog.Errorf("Failed to marshal node %s: %v", currentNode.Name, err)
		return nil
	}
	glog.Infof("Found a matching new node after %s got deleted. Migrating annotations & labels to new node %s", deletedNode.Name, currentNode.Name)

	currentNode.Labels[controllerLabelKey] = controllerName

	currentNode.Annotations[driverDataAnnotationKey] = deletedNode.Annotations[driverDataAnnotationKey]
	currentNode.Annotations[hostnameAnnotationKey] = deletedNode.Annotations[hostnameAnnotationKey]
	currentNode.Annotations[classAnnotationKey] = deletedNode.Annotations[classAnnotationKey]
	currentNode.Annotations[publicIPAnnotationKey] = deletedNode.Annotations[publicIPAnnotationKey]
	currentNode.Annotations[phaseAnnotationKey] = phaseRunning

	if !nodehelper.HasFinalizer(currentNode, deleteFinalizerName) {
		currentNode.Finalizers = append(currentNode.Finalizers, deleteFinalizerName)
	}

	return c.updateNode(originalData, currentNode)
}

func (c *Controller) waitUntilMigrationDone() {
	for {
		if len(pendingMigrationNodes) == 0 {
			return
		}
		glog.Infof("%d nodes are still pending for migration", len(pendingMigrationNodes))
		time.Sleep(migrationCheckInterval)
	}
}
