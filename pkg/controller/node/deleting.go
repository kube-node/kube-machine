package node

import (
	"k8s.io/client-go/pkg/api/v1"
)

func (c *Controller) syncDeletingNode(node *v1.Node) (changedN *v1.Node, err error) {
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

	h, err := c.mapi.Load(node)
	if err != nil {
		return nil, err
	}

	err = h.Driver.Remove()
	if err != nil {
		return nil, err
	}

	return node, nil
}
