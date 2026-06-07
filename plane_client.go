package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
)

type planeClient struct {
	baseURL       string
	apiKey        string
	workspaceSlug string
	httpClient    *http.Client
}

type meResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
}

type resolvedWorkItem struct {
	ProjectIdentifier string `json:"project_identifier"`
	WorkItemNumber    string `json:"work_item_number"`
	ReadableID        string `json:"readable_id"`
	ProjectID         string `json:"project_id,omitempty"`
	WorkItemID        string `json:"work_item_id,omitempty"`
	Name              string `json:"name,omitempty"`
	StateID           string `json:"state_id,omitempty"`
}

type projectSummary struct {
	ID              string `json:"id"`
	Identifier      string `json:"identifier"`
	Name            string `json:"name"`
	DescriptionText string `json:"description_text,omitempty"`
	ArchivedAt      string `json:"archived_at,omitempty"`
}

type stateSummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Group   string `json:"group"`
	Color   string `json:"color,omitempty"`
	Default bool   `json:"default"`
	Slug    string `json:"slug,omitempty"`
}

type memberSummary struct {
	ID          string `json:"id"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	FirstName   string `json:"first_name,omitempty"`
	LastName    string `json:"last_name,omitempty"`
}

func newPlaneClient(eff effectiveConfig, httpClient *http.Client) planeClient {
	return planeClient{
		baseURL:       strings.TrimRight(eff.BaseURL.Value, "/"),
		apiKey:        eff.APIKey.Secret,
		workspaceSlug: eff.WorkspaceSlug.Value,
		httpClient:    httpClient,
	}
}

func (c planeClient) getMe(ctx context.Context) (meResponse, *cliError) {
	var me meResponse
	err := c.getJSON(ctx, "/api/v1/users/me/", &me)
	return me, err
}

func (c planeClient) listProjects(ctx context.Context) ([]projectSummary, *cliError) {
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/projects/"
	var raw any
	if err := c.getJSON(ctx, endpoint, &raw); err != nil {
		return nil, err
	}
	items := extractResultMaps(raw)
	projects := make([]projectSummary, 0, len(items))
	for _, item := range items {
		projects = append(projects, projectSummary{
			ID:              stringFromMap(item, "id"),
			Identifier:      stringFromMap(item, "identifier"),
			Name:            stringFromMap(item, "name"),
			DescriptionText: stringFromMap(item, "description_text", "description"),
			ArchivedAt:      stringFromMap(item, "archived_at"),
		})
	}
	return projects, nil
}

func (c planeClient) getProjectByRef(ctx context.Context, ref string) (projectSummary, *cliError) {
	projects, err := c.listProjects(ctx)
	if err != nil {
		return projectSummary{}, err
	}
	for _, project := range projects {
		if sameRef(project.ID, ref) || sameRef(project.Identifier, ref) || sameRef(project.Name, ref) {
			return project, nil
		}
	}
	return projectSummary{}, newError("PROJECT_NOT_FOUND", "Project not found: "+ref, "Use plane-cli project list --format json to find the project identifier or UUID.", true, "plane-cli project list --format json")
}

func (c planeClient) listProjectStates(ctx context.Context, projectID string) ([]stateSummary, *cliError) {
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/projects/" + url.PathEscape(projectID) + "/states/"
	var raw any
	if err := c.getJSON(ctx, endpoint, &raw); err != nil {
		if err.Code == "RESOURCE_NOT_FOUND" {
			return nil, newError("PROJECT_NOT_FOUND", "Project not found: "+projectID, "Use plane-cli project list --format json to find a valid project.", true)
		}
		return nil, err
	}
	items := extractResultMaps(raw)
	states := make([]stateSummary, 0, len(items))
	for _, item := range items {
		states = append(states, stateSummary{
			ID:      stringFromMap(item, "id"),
			Name:    stringFromMap(item, "name"),
			Group:   stringFromMap(item, "group"),
			Color:   stringFromMap(item, "color"),
			Default: boolFromMap(item, "default"),
			Slug:    stringFromMap(item, "slug"),
		})
	}
	return states, nil
}

func (c planeClient) listProjectMembers(ctx context.Context, projectID string) ([]memberSummary, *cliError) {
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/projects/" + url.PathEscape(projectID) + "/members/"
	var raw any
	if err := c.getJSON(ctx, endpoint, &raw); err != nil {
		if err.Code == "RESOURCE_NOT_FOUND" {
			return nil, newError("PROJECT_NOT_FOUND", "Project not found: "+projectID, "Use plane-cli project list --format json to find a valid project.", true)
		}
		return nil, err
	}
	items := extractResultMaps(raw)
	members := make([]memberSummary, 0, len(items))
	for _, item := range items {
		if nested, ok := item["member"].(map[string]any); ok {
			item = nested
		}
		if nested, ok := item["user"].(map[string]any); ok {
			item = nested
		}
		members = append(members, memberSummary{
			ID:          stringFromMap(item, "id"),
			Email:       stringFromMap(item, "email"),
			DisplayName: stringFromMap(item, "display_name"),
			FirstName:   stringFromMap(item, "first_name"),
			LastName:    stringFromMap(item, "last_name"),
		})
	}
	return members, nil
}

func (c planeClient) resolveWorkItem(ctx context.Context, projectIdentifier, number string) (resolvedWorkItem, *cliError) {
	ref := projectIdentifier + "-" + number
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/work-items/" + url.PathEscape(ref) + "/"
	var raw map[string]any
	if err := c.getJSON(ctx, endpoint, &raw); err != nil {
		if err.Code == "RESOURCE_NOT_FOUND" {
			return resolvedWorkItem{}, newError("WORK_ITEM_NOT_FOUND", "Work item not found: "+ref, "Check the readable ID and workspace slug.", true, "plane-cli resolve "+ref+" --format json")
		}
		return resolvedWorkItem{}, err
	}
	item := resolvedWorkItem{
		ProjectIdentifier: projectIdentifier,
		WorkItemNumber:    number,
		ReadableID:        ref,
		ProjectID:         stringFromMap(raw, "project_id", "project"),
		WorkItemID:        stringFromMap(raw, "id", "work_item_id", "issue_id"),
		Name:              stringFromMap(raw, "name"),
		StateID:           stringFromMap(raw, "state_id", "state"),
	}
	if item.ProjectID == "" {
		if project, ok := raw["project"].(map[string]any); ok {
			item.ProjectID = stringFromMap(project, "id")
		}
		if project, ok := raw["project_detail"].(map[string]any); ok {
			item.ProjectID = stringFromMap(project, "id")
		}
	}
	return item, nil
}

func (c planeClient) getJSON(ctx context.Context, endpoint string, out any) *cliError {
	if c.baseURL == "" {
		return newError("MISSING_BASE_URL", "PLANE_BASE_URL is not configured.", "Set PLANE_BASE_URL or run: plane-cli config set base_url <url>", true)
	}
	base, err := url.Parse(c.baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return newError("MISSING_BASE_URL", "PLANE_BASE_URL must be an absolute URL.", "Set PLANE_BASE_URL to a URL like https://api.plane.so.", true, "plane-cli config set base_url https://api.plane.so")
	}
	requestURL := *base
	requestURL.Path = path.Join(base.Path, endpoint)
	if strings.HasSuffix(endpoint, "/") && !strings.HasSuffix(requestURL.Path, "/") {
		requestURL.Path += "/"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return newError("API_UNREACHABLE", "Could not build Plane API request.", "Check PLANE_BASE_URL.", true)
	}
	req.Header.Set("X-API-Key", c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return newError("API_UNREACHABLE", "Could not reach Plane API.", "Check PLANE_BASE_URL and network connectivity.", true)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return newError("API_RESPONSE_INVALID", "Plane API returned invalid JSON.", "Retry or check the Plane API endpoint.", true)
		}
		return nil
	case http.StatusUnauthorized:
		return newError("INVALID_API_KEY", "Plane API key is invalid or revoked.", "Regenerate the token in Plane settings and set PLANE_API_KEY.", true)
	case http.StatusForbidden:
		return newError("INSUFFICIENT_PERMISSIONS", "Plane API key does not have sufficient permissions.", "Check your Plane workspace/project role or token scope.", true)
	case http.StatusNotFound:
		return newError("RESOURCE_NOT_FOUND", "Plane resource was not found.", "Check workspace slug, identifiers, and base URL.", true)
	default:
		return newError("API_UNREACHABLE", fmt.Sprintf("Plane API returned HTTP %d.", resp.StatusCode), "Check Plane API status and request configuration.", true)
	}
}

func extractResultMaps(raw any) []map[string]any {
	var values []any
	switch v := raw.(type) {
	case []any:
		values = v
	case map[string]any:
		if results, ok := v["results"].([]any); ok {
			values = results
		} else {
			values = []any{v}
		}
	}
	items := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if item, ok := value.(map[string]any); ok {
			items = append(items, item)
		}
	}
	return items
}

func sameRef(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func stringFromMap(m map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := m[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case string:
			return v
		case float64:
			return fmt.Sprintf("%.0f", v)
		case map[string]any:
			if id := stringFromMap(v, "id"); id != "" {
				return id
			}
		}
	}
	return ""
}

func boolFromMap(m map[string]any, key string) bool {
	value, ok := m[key]
	if !ok {
		return false
	}
	b, ok := value.(bool)
	return ok && b
}
