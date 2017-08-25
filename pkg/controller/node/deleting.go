package node

import (
	"github.com/golang/glog"
	nodehelper "github.com/kube-node/kube-machine/pkg/node"
	"k8s.io/client-go/pkg/api/v1"
	"time"
)

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
		// Check for 60 seconds if a new node with the same name appeared.
		// In this case, migrate the node-controller labels&annotation to the new node
		// If a migration happened do not delete the instance at the cloud-provider
		for i := 0; i < 60; i++ {
			time.Sleep(1 * time.Second)

			newNode, err := c.findRecreatedNode(node)
			if err != nil {
				if err != nodeNotFoundErr {
					glog.Errorf("Failed to fetch node %s during migration check: %v", node.Name, err)
				}
				continue
			}
			err = c.migrateNode(node, newNode)
			if err != nil {
				glog.Errorf("Failed to migrate node %s: %v", node.Name, err)
				continue
			}
			glog.Infof("Migrated node %s", node.Name)
			return
		}

		h, err := c.mapi.Load(node)
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
		if node.Name == deletedNode.Name && node.UID != deletedNode.UID {
			return node, nil
		}
	}
	return nil, nodeNotFoundErr
}

func (c *Controller) migrateNode(deletedNode, currentNode *v1.Node) error {
	currentNode.Labels[controllerLabelKey] = controllerName

	currentNode.Annotations[driverDataAnnotationKey] = deletedNode.Annotations[driverDataAnnotationKey]
	currentNode.Annotations[hostnameAnnotationKey] = deletedNode.Annotations[hostnameAnnotationKey]
	currentNode.Annotations[classAnnotationKey] = deletedNode.Annotations[classAnnotationKey]
	currentNode.Annotations[publicIPAnnotationKey] = deletedNode.Annotations[publicIPAnnotationKey]
	currentNode.Annotations[phaseAnnotationKey] = phaseRunning

	currentNode, err := c.pendingCreateFinalizer(currentNode)
	if err != nil {
		return err
	}

	_, err = c.client.Nodes().Update(currentNode)
	return err
}
