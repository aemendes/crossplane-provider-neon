// Package neon is a minimal REST client for the Neon API (https://api-docs.neon.tech/reference/getting-started-with-neon-api).
package neon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	defaultBaseURL = "https://console.neon.tech/api/v2"
	requestTimeout = 30 * time.Second
)

// Client is a minimal Neon API client covering the Project resource.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

// NewClient returns a Client authenticated with the supplied Neon API key.
func NewClient(apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: requestTimeout},
		baseURL:    defaultBaseURL,
		apiKey:     apiKey,
	}
}

// NewClientWithBaseURL returns a Client pointed at a custom base URL, for use in tests.
func NewClientWithBaseURL(apiKey, baseURL string) *Client {
	c := NewClient(apiKey)
	c.baseURL = baseURL
	return c
}

// Project mirrors the subset of Neon's project object this provider manages.
type Project struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	RegionID  string `json:"region_id"`
	PgVersion int    `json:"pg_version"`
	ProxyHost string `json:"proxy_host"`
	CreatedAt string `json:"created_at"`
}

// ConnectionURI is one entry of the connection_uris list returned on project creation.
type ConnectionURI struct {
	ConnectionURI string `json:"connection_uri"`
}

type projectEnvelope struct {
	Project Project `json:"project"`
}

type listProjectsResponse struct {
	Projects   []Project           `json:"projects"`
	Pagination *projectsPageCursor `json:"pagination,omitempty"`
}

// projectsPageCursor is Neon's cursor-pagination envelope: pass Cursor back as
// the next request's "cursor" query param to fetch the following page.
type projectsPageCursor struct {
	Cursor string `json:"cursor"`
}

type createProjectRequest struct {
	Project createProjectSettings `json:"project"`
}

type createProjectSettings struct {
	Name      string `json:"name,omitempty"`
	RegionID  string `json:"region_id,omitempty"`
	PgVersion int    `json:"pg_version,omitempty"`
}

// CreateProjectResponse is the response to a project creation call.
type CreateProjectResponse struct {
	Project        Project         `json:"project"`
	ConnectionURIs []ConnectionURI `json:"connection_uris"`
}

type updateProjectRequest struct {
	Project updateProjectSettings `json:"project"`
}

type updateProjectSettings struct {
	Name string `json:"name,omitempty"`
}

// Branch mirrors the subset of Neon's branch object this provider reads.
type Branch struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Default bool   `json:"default"`
}

type listBranchesResponse struct {
	Branches []Branch `json:"branches"`
}

// Database mirrors the subset of Neon's database object this provider manages.
type Database struct {
	Name      string `json:"name"`
	OwnerName string `json:"owner_name"`
	BranchID  string `json:"branch_id"`
	CreatedAt string `json:"created_at"`
}

type databaseEnvelope struct {
	Database Database `json:"database"`
}

type createDatabaseRequest struct {
	Database createDatabaseSettings `json:"database"`
}

type createDatabaseSettings struct {
	Name      string `json:"name"`
	OwnerName string `json:"owner_name,omitempty"`
}

type updateDatabaseRequest struct {
	Database updateDatabaseSettings `json:"database"`
}

type updateDatabaseSettings struct {
	OwnerName string `json:"owner_name,omitempty"`
}

// Role mirrors the subset of Neon's role object this provider manages. Password
// is only ever populated by CreateRole or RevealRolePassword — a plain GetRole
// never includes it.
type Role struct {
	Name      string `json:"name"`
	BranchID  string `json:"branch_id"`
	Password  string `json:"password"`
	Protected bool   `json:"protected"`
	CreatedAt string `json:"created_at"`
}

type roleEnvelope struct {
	Role Role `json:"role"`
}

type createRoleRequest struct {
	Role createRoleSettings `json:"role"`
}

type createRoleSettings struct {
	Name string `json:"name"`
}

type revealRolePasswordResponse struct {
	Password string `json:"password"`
}

