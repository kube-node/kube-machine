package node

import (
	"fmt"

	"encoding/json"
	"k8s.io/client-go/pkg/api/v1"
)

func (c *Controller) syncProvisioningNode(node *v1.Node) (changedN *v1.Node, err error) {
	changedN, err = c.provisionInstance(node)
	if err != nil || changedN != nil {
		return changedN, err
	}

	return nil, nil
}

func (c *Controller) provisionInstance(node *v1.Node) (*v1.Node, error) {
	h, err := c.mapi.Load(node)
	if err != nil {
		return nil, err
	}

	_, config, err := c.getNodeClass(node.Annotations[classAnnotationKey])
	if err != nil {
		return nil, fmt.Errorf("could not get nodeclass %q for node %s: %v", node.Annotations[classAnnotationKey], node.Name, err)
	}

	err = c.mapi.Provision(h, config)
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
