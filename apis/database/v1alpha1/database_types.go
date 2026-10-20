package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatabaseParameters define the desired state of a Neon database.
type DatabaseParameters struct {
	// ProjectID is the Neon project this database belongs to. Either this or
	// projectIdRef/projectIdSelector must resolve to a value.
	// Immutable after creation — Neon does not support moving a database between projects.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="projectId is immutable"
	ProjectID string `json:"projectId,omitempty"`

	// ProjectIDRef references a Project to resolve projectId from.
	// +optional
	ProjectIDRef *xpv1.Reference `json:"projectIdRef,omitempty"`

	// ProjectIDSelector selects a Project to resolve projectId from.
	// +optional
	ProjectIDSelector *xpv1.Selector `json:"projectIdSelector,omitempty"`

	// BranchID is the Neon branch to create the database on. Defaults to the
	// project's default branch if unset. Immutable after creation.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="branchId is immutable"
	BranchID string `json:"branchId,omitempty"`

	// Name of the database, as it appears in Neon and in Postgres itself.
	// Immutable here even though Neon's API technically allows renaming — this
	// provider addresses databases by name, so a rename would require tracking
	// the old and new name separately. Not implemented; delete and recreate instead.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// OwnerName is the Neon role that owns this database. Either this or
	// ownerNameRef/ownerNameSelector must resolve to a value. The role must
	// already exist (e.g. a project's default role, or a Role managed resource).
	// +optional
	OwnerName string `json:"ownerName,omitempty"`

	// OwnerNameRef references a Role to resolve ownerName from.
	// +optional
	OwnerNameRef *xpv1.Reference `json:"ownerNameRef,omitempty"`

	// OwnerNameSelector selects a Role to resolve ownerName from.
	// +optional
	OwnerNameSelector *xpv1.Selector `json:"ownerNameSelector,omitempty"`
}

// DatabaseObservation reflects the observed state of a Neon database.
type DatabaseObservation struct {
	// BranchID is the branch this database was actually created on — useful
	// when spec.branchId was left unset and resolved to the project's default.
	BranchID string `json:"branchId,omitempty"`

	// OwnerName is the role Neon reports as owning this database.
	OwnerName string `json:"ownerName,omitempty"`

	// CreatedAt is when the database was created, per the Neon API.
	CreatedAt string `json:"createdAt,omitempty"`
}

// A DatabaseSpec defines the desired state of a Database.
type DatabaseSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       DatabaseParameters `json:"forProvider"`
}

// A DatabaseStatus reflects the observed state of a Database.
type DatabaseStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          DatabaseObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="PROJECT",type="string",JSONPath=".spec.forProvider.projectId"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,neon}

// A Database is a managed resource representing a Neon database.
type Database struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseSpec   `json:"spec"`
	Status DatabaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatabaseList contains a list of Database.
type DatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Database `json:"items"`
}

// GetItems of this DatabaseList, satisfying resource.ManagedList (required
// for cross-resource reference selectors to enumerate candidates).
func (l *DatabaseList) GetItems() []resource.Managed {
	items := make([]resource.Managed, len(l.Items))
	for i := range l.Items {
		items[i] = &l.Items[i]
	}
	return items
}

// GetCondition of this Database.
func (d *Database) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return d.Status.GetCondition(ct)
}

// SetConditions of this Database.
func (d *Database) SetConditions(c ...xpv1.Condition) {
	d.Status.SetConditions(c...)
}

// GetProviderConfigReference of this Database.
func (d *Database) GetProviderConfigReference() *xpv1.Reference {
	return d.Spec.ProviderConfigReference
}

// SetProviderConfigReference of this Database.
func (d *Database) SetProviderConfigReference(r *xpv1.Reference) {
	d.Spec.ProviderConfigReference = r
}

// GetWriteConnectionSecretToReference of this Database.
func (d *Database) GetWriteConnectionSecretToReference() *xpv1.SecretReference {
	return d.Spec.WriteConnectionSecretToReference
}

// SetWriteConnectionSecretToReference of this Database.
func (d *Database) SetWriteConnectionSecretToReference(r *xpv1.SecretReference) {
	d.Spec.WriteConnectionSecretToReference = r
}

// GetPublishConnectionDetailsTo of this Database.
func (d *Database) GetPublishConnectionDetailsTo() *xpv1.PublishConnectionDetailsTo {
	return d.Spec.PublishConnectionDetailsTo
}

// SetPublishConnectionDetailsTo of this Database.
func (d *Database) SetPublishConnectionDetailsTo(r *xpv1.PublishConnectionDetailsTo) {
	d.Spec.PublishConnectionDetailsTo = r
}

// GetDeletionPolicy of this Database.
func (d *Database) GetDeletionPolicy() xpv1.DeletionPolicy {
	return d.Spec.DeletionPolicy
}

// SetDeletionPolicy of this Database.
func (d *Database) SetDeletionPolicy(policy xpv1.DeletionPolicy) {
	d.Spec.DeletionPolicy = policy
}

// GetManagementPolicies of this Database.
func (d *Database) GetManagementPolicies() xpv1.ManagementPolicies {
	return d.Spec.ManagementPolicies
}

// SetManagementPolicies of this Database.
func (d *Database) SetManagementPolicies(policies xpv1.ManagementPolicies) {
	d.Spec.ManagementPolicies = policies
}
