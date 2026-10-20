package neon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient("test-api-key")
	c.httpClient = srv.Client()
	c.baseURL = srv.URL
	return c
}

func TestCreateProject(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/projects" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-api-key" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		var req createProjectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Project.RegionID != "aws-us-east-2" {
			t.Fatalf("unexpected region_id: %q", req.Project.RegionID)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(CreateProjectResponse{
			Project:        Project{ID: "proj-123", Name: req.Project.Name, RegionID: req.Project.RegionID},
			ConnectionURIs: []ConnectionURI{{ConnectionURI: "postgres://user:pass@host/db"}},
		})
	})

	resp, err := c.CreateProject(context.Background(), "my-project", "aws-us-east-2", 16)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if resp.Project.ID != "proj-123" {
		t.Fatalf("unexpected project ID: %q", resp.Project.ID)
	}
	if len(resp.ConnectionURIs) != 1 {
		t.Fatalf("expected 1 connection uri, got %d", len(resp.ConnectionURIs))
	}
}

func TestGetProjectNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(APIError{Code: "not_found", Message: "project not found"})
	})

	_, err := c.GetProject(context.Background(), "does-not-exist")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsNotFound(err) {
		t.Fatalf("expected IsNotFound(err) to be true, got err: %v", err)
	}
}

func TestGetProjectRateLimited(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(APIError{Code: "rate_limited", Message: "too many requests"})
	})

	_, err := c.GetProject(context.Background(), "proj-123")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsRateLimited(err) {
		t.Fatalf("expected IsRateLimited(err) to be true, got err: %v", err)
	}
}

func TestListProjectsByName(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("search"); got != "my-project" {
			t.Fatalf("unexpected search query param: %q", got)
		}
		_ = json.NewEncoder(w).Encode(listProjectsResponse{Projects: []Project{
			{ID: "proj-123", Name: "my-project"},
			{ID: "proj-456", Name: "my-project-2"},
		}})
	})

	projects, err := c.ListProjectsByName(context.Background(), "my-project")
	if err != nil {
		t.Fatalf("ListProjectsByName: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
}

func TestListProjectsByNameFollowsPaginationCursor(t *testing.T) {
	var calls int
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.URL.Query().Get("limit"); got != "400" {
			t.Fatalf("unexpected limit query param: %q", got)
		}
		if r.URL.Query().Get("cursor") == "" {
			// First page: full, so a cursor is returned for the next page.
			projects := make([]Project, maxProjectsPageSize)
			for i := range projects {
				projects[i] = Project{ID: "proj-filler", Name: "my-project-filler"}
			}
			_ = json.NewEncoder(w).Encode(listProjectsResponse{
				Projects:   projects,
				Pagination: &projectsPageCursor{Cursor: "next-page-cursor"},
			})
			return
		}
		if got := r.URL.Query().Get("cursor"); got != "next-page-cursor" {
			t.Fatalf("unexpected cursor query param: %q", got)
		}
		// Second (final) page: short, so the exact match here isn't missed.
		_ = json.NewEncoder(w).Encode(listProjectsResponse{Projects: []Project{
			{ID: "proj-123", Name: "my-project"},
		}})
	})

	projects, err := c.ListProjectsByName(context.Background(), "my-project")
	if err != nil {
		t.Fatalf("ListProjectsByName: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 requests to page through results, got %d", calls)
	}
	if len(projects) != maxProjectsPageSize+1 {
		t.Fatalf("expected %d projects across both pages, got %d", maxProjectsPageSize+1, len(projects))
	}
	if projects[len(projects)-1].ID != "proj-123" {
		t.Fatalf("expected the exact match from the second page to be included, got %+v", projects[len(projects)-1])
	}
}

func TestGetProjectNonJSONErrorBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>502 Bad Gateway</html>"))
	})

	_, err := c.GetProject(context.Background(), "proj-123")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "502 Bad Gateway") {
		t.Fatalf("expected error message to include the raw body snippet, got: %v", err)
	}
}

func TestGetProjectEscapesIDInPath(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/projects/proj%2F123" {
			t.Fatalf("expected escaped path, got: %s", r.URL.EscapedPath())
		}
		_ = json.NewEncoder(w).Encode(projectEnvelope{Project: Project{ID: "proj/123"}})
	})

	if _, err := c.GetProject(context.Background(), "proj/123"); err != nil {
		t.Fatalf("GetProject: %v", err)
	}
}

func TestGetProject(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/proj-123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(projectEnvelope{Project: Project{ID: "proj-123", Name: "my-project"}})
	})

	p, err := c.GetProject(context.Background(), "proj-123")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if p.Name != "my-project" {
		t.Fatalf("unexpected name: %q", p.Name)
	}
}

func TestDeleteProjectIsNoopWhenAlreadyGone(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(APIError{Code: "not_found", Message: "gone"})
	})

	if err := c.DeleteProject(context.Background(), "proj-123"); err != nil {
		t.Fatalf("expected no error deleting an already-gone project, got: %v", err)
	}
}

