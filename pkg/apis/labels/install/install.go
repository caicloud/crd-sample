package install

import (
	"github.com/caicloud/crd-sample/pkg/apis/labels/v1alpha1"
	"k8s.io/apimachinery/pkg/apimachinery/announced"
	"k8s.io/apimachinery/pkg/apimachinery/registered"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

// GroupName defines name of the group in this package
const GroupName = "labels.caicloud.io"

// Install registers the API group and adds types to a scheme
func Install(groupFactoryRegistry announced.APIGroupFactoryRegistry, registry *registered.APIRegistrationManager, scheme *runtime.Scheme) {
	if err := announced.NewGroupMetaFactory(
		&announced.GroupMetaFactoryArgs{
			GroupName: GroupName,
			// RootScopedKinds are resources that are not namespaced
			RootScopedKinds:        sets.NewString("LabelCounter", "LabelCounterList"),
			VersionPreferenceOrder: []string{v1alpha1.SchemeGroupVersion.Version},
		},
		announced.VersionToSchemeFunc{
			v1alpha1.SchemeGroupVersion.Version: v1alpha1.AddToScheme,
		},
	).Announce(groupFactoryRegistry).RegisterAndEnable(registry, scheme); err != nil {
		panic(err)
	}
}
