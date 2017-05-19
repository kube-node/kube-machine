/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package nodeclass

type NodeClassConfig struct {
	DockerMachineFlags map[string]string          `json:"dockerMachineFlags"`
	Provisioning       NodeClassProvisionerConfig `json:"provisioning"`
	Provider           string                     `json:"provider"`
}

type NodeClassProvisionerConfig struct {
	Files    []NodeClassProvisioningConfigFile `json:"files"`
	Commands []string                          `json:"commands"`
	Users    []NodeClassProvisioningUser       `json:"users"`
}

type NodeClassProvisioningConfigFile struct {
	Path        string `json:"path"`
	Permissions string `json:"permissions"`
	Owner       string `json:"owner"`
	Content     string `json:"content"`
}

type NodeClassProvisioningUser struct {
	Name    string   `json:"name"`
	SSHKeys []string `json:"ssh_keys"`
	Sudo    bool     `json:"sudo"`
}
