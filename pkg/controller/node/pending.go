package node

import (
	"fmt"

	"encoding/json"
	"errors"
	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/state"
	"github.com/kube-node/kube-machine/pkg/libmachine"
	nodehelper "github.com/kube-node/kube-machine/pkg/node"
	"github.com/kube-node/kube-machine/pkg/options"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	noExecuteTaintKey = "node.k8s.io/not-up"
)

func (c *Controller) syncPendingNode(node *v1.Node) (changedN *v1.Node, err error) {

	changedN, err = c.pendingCreateTaint(node)
	if err != nil || changedN != nil {
		return changedN, err
	}

	changedN, err = c.pendingCreateFinalizer(node)
	if err != nil || changedN != nil {
		return changedN, err
	}

	changedN, err = c.pendingCreateInstance(node)
	if err != nil || changedN != nil {
		return changedN, err
	}

	changedN, err = c.pendingCreateInstanceDetails(node)
	if err != nil || changedN != nil {
		return changedN, err
	}

	//Will set the phase to provisioning
	changedN, err = c.pendingWaitUntilInstanceIsRunning(node)
	if err != nil || changedN != nil {
		return changedN, err
	}

	return nil, nil
}

func (c *Controller) pendingCreateTaint(node *v1.Node) (*v1.Node, error) {
	if !nodehelper.HasTaint(node, noExecuteTaintKey) {
		node.Spec.Taints = append(node.Spec.Taints, v1.Taint{
			Key:    noExecuteTaintKey,
			Effect: v1.TaintEffectNoExecute,
			Value:  "kube-machine",
		})

		return node, nil
	}

	return nil, nil
}

func (c *Controller) pendingCreateFinalizer(node *v1.Node) (*v1.Node, error) {
	if !nodehelper.HasFinalizer(node, deleteFinalizerName) {
		node.Finalizers = append(node.Finalizers, deleteFinalizerName)
		return node, nil
	}

	return nil, nil
}

func (c *Controller) pendingCreateInstance(node *v1.Node) (*v1.Node, error) {
	if node.Annotations[driverDataAnnotationKey] != "" {
		return nil, nil
	}

	class, config, err := c.getNodeClass(node.Annotations[classAnnotationKey])
	if err != nil {
		return nil, fmt.Errorf("could not get nodeclass %q for node %s: %v", node.Annotations[classAnnotationKey], node.Name, err)
	}

	rawDriver, err := json.Marshal(&drivers.BaseDriver{MachineName: node.Name})
	if err != nil {
		return nil, fmt.Errorf("error attempting to marshal bare driver data: %s", err)
	}

	mapi := libmachine.New()
	defer mapi.Close()

	mhost, err := mapi.NewHost(config.Provider, rawDriver)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker machine host for node %q: %v", node.Name, err)
	}

	opts := options.New(config.DockerMachineFlags)
	mcnFlags := mhost.Driver.GetCreateFlags()
	driverOpts := options.GetDriverOpts(opts, mcnFlags, class.Resources)

	if url, exists := config.DockerMachineFlags["engine-install-url"]; exists {
		mhost.HostOptions.EngineOptions.InstallURL = url
	}

	mhost.Driver.SetConfigFromFlags(driverOpts)

	err = mapi.Create(mhost)
	if err != nil {
		mhost.Driver.Remove()
		return nil, fmt.Errorf("failed to create node %q on cloud provider: %v. Deleted eventually created node on cloud provider", node.Name, err)
	}

	data, err := json.Marshal(mhost)
	if err != nil {
		return nil, err
	}
	node.Annotations[driverDataAnnotationKey] = string(data)
	return node, nil
}

func (c *Controller) pendingCreateInstanceDetails(node *v1.Node) (*v1.Node, error) {
	if node.Annotations[publicIPAnnotationKey] != "" {
		return nil, nil
	}

	mapi := libmachine.New()
	defer mapi.Close()

	h, err := mapi.Load(node)
	if err != nil {
		return nil, err
	}

	ip, err := h.Driver.GetIP()
	if err != nil {
		return nil, errors.New("could not get public ip")
	}
	node.Annotations[publicIPAnnotationKey] = ip

	hostname, err := h.Driver.GetSSHHostname()
	if err != nil {
		return nil, errors.New("could not get hostname")
	}
	node.Annotations[hostnameAnnotationKey] = hostname

	return node, nil

}

func (c *Controller) pendingWaitUntilInstanceIsRunning(node *v1.Node) (*v1.Node, error) {
	mapi := libmachine.New()
	defer mapi.Close()

	h, err := mapi.Load(node)
	if err != nil {
		return nil, err
	}

	s, err := h.Driver.GetState()
	if err != nil {
		return nil, fmt.Errorf("failed getting instance state: %v", err)
	}
	if s == state.Running {
		node.Annotations[phaseAnnotationKey] = phaseProvisioning
		return node, nil
	}

	return nil, nil
}
