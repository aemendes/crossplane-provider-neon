package role

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/role/v1alpha1"
	"github.com/aemendes/crossplane-provider-neon/internal/clients/neon"
)

func newRole(name string) *v1alpha1.Role {
	r := &v1alpha1.Role{}
	r.SetName(name)
	r.Spec.ForProvider.ProjectID = "proj-123"
	r.Spec.ForProvider.BranchID = "br-1"
	r.Spec.ForProvider.Name = name
	return r
}

func TestObserveNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found", "message": "gone"})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}
	cr := newRole("my-role")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.ResourceExists {
		t.Fatal("expected ResourceExists=false for a 404 from Neon")
	}
}

func TestObserveAlwaysUpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/proj-123/branches/br-1/roles/my-role" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"role": map[string]any{"name": "my-role", "protected": false},
		})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}
	cr := newRole("my-role")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !obs.ResourceExists || !obs.ResourceUpToDate {
		t.Fatalf("expected ResourceExists=true and ResourceUpToDate=true, got %+v", obs)
	}
	if cr.Status.AtProvider.BranchID != "br-1" {
		t.Fatalf("expected status.atProvider.branchId to be populated, got %q", cr.Status.AtProvider.BranchID)
	}
}

func TestObserveResourceGoneWhenParentProjectDeleted(t *testing.T) {
	cr := newRole("my-role")
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
	cr := newRole("my-role")
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

func TestCreatePublishesUsernameAndPassword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"role": map[string]any{"name": "my-role", "password": "s3cr3t"},
		})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}
	cr := newRole("my-role")

	creation, err := e.Create(context.Background(), cr)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if string(creation.ConnectionDetails["username"]) != "my-role" {
		t.Fatalf("unexpected username: %q", creation.ConnectionDetails["username"])
	}
	if string(creation.ConnectionDetails["password"]) != "s3cr3t" {
		t.Fatalf("unexpected password: %q", creation.ConnectionDetails["password"])
	}
}

func TestCreateResolvesDefaultBranchWhenUnset(t *testing.T) {
	cr := newRole("my-role")
	cr.Spec.ForProvider.BranchID = "" // force default-branch resolution

	var sawBranchesCall, sawCreateCall bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/projects/proj-123/branches":
			sawBranchesCall = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"branches": []map[string]any{{"id": "br-default", "name": "main", "default": true}},
			})
		case r.URL.Path == "/projects/proj-123/branches/br-default/roles":
			sawCreateCall = true
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"role": map[string]any{"name": "my-role", "password": "s3cr3t"},
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
	if !sawBranchesCall || !sawCreateCall {
		t.Fatalf("expected default-branch resolution then create, got branches=%v create=%v", sawBranchesCall, sawCreateCall)
	}
}

func TestDeleteIsNoopWhenAlreadyGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found", "message": "gone"})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}
	cr := newRole("my-role")

	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