func TestDefaultBranch(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/proj-123/branches" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(listBranchesResponse{Branches: []Branch{
			{ID: "br-not-default", Name: "dev", Default: false},
			{ID: "br-default", Name: "main", Default: true},
		}})
	})

	b, err := c.DefaultBranch(context.Background(), "proj-123")
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if b.ID != "br-default" {
		t.Fatalf("expected the branch marked default, got %q", b.ID)
	}
}

func TestDefaultBranchErrorsWhenNoneMarkedDefault(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(listBranchesResponse{Branches: []Branch{
			{ID: "br-1", Name: "dev", Default: false},
		}})
	})

	if _, err := c.DefaultBranch(context.Background(), "proj-123"); err == nil {
		t.Fatal("expected an error when no branch is marked default")
	}
}

func TestCreateDatabase(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/projects/proj-123/branches/br-1/databases" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req createDatabaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Database.Name != "my-db" || req.Database.OwnerName != "my-role" {
			t.Fatalf("unexpected request body: %+v", req.Database)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(databaseEnvelope{Database: Database{
			Name: "my-db", OwnerName: "my-role", BranchID: "br-1",
		}})
	})

	db, err := c.CreateDatabase(context.Background(), "proj-123", "br-1", "my-db", "my-role")
	if err != nil {
		t.Fatalf("CreateDatabase: %v", err)
	}
	if db.Name != "my-db" {
		t.Fatalf("unexpected name: %q", db.Name)
	}
}

func TestGetDatabaseNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(APIError{Code: "not_found", Message: "gone"})
	})

	_, err := c.GetDatabase(context.Background(), "proj-123", "br-1", "my-db")
	if !IsNotFound(err) {
		t.Fatalf("expected IsNotFound(err) to be true, got err: %v", err)
	}
}

func TestUpdateDatabase(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/projects/proj-123/branches/br-1/databases/my-db" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req updateDatabaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Database.OwnerName != "new-role" {
			t.Fatalf("unexpected owner_name: %q", req.Database.OwnerName)
		}
		_ = json.NewEncoder(w).Encode(databaseEnvelope{Database: Database{Name: "my-db", OwnerName: "new-role"}})
	})

	db, err := c.UpdateDatabase(context.Background(), "proj-123", "br-1", "my-db", "new-role")
	if err != nil {
		t.Fatalf("UpdateDatabase: %v", err)
	}
	if db.OwnerName != "new-role" {
		t.Fatalf("unexpected owner_name: %q", db.OwnerName)
	}
}

func TestDeleteDatabaseIsNoopWhenAlreadyGone(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(APIError{Code: "not_found", Message: "gone"})
	})

	if err := c.DeleteDatabase(context.Background(), "proj-123", "br-1", "my-db"); err != nil {
		t.Fatalf("expected no error deleting an already-gone database, got: %v", err)
	}
}

func TestCreateRoleReturnsPassword(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/projects/proj-123/branches/br-1/roles" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req createRoleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Role.Name != "my-role" {
			t.Fatalf("unexpected role name: %q", req.Role.Name)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(roleEnvelope{Role: Role{Name: "my-role", Password: "s3cr3t"}})
	})

	r, err := c.CreateRole(context.Background(), "proj-123", "br-1", "my-role")
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if r.Password != "s3cr3t" {
		t.Fatalf("expected the create response to include a password, got: %q", r.Password)
	}
}

func TestGetRoleNeverIncludesPassword(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/proj-123/branches/br-1/roles/my-role" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(roleEnvelope{Role: Role{Name: "my-role", Protected: false}})
	})

	r, err := c.GetRole(context.Background(), "proj-123", "br-1", "my-role")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if r.Password != "" {
		t.Fatalf("expected GetRole to never populate Password, got: %q", r.Password)
	}
}

func TestGetRoleNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(APIError{Code: "not_found", Message: "gone"})
	})

	_, err := c.GetRole(context.Background(), "proj-123", "br-1", "my-role")
	if !IsNotFound(err) {
		t.Fatalf("expected IsNotFound(err) to be true, got err: %v", err)
	}
}

func TestRevealRolePassword(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/proj-123/branches/br-1/roles/my-role/reveal_password" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(revealRolePasswordResponse{Password: "revealed"})
	})

	pw, err := c.RevealRolePassword(context.Background(), "proj-123", "br-1", "my-role")
	if err != nil {
		t.Fatalf("RevealRolePassword: %v", err)
	}
	if pw != "revealed" {
		t.Fatalf("unexpected password: %q", pw)
	}
}

func TestDeleteRoleIsNoopWhenAlreadyGone(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(APIError{Code: "not_found", Message: "gone"})
	})

	if err := c.DeleteRole(context.Background(), "proj-123", "br-1", "my-role"); err != nil {
		t.Fatalf("expected no error deleting an already-gone role, got: %v", err)
	}
}
