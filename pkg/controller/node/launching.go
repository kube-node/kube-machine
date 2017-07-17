package node

import (
	nodehelper "github.com/kube-node/kube-machine/pkg/node"
	"k8s.io/client-go/pkg/api/v1"
)

func (c *Controller) syncLaunchingNode(node *v1.Node) (changedN *v1.Node, err error) {
	changedN, err = c.syncLaunchingHeartbeat(node)
	if err != nil || changedN != nil {
		return changedN, err
	}

	return nil, nil
}

func (c *Controller) syncLaunchingHeartbeat(node *v1.Node) (*v1.Node, error) {
	if !nodehelper.HasJoined(node) {
		return nil, nil
	}

	for i, t := range node.Spec.Taints {
		if t.Key == noExecuteTaintKey {
			node.Spec.Taints = append(node.Spec.Taints[:i], node.Spec.Taints[i+1:]...)
		}
	}

	node.Annotations[phaseAnnotationKey] = phaseRunning
	return node, nil
}
