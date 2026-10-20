package database

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/database/v1alpha1"
	"github.com/aemendes/crossplane-provider-neon/internal/clients/neon"
)

func newDatabase(name string) *v1alpha1.Database {
	d := &v1alpha1.Database{}
	d.SetName(name)
	d.Spec.ForProvider.ProjectID = "proj-123"
	d.Spec.ForProvider.BranchID = "br-1"
	d.Spec.ForProvider.Name = name
	d.Spec.ForProvider.OwnerName = "my-role"
	return d
}

func TestObserveNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found", "message": "gone"})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}
	cr := newDatabase("my-db")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.ResourceExists {
		t.Fatal("expected ResourceExists=false for a 404 from Neon")
	}
}

func TestObserveUpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/proj-123/branches/br-1/databases/my-db" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"database": map[string]any{"name": "my-db", "owner_name": "my-role"},
		})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}
	cr := newDatabase("my-db")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !obs.ResourceExists {
		t.Fatal("expected ResourceExists=true")
	}
	if !obs.ResourceUpToDate {
		t.Fatal("expected ResourceUpToDate=true when Neon's owner_name matches spec")
	}
	if cr.Status.AtProvider.OwnerName != "my-role" {
		t.Fatalf("expected status.atProvider.ownerName to be populated, got %q", cr.Status.AtProvider.OwnerName)
	}
}

func TestObserveOutOfDateWhenOwnerDiffers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"database": map[string]any{"name": "my-db", "owner_name": "someone-else"},
		})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}
	cr := newDatabase("my-db")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.ResourceUpToDate {
		t.Fatal("expected ResourceUpToDate=false when owner_name differs from spec")
	}
}

func TestObserveResourceGoneWhenParentProjectDeleted(t *testing.T) {
	cr := newDatabase("my-db")
	cr.Spec.ForProvider.BranchID = "" // force default-branch resolution

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/proj-123/branches" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found", "message": "project not found"})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.ResourceExists {
		t.Fatal("expected ResourceExists=false when the parent project is gone")
	}
}

func TestDeleteIsNoopWhenParentProjectDeleted(t *testing.T) {
	cr := newDatabase("my-db")
	cr.Spec.ForProvider.BranchID = "" // force default-branch resolution

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/proj-123/branches" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found", "message": "project not found"})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}

	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestObserveLateInitializesUnsetOwnerName(t *testing.T) {
	cr := newDatabase("my-db")
	cr.Spec.ForProvider.OwnerName = "" // no owner requested

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"database": map[string]any{"name": "my-db", "owner_name": "default-owner"},
		})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !obs.ResourceLateInitialized {
		t.Fatal("expected ResourceLateInitialized=true when adopting Neon's assigned owner")
	}
	if !obs.ResourceUpToDate {
		t.Fatal("expected ResourceUpToDate=true once the owner is adopted into spec")
	}
	if cr.Spec.ForProvider.OwnerName != "default-owner" {
		t.Fatalf("expected spec.forProvider.ownerName to be adopted as default-owner, got %q", cr.Spec.ForProvider.OwnerName)
	}
}

func TestCreateResolvesDefaultBranchWhenUnset(t *testing.T) {
	cr := newDatabase("my-db")
	cr.Spec.ForProvider.BranchID = "" // force default-branch resolution

	var sawBranchesCall, sawCreateCall bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/projects/proj-123/branches":
			sawBranchesCall = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"branches": []map[string]any{{"id": "br-default", "name": "main", "default": true}},
			})
		case r.URL.Path == "/projects/proj-123/branches/br-default/databases":
			sawCreateCall = true
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"database": map[string]any{"name": "my-db", "owner_name": "my-role"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}
	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !sawBranchesCall {
		t.Fatal("expected a call to resolve the default branch")
	}
	if !sawCreateCall {
		t.Fatal("expected the create call to use the resolved default branch")
	}
}

func TestDeleteIsNoopWhenAlreadyGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found", "message": "gone"})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}
	cr := newDatabase("my-db")

	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
