package node

import "k8s.io/client-go/pkg/api/v1"

func HasFinalizer(n *v1.Node, name string) bool {
	for _, f := range n.Finalizers {
		if f == name {
			return true
		}
	}
	return false
}

func HasTaint(n *v1.Node, key string) bool {
	for _, t := range n.Spec.Taints {
		if t.Key == key {
			return true
		}
	}
	return false
}

func HasJoined(n *v1.Node) bool {
	if len(n.Status.Conditions) == 0 {
		return false
	}
	for _, c := range n.Status.Conditions {
		if c.Reason == "NodeStatusNeverUpdated" {
			return false
		}
	}
	return true
}
