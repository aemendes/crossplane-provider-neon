package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RoleParameters define the desired state of a Neon role.
type RoleParameters struct {
	// ProjectID is the Neon project this role belongs to. Either this or
	// projectIdRef/projectIdSelector must resolve to a value.
	// Immutable after creation — Neon does not support moving a role between projects.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="projectId is immutable"
	ProjectID string `json:"projectId,omitempty"`

	// ProjectIDRef references a Project to resolve projectId from.
	// +optional
	ProjectIDRef *xpv1.Reference `json:"projectIdRef,omitempty"`

	// ProjectIDSelector selects a Project to resolve projectId from.
	// +optional
	ProjectIDSelector *xpv1.Selector `json:"projectIdSelector,omitempty"`

	// BranchID is the Neon branch to create the role on. Defaults to the
	// project's default branch if unset. Immutable after creation.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="branchId is immutable"
	BranchID string `json:"branchId,omitempty"`

	// Name of the role, as it appears in Neon and in Postgres itself. Immutable —
	// Neon has no rename endpoint for roles.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`
}

// RoleObservation reflects the observed state of a Neon role.
type RoleObservation struct {
	// BranchID is the branch this role was actually created on — useful when
	// spec.branchId was left unset and resolved to the project's default.
	BranchID string `json:"branchId,omitempty"`

	// Protected is true for Neon-managed system roles that can't be deleted.
	Protected bool `json:"protected,omitempty"`

	// CreatedAt is when the role was created, per the Neon API.
	CreatedAt string `json:"createdAt,omitempty"`
}

// A RoleSpec defines the desired state of a Role.
type RoleSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       RoleParameters `json:"forProvider"`
}

// A RoleStatus reflects the observed state of a Role.
type RoleStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          RoleObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="PROJECT",type="string",JSONPath=".spec.forProvider.projectId"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,neon}

// A Role is a managed resource representing a Neon (Postgres) role.
type Role struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RoleSpec   `json:"spec"`
	Status RoleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RoleList contains a list of Role.
type RoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Role `json:"items"`
}

// GetItems of this RoleList, satisfying resource.ManagedList (required for
// cross-resource reference selectors to enumerate candidates).
func (l *RoleList) GetItems() []resource.Managed {
	items := make([]resource.Managed, len(l.Items))
	for i := range l.Items {
		items[i] = &l.Items[i]
	}
	return items
}

// GetCondition of this Role.
func (r *Role) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return r.Status.GetCondition(ct)
}

// SetConditions of this Role.
func (r *Role) SetConditions(c ...xpv1.Condition) {
	r.Status.SetConditions(c...)
}

// GetProviderConfigReference of this Role.
func (r *Role) GetProviderConfigReference() *xpv1.Reference {
	return r.Spec.ProviderConfigReference
}

// SetProviderConfigReference of this Role.
func (r *Role) SetProviderConfigReference(p *xpv1.Reference) {
	r.Spec.ProviderConfigReference = p
}

// GetWriteConnectionSecretToReference of this Role.
func (r *Role) GetWriteConnectionSecretToReference() *xpv1.SecretReference {
	return r.Spec.WriteConnectionSecretToReference
}

// SetWriteConnectionSecretToReference of this Role.
func (r *Role) SetWriteConnectionSecretToReference(s *xpv1.SecretReference) {
	r.Spec.WriteConnectionSecretToReference = s
}

// GetPublishConnectionDetailsTo of this Role.
func (r *Role) GetPublishConnectionDetailsTo() *xpv1.PublishConnectionDetailsTo {
	return r.Spec.PublishConnectionDetailsTo
}

// SetPublishConnectionDetailsTo of this Role.
func (r *Role) SetPublishConnectionDetailsTo(p *xpv1.PublishConnectionDetailsTo) {
	r.Spec.PublishConnectionDetailsTo = p
}

// GetDeletionPolicy of this Role.
func (r *Role) GetDeletionPolicy() xpv1.DeletionPolicy {
	return r.Spec.DeletionPolicy
}

// SetDeletionPolicy of this Role.
func (r *Role) SetDeletionPolicy(policy xpv1.DeletionPolicy) {
	r.Spec.DeletionPolicy = policy
}

// GetManagementPolicies of this Role.
func (r *Role) GetManagementPolicies() xpv1.ManagementPolicies {
	return r.Spec.ManagementPolicies
}

// SetManagementPolicies of this Role.
func (r *Role) SetManagementPolicies(policies xpv1.ManagementPolicies) {
	r.Spec.ManagementPolicies = policies
}
