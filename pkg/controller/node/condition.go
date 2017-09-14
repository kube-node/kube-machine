package node

import (
	"encoding/json"
	"time"

	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
)

func (c *Controller) readyConditionWorker() {
	c.nodeIndexer.Resync()
	nlist := c.nodeIndexer.List()
	for _, obj := range nlist {
		node := obj.(*v1.Node)
		if node.Annotations[phaseAnnotationKey] == phaseRunning {
			continue
		}

		originalData, err := json.Marshal(node)
		if err != nil {
			glog.Errorf("Failed marshal node %s: %v", node.Name, err)
		}

		con := v1.NodeCondition{
			Type:               v1.NodeReady,
			Status:             v1.ConditionTrue,
			Reason:             "Kubelet is being provisioned by the nodecontroller",
			Message:            "kubelet is not created by kube-machine. This condition prevents node deletion by the controller-manager",
			LastHeartbeatTime:  metav1.NewTime(time.Now()),
			LastTransitionTime: metav1.NewTime(time.Now()),
		}

		var found bool
		for i := range node.Status.Conditions {
			if node.Status.Conditions[i].Type == v1.NodeReady {
				node.Status.Conditions[i] = con
				glog.Infof("Node ready condition found for %s", node.Name)
				found = true
				break
			}
		}

		if !found {
			glog.Infof("Node ready condition not found found for %s", node.Name)
			node.Status.Conditions = append(node.Status.Conditions, con)
		}

		glog.Infof("Updating node ready condition for %s", node.Name)
		err = c.updateNode(originalData, node)
		if err != nil {
			glog.Errorf("Failed to update node ready condition for %s: %v", node.Name, err)
		}
	}
}
