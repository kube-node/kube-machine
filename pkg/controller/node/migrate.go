package node

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang/glog"
	nodehelper "github.com/kube-node/kube-machine/pkg/node"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	LabelHostname = "kubernetes.io/hostname"
)

func (c *Controller) migrateNode(srcNode, targetNode *v1.Node) error {
	originalData, err := json.Marshal(targetNode)
	if err != nil {
		glog.V(0).Infof("Failed to marshal node %s: %v", targetNode.Name, err)
		return nil
	}
	glog.V(4).Infof("Found a matching new node after %s got deleted. Migrating annotations & labels to new node %s", srcNode.Name, targetNode.Name)


	for k, v := range srcNode.Annotations {
		targetNode.Annotations[k] = v
	}
	for k, v := range srcNode.Labels {
		targetNode.Labels[k] = v
	}
	// If we migrate the node we need to set phase to running.
	targetNode.Annotations[phaseAnnotationKey] = phaseRunning

	if !nodehelper.HasFinalizer(targetNode, deleteFinalizerName) {
		targetNode.Finalizers = append(targetNode.Finalizers, deleteFinalizerName)
	}

	return c.updateNode(originalData, targetNode)
}

func (c *Controller) waitUntilMigrationDone() {
	for {
		if len(pendingMigrationNodes) == 0 {
			return
		}
		glog.V(2).Infof("%d nodes are still pending for migration", len(pendingMigrationNodes))
		time.Sleep(migrationCheckInterval)
	}
}

// findForeignSibling returns a node which is the sibling of the given node
// Necessary in case the kubelet deletes the node-controller managed node & creates a new one...
// Then we need to migrate.
func (c *Controller) findForeignSibling(node *v1.Node) (*v1.Node, error) {
	nlist := c.nodeIndexer.List()
	for _, obj := range nlist {
		candidate := obj.(*v1.Node)
		if candidate.UID == node.UID {
			continue
		}

		isControllerNode, err := c.isControllerNode(candidate)
		if err != nil {
			glog.V(0).Infof("failed to identify if node %s belongs to this controller: %v", node.Name, err)
			continue
		}
		//We really just want nodes which are not controlled by this controller
		if isControllerNode {
			continue
		}

		//Either if the name or the hostname label key is the same
		if candidate.Name == node.Name {
			glog.V(6).Infof("Found a matching node via name comparison. Deleted: %q New: %q", node.Name, candidate.Name)
			return candidate, nil
		}
		if candidate.ObjectMeta.Labels[LabelHostname] != "" && candidate.ObjectMeta.Labels[LabelHostname] == node.ObjectMeta.Labels[LabelHostname] {
			glog.V(6).Infof("Found a matching node via hostname-label comparison. Deleted: %q New: %q", node.ObjectMeta.Labels[LabelHostname], candidate.ObjectMeta.Labels[LabelHostname])
			return candidate, nil
		}
	}
	return nil, nodeNotFoundErr
}

func (c *Controller) isControllerNode(node *v1.Node) (bool, error) {
	class, _, err := c.getNodeClass(node)
	if err != nil {
		if err == noNodeClassDefinedErr {
			return false, nil
		}
		return false, fmt.Errorf("failed to get nodeclass for node %s: %v", node.Name, err)
	}

	return class.NodeController == controllerName, nil
}

func (c *Controller) migrationWorker() {
	nlist := c.nodeIndexer.List()
	for _, obj := range nlist {
		node := obj.(*v1.Node)
		//Only check for nodes for this controller
		isControllerNode, err := c.isControllerNode(node)
		if err != nil {
			glog.V(0).Infof("failed to identify if node %s belongs to this controller: %v", node.Name, err)
			continue
		}
		if !isControllerNode {
			glog.V(8).Infof("Skipping node %s as the specified node-controller != %s", node.Name, controllerName)
			continue
		}

		glog.V(8).Infof("Processing node %s for migration check", node.Name)

		sibling, err := c.findForeignSibling(node)
		if err != nil {
			if err != nodeNotFoundErr {
				glog.V(0).Infof("Failed to find a sibling for %s: %v", node.Name, err)
			}
			continue
		}
		if node.ObjectMeta.CreationTimestamp.Before(&sibling.ObjectMeta.CreationTimestamp) {
			glog.V(4).Infof("Found a suitable sibling for node %s to migrate. Deleting now %s to trigger the migration", node.Name, node.Name)
			err := c.client.CoreV1().Nodes().Delete(node.Name, &metav1.DeleteOptions{})
			if err != nil {
				glog.V(0).Infof("Failed to delete node %s for migration: %v", node.Name, err)
			}
		} else {
			continue
		}
	}
}

func (c *Controller) deleteMigrationWatcher(node *v1.Node) wait.ConditionFunc {
	return func() (done bool, err error) {
		newNode, err := c.findForeignSibling(node)
		if err != nil {
			if err != nodeNotFoundErr {
				glog.V(0).Infof("Failed to fetch node %s during migration check: %v", node.Name, err)
			}
			return false, nil
		}
		err = c.migrateNode(node, newNode)
		if err != nil {
			glog.V(0).Infof("Failed to migrate node %s: %v", node.Name, err)
			return false, nil
		}
		glog.V(4).Infof("Migrated node %s to %s", node.Name, newNode.Name)
		delete(node.Annotations, driverDataAnnotationKey)
		return true, nil
	}
}
