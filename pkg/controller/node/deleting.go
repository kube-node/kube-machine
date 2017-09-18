package node

import (
	"time"

	"github.com/golang/glog"
	"github.com/kube-node/kube-machine/pkg/libmachine"
	nodehelper "github.com/kube-node/kube-machine/pkg/node"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
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
		pendingMigrationNodes[node.Name] = true
		defer func() { delete(pendingMigrationNodes, node.Name) }()
		// Check for c.maxMigrationWaitTime if a new node with the same name appeared.
		// In this case, migrate the node-controller labels&annotation to the new node
		// If a migration happened do not delete the instance at the cloud-provider
		glog.V(6).Infof("Waiting %s to see if a new node appears for migration after %s got deleted", c.maxMigrationWaitTime, node.Name)

		wait.Poll(migrationCheckInterval, c.maxMigrationWaitTime, c.deleteMigrationWatcher(node))
		if node.Annotations[driverDataAnnotationKey] == "" {
			return
		}

		glog.V(4).Infof("No new node found for deleted node %s. Will delete it at cloud-provider", node.Name)

		mapi := libmachine.New()
		defer mapi.Close()

		h, err := mapi.Load(node)
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
