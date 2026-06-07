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
