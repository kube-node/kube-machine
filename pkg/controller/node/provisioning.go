package node

import (
	"encoding/json"
	"fmt"

	"github.com/kube-node/kube-machine/pkg/libmachine"
	"github.com/kube-node/nodeset/pkg/nodeset/v1alpha1"

	"k8s.io/api/core/v1"
)

func (c *Controller) syncProvisioningNode(node *v1.Node) (changedN *v1.Node, err error) {
	changedN, err = c.provisionInstance(node)
	if err != nil || changedN != nil {
		return changedN, err
	}

	return nil, nil
}

func (c *Controller) provisionInstance(node *v1.Node) (*v1.Node, error) {
	mapi := libmachine.New()
	defer mapi.Close()

	h, err := mapi.Load(node)
	if err != nil {
		return nil, err
	}

	_, config, err := c.getNodeClass(node)
	if err != nil {
		return nil, fmt.Errorf("could not get nodeclass %q for node %s: %v", node.Annotations[v1alpha1.NodeClassNameAnnotationKey], node.Name, err)
	}

	err = mapi.Provision(h, config)
	if err != nil {
		return nil, fmt.Errorf("could not provision: %v", err)
	}

	data, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}

	node.Annotations[driverDataAnnotationKey] = string(data)
	node.Annotations[phaseAnnotationKey] = phaseLaunching

	return node, nil
}
