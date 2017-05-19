package nodestore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/machine/libmachine/host"
	"github.com/docker/machine/libmachine/log"
	"github.com/docker/machine/libmachine/mcnerror"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	kcorev1 "k8s.io/client-go/pkg/api/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	KubeMachineAnnotationKey = "node.alpha.kubernetes.io/kube-machine"
	KubeMachineLabel         = "kube-machine"
)

var (
	defaultConfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
)

type NodeStore struct {
	Path             string
	CaCertPath       string
	CaPrivateKeyPath string
	Client           kubernetes.Interface
	nodes            map[string]*kcorev1.Node
}

func NewNodeStore(path, caCertPath, caPrivateKeyPath string, kubeconfig string) *NodeStore {
	var (
		err    error
		config *rest.Config
	)
	if _, err := os.Stat(defaultConfig); kubeconfig == "" && os.IsNotExist(err) {
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	} else {
		if kubeconfig == "" {
			kubeconfig = defaultConfig
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			log.Errorf("Failed to load kubeconfig %q: %v", kubeconfig, err)
			os.Exit(1)
		}
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return &NodeStore{
		Path:             path,
		CaCertPath:       caCertPath,
		CaPrivateKeyPath: caPrivateKeyPath,
		Client:           client,
	}
}

func (s NodeStore) GetMachinesDir() string {
	return filepath.Join(s.Path, "machines")
}

func (s NodeStore) Save(host *host.Host) error {
	data, err := json.MarshalIndent(host, "", "    ")
	if err != nil {
		return err
	}

	node, err := s.Client.CoreV1().Nodes().Get(host.Name, metav1.GetOptions{})
	if err != nil && errors.IsNotFound(err) {
		node = &kcorev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: host.Name,
				Annotations: map[string]string{
					KubeMachineAnnotationKey: string(data),
				},
				Labels: map[string]string{
					KubeMachineLabel: "true",
				},
			},
			Status: kcorev1.NodeStatus{
				Phase: kcorev1.NodePending,
				// The following makes the node controller to immediately remove the node:
				/*
					Conditions: []kcorev1.NodeCondition{
						{
							Type:               kcorev1.NodeReady,
							Status:             kcorev1.ConditionFalse,
							Reason:             "created",
							Message:            fmt.Sprintf("created by kube-machine %v driver", host.DriverName),
							LastTransitionTime: metav1.NewTime(time.Now()),
							LastHeartbeatTime:  metav1.NewTime(time.Now()),
						},
					},
				*/
			},
		}
		_, err := s.Client.CoreV1().Nodes().Create(node)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		if node.Annotations == nil {
			node.Annotations = map[string]string{}
		}
		node.Annotations[KubeMachineAnnotationKey] = string(data)

		if node.Labels == nil {
			node.Labels = map[string]string{}
		}
		node.Labels[KubeMachineLabel] = "true"

		_, err = s.Client.CoreV1().Nodes().Update(node)
		if err != nil {
			return err
		}
	}

	// Ensure that the directory we want to save to exists.
	hostPath := filepath.Join(s.GetMachinesDir(), host.Name)
	if err := os.MkdirAll(hostPath, 0700); err != nil {
		return err
	}

	return nil
}

func (s NodeStore) Remove(name string) error {
	hostPath := filepath.Join(s.GetMachinesDir(), name)

	err := s.Client.CoreV1().Nodes().Delete(name, &metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	return os.RemoveAll(hostPath)
}

func (s NodeStore) Nodes() (map[string]*kcorev1.Node, error) {
	if s.nodes == nil {
		nodes, err := s.Client.CoreV1().Nodes().List(metav1.ListOptions{LabelSelector: KubeMachineLabel + "=true"})
		if err != nil {
			return nil, err
		}
		s.nodes = map[string]*kcorev1.Node{}
		for i := range nodes.Items {
			s.nodes[nodes.Items[i].Name] = &nodes.Items[i]
		}
	}
	return s.nodes, nil
}

func (s NodeStore) List() ([]string, error) {
	nodes, err := s.Nodes()
	if err != nil {
		return nil, err
	}

	hostNames := []string{}
	for name, node := range nodes {
		if _, exists := node.Annotations[KubeMachineAnnotationKey]; exists {
			hostNames = append(hostNames, name)
		}
	}

	return hostNames, nil
}

func (s NodeStore) Exists(name string) (bool, error) {
	nodes, err := s.Nodes()
	if err != nil {
		return false, err
	}

	_, found := nodes[name]
	return found, nil
}

func (s NodeStore) loadConfig(node *kcorev1.Node, h *host.Host) error {
	data, exists := node.Annotations[KubeMachineAnnotationKey]
	if !exists {
		return os.ErrNotExist
	}

	// Remember the machine name so we don't have to pass it through each
	// struct in the migration.
	name := h.Name

	migratedHost, migrationPerformed, err := host.MigrateHost(h, []byte(data))
	if err != nil {
		return fmt.Errorf("Error getting migrated host: %s", err)
	}

	*h = *migratedHost

	h.Name = name

	// If we end up performing a migration, we should save afterwards so we don't have to do it again on subsequent invocations.
	if migrationPerformed {
		if err := s.Save(h); err != nil {
			return fmt.Errorf("Error saving config after migration was performed: %s", err)
		}
	}

	return nil
}

func (s NodeStore) Load(name string) (*host.Host, error) {
	nodes, err := s.Nodes()
	if err != nil {
		return nil, err
	}
	node, found := nodes[name]
	if !found {
		return nil, mcnerror.ErrHostDoesNotExist{
			Name: name,
		}
	}
	if err != nil {
		return nil, err
	}

	host := &host.Host{
		Name: name,
	}

	if err := s.loadConfig(node, host); err != nil {
		return nil, err
	}

	return host, nil
}
