package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProjectParameters define the desired state of a Neon project.
type ProjectParameters struct {
	// Name of the project, shown in the Neon console.
	// +optional
	Name string `json:"name,omitempty"`

	// RegionID is the Neon region to create the project in. As of this writing:
	// aws-us-east-2, aws-us-east-1, aws-us-west-2, aws-eu-central-1, aws-eu-west-2,
	// aws-ap-southeast-1, aws-ap-southeast-2, aws-sa-east-1, azure-eastus2,
	// azure-westus3, azure-gwc. Not enforced here (Neon's own API is the source of
	// truth and rejects an invalid value) — check
	// https://neon.com/docs/reference/api/regions/get-active-regions for the
	// current list, which can change without a provider update.
	// Immutable after creation — Neon does not support moving a project between regions.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="regionId is immutable"
	RegionID string `json:"regionId"`

	// PGVersion is the Postgres major version, e.g. 16. Immutable after creation —
	// Neon does not support changing a project's Postgres major version in place.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="pgVersion is immutable"
	PGVersion int `json:"pgVersion,omitempty"`
}

// ProjectObservation reflects the observed state of a Neon project.
type ProjectObservation struct {
	// ID is the Neon project ID.
	ID string `json:"id,omitempty"`

	// Host is the Neon connection host for the project's default branch/endpoint.
	Host string `json:"host,omitempty"`

	// RegionID is the region Neon reports for this project.
	RegionID string `json:"regionId,omitempty"`

	// PGVersion is the Postgres major version Neon reports for this project.
	PGVersion int `json:"pgVersion,omitempty"`

	// CreatedAt is when the project was created, per the Neon API.
	CreatedAt string `json:"createdAt,omitempty"`
}

// A ProjectSpec defines the desired state of a Project.
type ProjectSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       ProjectParameters `json:"forProvider"`
}

// A ProjectStatus reflects the observed state of a Project.
type ProjectStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          ProjectObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.atProvider.id"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,neon}

// A Project is a managed resource representing a Neon project.
type Project struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectSpec   `json:"spec"`
	Status ProjectStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProjectList contains a list of Project.
type ProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Project `json:"items"`
}

// GetItems of this ProjectList, satisfying resource.ManagedList (required for
// cross-resource reference selectors to enumerate candidates).
func (l *ProjectList) GetItems() []resource.Managed {
	items := make([]resource.Managed, len(l.Items))
	for i := range l.Items {
		items[i] = &l.Items[i]
	}
	return items
}

// GetCondition of this Project.
func (p *Project) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return p.Status.GetCondition(ct)
}

// SetConditions of this Project.
func (p *Project) SetConditions(c ...xpv1.Condition) {
	p.Status.SetConditions(c...)
}

// GetProviderConfigReference of this Project.
func (p *Project) GetProviderConfigReference() *xpv1.Reference {
	return p.Spec.ProviderConfigReference
}

// SetProviderConfigReference of this Project.
func (p *Project) SetProviderConfigReference(r *xpv1.Reference) {
	p.Spec.ProviderConfigReference = r
}

// GetWriteConnectionSecretToReference of this Project.
func (p *Project) GetWriteConnectionSecretToReference() *xpv1.SecretReference {
	return p.Spec.WriteConnectionSecretToReference
}

// SetWriteConnectionSecretToReference of this Project.
func (p *Project) SetWriteConnectionSecretToReference(r *xpv1.SecretReference) {
	p.Spec.WriteConnectionSecretToReference = r
}

// GetPublishConnectionDetailsTo of this Project.
func (p *Project) GetPublishConnectionDetailsTo() *xpv1.PublishConnectionDetailsTo {
	return p.Spec.PublishConnectionDetailsTo
}

// SetPublishConnectionDetailsTo of this Project.
func (p *Project) SetPublishConnectionDetailsTo(r *xpv1.PublishConnectionDetailsTo) {
	p.Spec.PublishConnectionDetailsTo = r
}

// GetDeletionPolicy of this Project.
func (p *Project) GetDeletionPolicy() xpv1.DeletionPolicy {
	return p.Spec.DeletionPolicy
}

// SetDeletionPolicy of this Project.
func (p *Project) SetDeletionPolicy(policy xpv1.DeletionPolicy) {
	p.Spec.DeletionPolicy = policy
}

// GetManagementPolicies of this Project.
func (p *Project) GetManagementPolicies() xpv1.ManagementPolicies {
	return p.Spec.ManagementPolicies
}

// SetManagementPolicies of this Project.
func (p *Project) SetManagementPolicies(policies xpv1.ManagementPolicies) {
	p.Spec.ManagementPolicies = policies
}
