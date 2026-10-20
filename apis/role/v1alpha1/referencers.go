package v1alpha1

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/reference"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	projectv1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/project/v1alpha1"
)

// RoleName extracts a Role's Neon role name for use by other resources
// (Database) that reference a Role by its Kubernetes object name.
func RoleName() reference.ExtractValueFn {
	return func(mg resource.Managed) string {
		r, ok := mg.(*Role)
		if !ok {
			return ""
		}
		return r.Spec.ForProvider.Name
	}
}

// ResolveReferences of this Role. Detected and called automatically by
// crossplane-runtime's default APISimpleReferenceResolver — see
// managed.NewAPISimpleReferenceResolver's duck-typed interface check.
func (mg *Role) ResolveReferences(ctx context.Context, c client.Reader) error {
	r := reference.NewAPIResolver(c, mg)

	rsp, err := r.Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: mg.Spec.ForProvider.ProjectID,
		Reference:    mg.Spec.ForProvider.ProjectIDRef,
		Selector:     mg.Spec.ForProvider.ProjectIDSelector,
		To:           reference.To{Managed: &projectv1alpha1.Project{}, List: &projectv1alpha1.ProjectList{}},
		Extract:      projectv1alpha1.ProjectID(),
	})
	if err != nil {
		return errors.Wrap(err, "spec.forProvider.projectId")
	}
	mg.Spec.ForProvider.ProjectID = rsp.ResolvedValue
	mg.Spec.ForProvider.ProjectIDRef = rsp.ResolvedReference

	return nil
}
