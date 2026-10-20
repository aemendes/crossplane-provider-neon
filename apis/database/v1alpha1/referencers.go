package v1alpha1

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/reference"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	projectv1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/project/v1alpha1"
	rolev1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/role/v1alpha1"
)

// ResolveReferences of this Database. Detected and called automatically by
// crossplane-runtime's default APISimpleReferenceResolver — see
// managed.NewAPISimpleReferenceResolver's duck-typed interface check.
func (mg *Database) ResolveReferences(ctx context.Context, c client.Reader) error {
	r := reference.NewAPIResolver(c, mg)

	project, err := r.Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: mg.Spec.ForProvider.ProjectID,
		Reference:    mg.Spec.ForProvider.ProjectIDRef,
		Selector:     mg.Spec.ForProvider.ProjectIDSelector,
		To:           reference.To{Managed: &projectv1alpha1.Project{}, List: &projectv1alpha1.ProjectList{}},
		Extract:      projectv1alpha1.ProjectID(),
	})
	if err != nil {
		return errors.Wrap(err, "spec.forProvider.projectId")
	}
	mg.Spec.ForProvider.ProjectID = project.ResolvedValue
	mg.Spec.ForProvider.ProjectIDRef = project.ResolvedReference

	owner, err := r.Resolve(ctx, reference.ResolutionRequest{
		CurrentValue: mg.Spec.ForProvider.OwnerName,
		Reference:    mg.Spec.ForProvider.OwnerNameRef,
		Selector:     mg.Spec.ForProvider.OwnerNameSelector,
		To:           reference.To{Managed: &rolev1alpha1.Role{}, List: &rolev1alpha1.RoleList{}},
		Extract:      rolev1alpha1.RoleName(),
	})
	if err != nil {
		return errors.Wrap(err, "spec.forProvider.ownerName")
	}
	mg.Spec.ForProvider.OwnerName = owner.ResolvedValue
	mg.Spec.ForProvider.OwnerNameRef = owner.ResolvedReference

	return nil
}
