package v1alpha1

import (
	"github.com/crossplane/crossplane-runtime/pkg/reference"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

// ProjectID extracts a Project's Neon project ID for use by other resources
// (Database, Role) that reference a Project by its Kubernetes object name.
func ProjectID() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		p, ok := mg.(*Project)
		if !ok {
			return ""
		}
		return p.Status.AtProvider.ID
	}
}
