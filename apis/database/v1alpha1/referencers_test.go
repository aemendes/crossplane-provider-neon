package v1alpha1

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	projectv1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/project/v1alpha1"
	rolev1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/role/v1alpha1"
)

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	if err := projectv1alpha1.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("add project scheme: %v", err)
	}
	if err := rolev1alpha1.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("add role scheme: %v", err)
	}
	if err := SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("add database scheme: %v", err)
	}
	return s
}

func TestResolveReferencesByRef(t *testing.T) {
	project := &projectv1alpha1.Project{ObjectMeta: metav1.ObjectMeta{Name: "my-project"}}
	project.Status.AtProvider.ID = "proj-123"

	role := &rolev1alpha1.Role{ObjectMeta: metav1.ObjectMeta{Name: "my-role"}}
	role.Spec.ForProvider.Name = "app_owner"

	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(project, role).Build()

	db := &Database{}
	db.Spec.ForProvider.ProjectIDRef = &xpv1.Reference{Name: "my-project"}
	db.Spec.ForProvider.OwnerNameRef = &xpv1.Reference{Name: "my-role"}

	if err := db.ResolveReferences(context.Background(), c); err != nil {
		t.Fatalf("ResolveReferences: %v", err)
	}
	if db.Spec.ForProvider.ProjectID != "proj-123" {
		t.Fatalf("expected projectId resolved to proj-123, got %q", db.Spec.ForProvider.ProjectID)
	}
	if db.Spec.ForProvider.OwnerName != "app_owner" {
		t.Fatalf("expected ownerName resolved to app_owner, got %q", db.Spec.ForProvider.OwnerName)
	}
}

func TestResolveReferencesDoesNotOverwriteExistingValue(t *testing.T) {
	project := &projectv1alpha1.Project{ObjectMeta: metav1.ObjectMeta{Name: "my-project"}}
	project.Status.AtProvider.ID = "proj-123"

	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(project).Build()

	db := &Database{}
	db.Spec.ForProvider.ProjectID = "already-set"
	db.Spec.ForProvider.ProjectIDRef = &xpv1.Reference{Name: "my-project"}
	db.Spec.ForProvider.OwnerName = "already-set-owner"

	if err := db.ResolveReferences(context.Background(), c); err != nil {
		t.Fatalf("ResolveReferences: %v", err)
	}
	if db.Spec.ForProvider.ProjectID != "already-set" {
		t.Fatalf("expected an already-set projectId to be left alone, got %q", db.Spec.ForProvider.ProjectID)
	}
}
