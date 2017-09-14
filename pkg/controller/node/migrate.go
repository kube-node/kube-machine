package node

import (
	"encoding/json"
	"time"

	"github.com/golang/glog"
	nodehelper "github.com/kube-node/kube-machine/pkg/node"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/pkg/api/v1"
)

func (c *Controller) migrateNode(srcNode, targetNode *v1.Node) error {
	originalData, err := json.Marshal(targetNode)
	if err != nil {
		glog.V(0).Infof("Failed to marshal node %s: %v", targetNode.Name, err)
		return nil
	}
	glog.V(4).Infof("Found a matching new node after %s got deleted. Migrating annotations & labels to new node %s", srcNode.Name, targetNode.Name)

	targetNode.Labels[controllerLabelKey] = controllerName

	targetNode.Annotations[driverDataAnnotationKey] = srcNode.Annotations[driverDataAnnotationKey]
	targetNode.Annotations[hostnameAnnotationKey] = srcNode.Annotations[hostnameAnnotationKey]
	targetNode.Annotations[classAnnotationKey] = srcNode.Annotations[classAnnotationKey]
	targetNode.Annotations[publicIPAnnotationKey] = srcNode.Annotations[publicIPAnnotationKey]
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

func (c *Controller) findSibling(node *v1.Node) (*v1.Node, error) {
	nlist := c.nodeIndexer.List()
	for _, obj := range nlist {
		candidate := obj.(*v1.Node)
		if candidate.UID == node.UID {
			continue
		}
		if candidate.Annotations[controllerLabelKey] == controllerName {
			continue
		}

		//Either if the name or the hostname label key is the same
		if candidate.Name == node.Name {
			glog.V(6).Infof("Found a matching node via name comparison. Deleted: %q New: %q", node.Name, candidate.Name)
			return candidate, nil
		}
		if candidate.ObjectMeta.Labels[metav1.LabelHostname] != "" && candidate.ObjectMeta.Labels[metav1.LabelHostname] == node.ObjectMeta.Labels[metav1.LabelHostname] {
			glog.V(6).Infof("Found a matching node via hostname-label comparison. Deleted: %q New: %q", node.ObjectMeta.Labels[metav1.LabelHostname], candidate.ObjectMeta.Labels[metav1.LabelHostname])
			return candidate, nil
		}
	}
	return nil, nodeNotFoundErr
}

func (c *Controller) migrationWorker() {
	nlist := c.nodeIndexer.List()
	for _, obj := range nlist {
		node := obj.(*v1.Node)
		//Only check for nodes for this controller
		if node.Labels[controllerLabelKey] != controllerName {
			continue
		}

		glog.V(8).Infof("Processing node %s for migration check", node.Name)

		sibling, err := c.findSibling(node)
		if err != nil {
			if err != nodeNotFoundErr {
				glog.V(0).Infof("Failed to find a sibling for %s: %v", node.Name, err)
			}
			continue
		}
		if node.ObjectMeta.CreationTimestamp.Before(sibling.ObjectMeta.CreationTimestamp) {
			glog.V(4).Infof("Found a suitable sibling for node %s to migrate. Deleting now %s to trigger the migration", node.Name, node.Name)
			err := c.client.Nodes().Delete(node.Name, &metav1.DeleteOptions{})
			if err != nil {
				glog.V(0).Infof("Failed to delete node %s for migration: %v", node.Name, err)
			}
			c.nodeIndexer.Delete(node)
			if err != nil {
				glog.V(0).Infof("Failed to delete node %s from indexer for migration: %v", node.Name, err)
			}
		} else {
			continue
		}
	}
}

func (c *Controller) deleteMigrationWatcher(node *v1.Node) wait.ConditionFunc {
	return func() (done bool, err error) {
		newNode, err := c.findSibling(node)
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
