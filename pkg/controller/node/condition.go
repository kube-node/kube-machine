package node

import (
	"time"

	"github.com/golang/glog"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	tempReadyConditionReason = "Kubelet is being provisioned by the nodecontroller"
)

func (c *Controller) readyConditionWorker() {
	nlist := c.nodeIndexer.List()
	for _, obj := range nlist {
		node := obj.(*v1.Node)

		con := v1.NodeCondition{
			Type:               v1.NodeReady,
			Status:             v1.ConditionTrue,
			Reason:             tempReadyConditionReason,
			Message:            "kubelet is not created by kube-machine. This condition prevents node deletion by the controller-manager",
			LastHeartbeatTime:  metav1.NewTime(time.Now()),
			LastTransitionTime: metav1.NewTime(time.Now()),
		}

		var found, updated bool
		for i := range node.Status.Conditions {
			if node.Status.Conditions[i].Type == v1.NodeReady {
				found = true
				if node.Status.Conditions[i].Reason == tempReadyConditionReason {
					node.Status.Conditions[i] = con
					updated = true
				}
				break
			}
		}

		if !found {
			node.Status.Conditions = append(node.Status.Conditions, con)
			updated = true
		}

		if updated {
			glog.V(6).Infof("Updating node ready condition for %s to avoid node deletion by kube-controller-manager", node.Name)
			_, err := c.client.CoreV1().Nodes().UpdateStatus(node)
			if err != nil {

				glog.V(0).Infof("Failed to update node ready condition for %s: %v", node.Name, err)
			}
		}
	}
}