// APIError represents a non-2xx response from the Neon API.
type APIError struct {
	StatusCode int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("neon api error (status %d, code %q): %s", e.StatusCode, e.Code, e.Message)
}

// IsNotFound returns true if the supplied error is a Neon 404 response.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// IsRateLimited returns true if the supplied error is a Neon 429 response.
func IsRateLimited(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best effort close

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{StatusCode: resp.StatusCode}
		_ = json.Unmarshal(respBody, apiErr)
		if apiErr.Message == "" {
			const maxSnippet = 200
			snippet := respBody
			if len(snippet) > maxSnippet {
				snippet = snippet[:maxSnippet]
			}
			apiErr.Message = fmt.Sprintf("non-JSON response body: %q", snippet)
		}
		return apiErr
	}

	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}
	return nil
}

// maxProjectsPageSize is the largest page Neon's List Projects endpoint
// accepts per its "limit" query parameter (documented range: 1-400).
const maxProjectsPageSize = 400

// ListProjectsByName searches for projects whose name or id contains the given
// substring, per Neon's "search" query parameter (partial match, not exact).
// Callers that need an exact name match must filter the results themselves.
// Results are paginated via Neon's cursor mechanism until exhausted, so an
// account with more than one page of matches won't have later matches missed.
func (c *Client) ListProjectsByName(ctx context.Context, name string) ([]Project, error) {
	var all []Project
	cursor := ""
	for {
		q := url.Values{}
		q.Set("search", name)
		q.Set("limit", strconv.Itoa(maxProjectsPageSize))
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var resp listProjectsResponse
		if err := c.do(ctx, http.MethodGet, "/projects?"+q.Encode(), nil, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Projects...)

		if resp.Pagination == nil || resp.Pagination.Cursor == "" || resp.Pagination.Cursor == cursor || len(resp.Projects) < maxProjectsPageSize {
			break
		}
		cursor = resp.Pagination.Cursor
	}
	return all, nil
}

