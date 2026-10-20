package project

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/meta"

	v1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/project/v1alpha1"
	"github.com/aemendes/crossplane-provider-neon/internal/clients/neon"
)

func newProject(name string) *v1alpha1.Project {
	p := &v1alpha1.Project{}
	p.SetName(name)
	p.Spec.ForProvider.RegionID = "aws-us-east-2"
	return p
}

func TestObserveNoExternalNameAndNoMatchByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"projects": []any{}})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}
	cr := newProject("my-project")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.ResourceExists {
		t.Fatal("expected ResourceExists=false when no external-name annotation is set and no name match is found")
	}
	if meta.GetExternalName(cr) != "" {
		t.Fatalf("expected external-name to stay unset, got %q", meta.GetExternalName(cr))
	}
}

func TestObserveAdoptsByNameWhenExternalNameMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("search"); got != "my-project" {
			t.Fatalf("unexpected search query param: %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"projects": []map[string]any{
			{"id": "proj-123", "name": "my-project"},
		}})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}
	cr := newProject("my-project")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !obs.ResourceExists {
		t.Fatal("expected ResourceExists=true when a project with an exact name match is found")
	}
	if meta.GetExternalName(cr) != "proj-123" {
		t.Fatalf("expected external-name to be adopted as proj-123, got %q", meta.GetExternalName(cr))
	}
	if !obs.ResourceLateInitialized {
		t.Fatal("expected ResourceLateInitialized=true so the adopted external-name annotation is persisted")
	}
}

func TestObserveNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found", "message": "gone"})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}
	cr := newProject("my-project")
	meta.SetExternalName(cr, "proj-123")

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
		_ = json.NewEncoder(w).Encode(map[string]any{
			"project": map[string]any{"id": "proj-123", "name": "my-project", "proxy_host": "us-east-2.aws.neon.tech"},
		})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}
	cr := newProject("my-project")
	meta.SetExternalName(cr, "proj-123")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !obs.ResourceExists {
		t.Fatal("expected ResourceExists=true")
	}
	if !obs.ResourceUpToDate {
		t.Fatal("expected ResourceUpToDate=true when Neon's name matches spec")
	}
	if cr.Status.AtProvider.ID != "proj-123" {
		t.Fatalf("expected status.atProvider.id to be populated, got %q", cr.Status.AtProvider.ID)
	}
	if cr.Status.AtProvider.Host != "us-east-2.aws.neon.tech" {
		t.Fatalf("expected status.atProvider.host to be populated, got %q", cr.Status.AtProvider.Host)
	}
}

func TestCreateSetsExternalNameAndConnectionDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"project":         map[string]any{"id": "proj-123", "name": "my-project"},
			"connection_uris": []map[string]string{{"connection_uri": "postgres://user:pass@host/db"}},
		})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}
	cr := newProject("my-project")

	creation, err := e.Create(context.Background(), cr)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if meta.GetExternalName(cr) != "proj-123" {
		t.Fatalf("expected external-name to be set to proj-123, got %q", meta.GetExternalName(cr))
	}
	if string(creation.ConnectionDetails["connectionUri"]) != "postgres://user:pass@host/db" {
		t.Fatalf("unexpected connection details: %v", creation.ConnectionDetails)
	}
}

func TestDeleteIsNoopWhenAlreadyGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found", "message": "gone"})
	}))
	defer srv.Close()

	e := &external{client: neon.NewClientWithBaseURL("key", srv.URL)}
	cr := newProject("my-project")
	meta.SetExternalName(cr, "proj-123")

	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
