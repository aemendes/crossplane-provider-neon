package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

// Package type metadata.
const (
	Group   = "database.neon.crossplane.io"
	Version = "v1alpha1"
)

// Compile-time checks that these types satisfy the crossplane-runtime
// interfaces reference resolution and reconciliation require. A missing
// method here surfaces only as a runtime panic on provider startup — see
// apis/v1alpha1/register.go for the same pattern on ProviderConfig types.
var (
	_ resource.Managed     = &Project{}
	_ resource.ManagedList = &ProjectList{}
)

var (
	// SchemeGroupVersion is group version used to register these objects.
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

// ProjectKind is the kind of a Project resource.
const ProjectKind = "Project"

func init() {
	SchemeBuilder.Register(&Project{}, &ProjectList{})
}
