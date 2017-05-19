package nodeclass

import (
	"strings"

	"github.com/kube-node/nodeset/pkg/nodeset/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

// EnsureThirdPartyResourcesExist checks if the thirdPartyResources exist and creates them if not
func EnsureThirdPartyResourcesExist(client kubernetes.Interface) error {
	resourceNames := []string{"node-class", "node-set"}
	for _, resName := range resourceNames {
		if err := ensureThirdPartyResource(client, resName); err != nil {
			return err
		}
	}

	return nil
}

func ensureThirdPartyResource(client kubernetes.Interface, name string) error {
	fullName := strings.Join([]string{name, v1alpha1.GroupName}, ".")
	_, err := client.ExtensionsV1beta1().ThirdPartyResources().Get(fullName, v1.GetOptions{})
	if err == nil {
		return nil
	}

	resource := &v1beta1.ThirdPartyResource{
		Versions: []v1beta1.APIVersion{
			{Name: v1alpha1.GroupVersion},
		}}
	resource.SetName(fullName)
	_, err = client.ExtensionsV1beta1().ThirdPartyResources().Create(resource)
	return err
}
