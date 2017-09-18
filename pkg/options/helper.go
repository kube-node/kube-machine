package options

import (
	"fmt"

	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/drivers/rpc"
	"github.com/docker/machine/libmachine/mcnflag"
	"github.com/kube-node/nodeset/pkg/nodeset/v1alpha1"
)

func GetDriverOpts(opts StringMapOptions, mcnflags []mcnflag.Flag, resources []v1alpha1.NodeClassResource) drivers.DriverOptions {
	driverOpts := rpcdriver.RPCFlags{
		Values: make(map[string]interface{}),
	}

	for _, f := range mcnflags {
		name := f.String()
		driverOpts.Values[name] = f.Default()

		// Hardcoded logic for boolean... :(
		if f.Default() == nil {
			driverOpts.Values[name] = false
		}
	}

	names := opts.Names()
	for _, name := range names {
		for _, f := range mcnflags {
			if f.String() != name {
				continue
			}

			switch v := driverOpts.Values[name].(type) {
			case int:
				driverOpts.Values[name] = opts.Int(name)
			case string:
				driverOpts.Values[name] = opts.String(name)
			case []string:
				driverOpts.Values[name] = opts.StringSlice(name)
			case bool:
				driverOpts.Values[name] = opts.Bool(name)
			default:
				panic(fmt.Errorf("unknown flag type %s for %s", v, name))
			}
		}
	}

	return driverOpts
}
