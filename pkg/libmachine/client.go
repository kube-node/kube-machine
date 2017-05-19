package libmachine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/machine/drivers/errdriver"
	"github.com/docker/machine/libmachine/auth"
	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/drivers/plugin/localbinary"
	"github.com/docker/machine/libmachine/drivers/rpc"
	"github.com/docker/machine/libmachine/engine"
	"github.com/docker/machine/libmachine/host"
	"github.com/docker/machine/libmachine/log"
	"github.com/docker/machine/libmachine/mcnerror"
	"github.com/docker/machine/libmachine/swarm"
	"github.com/docker/machine/libmachine/version"
	"github.com/kube-node/kube-machine/pkg/nodeclass"
	"github.com/kube-node/kube-machine/pkg/provision"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	driverDataAnnotationKey = "node.k8s.io/driver-data"
)

type Client struct {
	clientDriverFactory rpcdriver.RPCClientDriverFactory
}

func New() *Client {
	return &Client{
		clientDriverFactory: rpcdriver.NewRPCClientDriverFactory(),
	}
}

func (api *Client) NewHost(driverName string, rawDriver []byte) (*host.Host, error) {
	driver, err := api.clientDriverFactory.NewRPCClientDriver(driverName, rawDriver)
	if err != nil {
		return nil, err
	}

	//Initially create filesystem structure - needed otherwise we would need to patch the basedriver
	// which would make every external driver incompatible
	err = os.MkdirAll(filepath.Join(".", "machines", driver.GetMachineName()), 0755)
	if err != nil {
		return nil, err
	}

	return &host.Host{
		ConfigVersion: version.ConfigVersion,
		Name:          driver.GetMachineName(),
		Driver:        driver,
		DriverName:    driver.DriverName(),
		HostOptions: &host.Options{
			AuthOptions: &auth.Options{
				Skip: true,
			},
			EngineOptions: &engine.Options{
				InstallURL:    drivers.DefaultEngineInstallURL,
				StorageDriver: "aufs",
				TLSVerify:     true,
			},
			SwarmOptions: &swarm.Options{},
		},
	}, nil
}

func (api *Client) Load(node *v1.Node) (*host.Host, error) {
	data := node.Annotations[driverDataAnnotationKey]

	h := &host.Host{
		Name: node.Name,
	}

	migratedHost, _, err := host.MigrateHost(h, []byte(data))
	if err != nil {
		return nil, fmt.Errorf("error getting migrating host: %s", err)
	}

	*h = *migratedHost
	h.Name = node.Name

	d, err := api.clientDriverFactory.NewRPCClientDriver(h.DriverName, h.RawDriver)
	if err != nil {
		// Not being able to find a driver binary is a "known error"
		if _, ok := err.(localbinary.ErrPluginBinaryNotFound); ok {
			h.Driver = errdriver.NewDriver(h.DriverName)
			return h, nil
		}
		return nil, err
	}

	if h.DriverName == "virtualbox" {
		h.Driver = drivers.NewSerialDriver(d)
	} else {
		h.Driver = d
	}

	return h, nil
}

func (api *Client) Create(h *host.Host) error {
	log.Info("Running pre-create checks...")
	if err := h.Driver.PreCreateCheck(); err != nil {
		return mcnerror.ErrDuringPreCreate{
			Cause: err,
		}
	}
	log.Info("Creating machine...")
	if err := h.Driver.Create(); err != nil {
		return fmt.Errorf("Error in driver during machine creation: %s", err)
	}

	return nil
}

func (api *Client) Provision(h *host.Host, config *nodeclass.NodeClassConfig) error {
	log.Info("Detecting operating system of created instance...")
	provisioner, err := detector.DetectProvisioner(h.Driver)
	if err != nil {
		return fmt.Errorf("Error detecting OS: %s", err)
	}

	log.Infof("Provisioning with %s...", provisioner.String())
	if err := provisioner.Provision(*h.HostOptions.SwarmOptions, *h.HostOptions.AuthOptions, *h.HostOptions.EngineOptions); err != nil {
		return fmt.Errorf("Error running provisioning: %s", err)
	}

	log.Info("Provisioning with kube-machine provisioner...")
	if err := provisioner.ProvisionConfig(config); err != nil {
		return fmt.Errorf("Error running provisioning: %s", err)
	}

	log.Info("Node is up and running!")
	return nil
}

func (api *Client) Close() error {
	return api.clientDriverFactory.Close()
}