// CreateProject creates a new Neon project.
func (c *Client) CreateProject(ctx context.Context, name, regionID string, pgVersion int) (*CreateProjectResponse, error) {
	req := createProjectRequest{Project: createProjectSettings{Name: name, RegionID: regionID, PgVersion: pgVersion}}
	var resp CreateProjectResponse
	if err := c.do(ctx, http.MethodPost, "/projects", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetProject fetches a Neon project by ID. Returns an *APIError with StatusCode 404 if it does not exist.
func (c *Client) GetProject(ctx context.Context, id string) (*Project, error) {
	var resp projectEnvelope
	if err := c.do(ctx, http.MethodGet, "/projects/"+url.PathEscape(id), nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Project, nil
}

// UpdateProject updates the mutable fields of a Neon project (currently just its name).
func (c *Client) UpdateProject(ctx context.Context, id, name string) (*Project, error) {
	req := updateProjectRequest{Project: updateProjectSettings{Name: name}}
	var resp projectEnvelope
	if err := c.do(ctx, http.MethodPatch, "/projects/"+url.PathEscape(id), req, &resp); err != nil {
		return nil, err
	}
	return &resp.Project, nil
}

// DeleteProject deletes a Neon project by ID. It is a no-op if the project does not exist.
func (c *Client) DeleteProject(ctx context.Context, id string) error {
	err := c.do(ctx, http.MethodDelete, "/projects/"+url.PathEscape(id), nil, nil)
	if err != nil && IsNotFound(err) {
		return nil
	}
	return err
}

// ListBranches lists every branch of a Neon project.
func (c *Client) ListBranches(ctx context.Context, projectID string) ([]Branch, error) {
	var resp listBranchesResponse
	path := "/projects/" + url.PathEscape(projectID) + "/branches"
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Branches, nil
}

// DefaultBranch returns the project's default branch. Returns an error if the
// project has no branch marked default, which should not happen in practice —
// every Neon project has exactly one.
func (c *Client) DefaultBranch(ctx context.Context, projectID string) (*Branch, error) {
	branches, err := c.ListBranches(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for i := range branches {
		if branches[i].Default {
			return &branches[i], nil
		}
	}
	return nil, fmt.Errorf("project %s has no default branch", projectID)
}

func databasePath(projectID, branchID, name string) string {
	path := "/projects/" + url.PathEscape(projectID) + "/branches/" + url.PathEscape(branchID) + "/databases"
	if name != "" {
		path += "/" + url.PathEscape(name)
	}
	return path
}

// CreateDatabase creates a new database on the given project/branch.
func (c *Client) CreateDatabase(ctx context.Context, projectID, branchID, name, ownerName string) (*Database, error) {
	req := createDatabaseRequest{Database: createDatabaseSettings{Name: name, OwnerName: ownerName}}
	var resp databaseEnvelope
	if err := c.do(ctx, http.MethodPost, databasePath(projectID, branchID, ""), req, &resp); err != nil {
		return nil, err
	}
	return &resp.Database, nil
}

// GetDatabase fetches a database by name. Returns an *APIError with StatusCode 404 if it does not exist.
func (c *Client) GetDatabase(ctx context.Context, projectID, branchID, name string) (*Database, error) {
	var resp databaseEnvelope
	if err := c.do(ctx, http.MethodGet, databasePath(projectID, branchID, name), nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Database, nil
}

// UpdateDatabase updates the mutable fields of a database (currently just its owner).
func (c *Client) UpdateDatabase(ctx context.Context, projectID, branchID, name, ownerName string) (*Database, error) {
	req := updateDatabaseRequest{Database: updateDatabaseSettings{OwnerName: ownerName}}
	var resp databaseEnvelope
	if err := c.do(ctx, http.MethodPatch, databasePath(projectID, branchID, name), req, &resp); err != nil {
		return nil, err
	}
	return &resp.Database, nil
}

// DeleteDatabase deletes a database by name. It is a no-op if the database does not exist.
func (c *Client) DeleteDatabase(ctx context.Context, projectID, branchID, name string) error {
	err := c.do(ctx, http.MethodDelete, databasePath(projectID, branchID, name), nil, nil)
	if err != nil && IsNotFound(err) {
		return nil
	}
	return err
}

func rolePath(projectID, branchID, name string) string {
	path := "/projects/" + url.PathEscape(projectID) + "/branches/" + url.PathEscape(branchID) + "/roles"
	if name != "" {
		path += "/" + url.PathEscape(name)
	}
	return path
}

// CreateRole creates a new role on the given project/branch. Unlike Database,
// Neon returns the role's password directly in the create response.
func (c *Client) CreateRole(ctx context.Context, projectID, branchID, name string) (*Role, error) {
	req := createRoleRequest{Role: createRoleSettings{Name: name}}
	var resp roleEnvelope
	if err := c.do(ctx, http.MethodPost, rolePath(projectID, branchID, ""), req, &resp); err != nil {
		return nil, err
	}
	return &resp.Role, nil
}

// GetRole fetches a role by name. Returns an *APIError with StatusCode 404 if it does not exist.
// The returned Role's Password is always empty — use RevealRolePassword to fetch it.
func (c *Client) GetRole(ctx context.Context, projectID, branchID, name string) (*Role, error) {
	var resp roleEnvelope
	if err := c.do(ctx, http.MethodGet, rolePath(projectID, branchID, name), nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Role, nil
}

// RevealRolePassword fetches the current password for an existing role.
func (c *Client) RevealRolePassword(ctx context.Context, projectID, branchID, name string) (string, error) {
	var resp revealRolePasswordResponse
	if err := c.do(ctx, http.MethodGet, rolePath(projectID, branchID, name)+"/reveal_password", nil, &resp); err != nil {
		return "", err
	}
	return resp.Password, nil
}

// DeleteRole deletes a role by name. It is a no-op if the role does not exist.
func (c *Client) DeleteRole(ctx context.Context, projectID, branchID, name string) error {
	err := c.do(ctx, http.MethodDelete, rolePath(projectID, branchID, name), nil, nil)
	if err != nil && IsNotFound(err) {
		return nil
	}
	return err
}
