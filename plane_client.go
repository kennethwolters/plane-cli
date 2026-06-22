package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
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

type labelSummary struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

type workItemSummary struct {
	ProjectIdentifier string   `json:"project_identifier,omitempty"`
	ReadableID        string   `json:"readable_id,omitempty"`
	ProjectID         string   `json:"project_id"`
	WorkItemID        string   `json:"work_item_id"`
	SequenceID        string   `json:"sequence_id,omitempty"`
	Name              string   `json:"name"`
	DescriptionHTML   string   `json:"description_html,omitempty"`
	StateID           string   `json:"state_id,omitempty"`
	StateName         string   `json:"state_name,omitempty"`
	StateGroup        string   `json:"state_group,omitempty"`
	Priority          string   `json:"priority,omitempty"`
	AssigneeIDs       []string `json:"assignee_ids"`
	LabelIDs          []string `json:"label_ids"`
	ParentID          string   `json:"parent_id,omitempty"`
	CreatedAt         string   `json:"created_at,omitempty"`
	UpdatedAt         string   `json:"updated_at,omitempty"`
}

type commentSummary struct {
	ID          string `json:"id"`
	CommentHTML string `json:"comment_html,omitempty"`
	Excerpt     string `json:"excerpt,omitempty"`
	AuthorID    string `json:"author_id,omitempty"`
	AuthorName  string `json:"author_name,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	Permalink   string `json:"permalink,omitempty"`
}

type relationSummary struct {
	ID                string `json:"id"`
	RelationType      string `json:"relation_type"`
	RelatedWorkItemID string `json:"related_work_item_id"`
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

func (c planeClient) listProjectLabels(ctx context.Context, projectID string) ([]labelSummary, *cliError) {
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/projects/" + url.PathEscape(projectID) + "/labels/"
	var raw any
	if err := c.getJSON(ctx, endpoint, &raw); err != nil {
		if err.Code == "RESOURCE_NOT_FOUND" {
			return nil, newError("PROJECT_NOT_FOUND", "Project not found: "+projectID, "Use plane-cli project list --format json to find a valid project.", true)
		}
		return nil, err
	}
	items := extractResultMaps(raw)
	labels := make([]labelSummary, 0, len(items))
	for _, item := range items {
		labels = append(labels, labelSummary{
			ID:    stringFromMap(item, "id"),
			Name:  stringFromMap(item, "name"),
			Color: stringFromMap(item, "color"),
		})
	}
	return labels, nil
}

func (c planeClient) listWorkItems(ctx context.Context, project projectSummary, stateGroup string, limit int) ([]workItemSummary, *cliError) {
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/projects/" + url.PathEscape(project.ID) + "/work-items/"
	var raw any
	if err := c.getJSON(ctx, endpoint, &raw); err != nil {
		if err.Code == "RESOURCE_NOT_FOUND" {
			return nil, newError("PROJECT_NOT_FOUND", "Project not found: "+project.ID, "Use plane-cli project list --format json to find a valid project.", true)
		}
		return nil, err
	}
	items := extractResultMaps(raw)
	workItems := make([]workItemSummary, 0, len(items))
	var stateGroupsByID map[string]string
	statesLoaded := false
	loadStateGroups := func(required bool) (map[string]string, *cliError) {
		if statesLoaded {
			return stateGroupsByID, nil
		}
		statesLoaded = true
		states, err := c.listProjectStates(ctx, project.ID)
		if err != nil {
			if required {
				return nil, err
			}
			return nil, nil
		}
		stateGroupsByID = make(map[string]string, len(states))
		for _, state := range states {
			if state.ID != "" && state.Group != "" {
				stateGroupsByID[state.ID] = state.Group
			}
		}
		return stateGroupsByID, nil
	}
	for _, item := range items {
		mapped := mapWorkItem(item, project)
		if mapped.StateGroup == "" && mapped.StateID != "" {
			groups, err := loadStateGroups(stateGroup != "")
			if err != nil {
				return nil, err
			}
			mapped = workItemWithResolvedStateGroup(mapped, groups)
		}
		if stateGroup != "" && !sameRef(mapped.StateGroup, stateGroup) {
			continue
		}
		workItems = append(workItems, mapped)
		if limit > 0 && len(workItems) >= limit {
			break
		}
	}
	return workItems, nil
}

func workItemWithResolvedStateGroup(item workItemSummary, stateGroupsByID map[string]string) workItemSummary {
	if item.StateGroup != "" || item.StateID == "" {
		return item
	}
	if group := stateGroupsByID[item.StateID]; group != "" {
		item.StateGroup = group
	}
	return item
}

func (c planeClient) getWorkItemByRef(ctx context.Context, ref string) (workItemSummary, *cliError) {
	projectIdentifier, _, parseErr := parseReadableRef(ref)
	if parseErr != nil {
		return workItemSummary{}, parseErr
	}
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/work-items/" + url.PathEscape(strings.ToUpper(ref)) + "/"
	var raw map[string]any
	if err := c.getJSON(ctx, endpoint, &raw); err != nil {
		if err.Code == "RESOURCE_NOT_FOUND" {
			return workItemSummary{}, newError("WORK_ITEM_NOT_FOUND", "Work item not found: "+strings.ToUpper(ref), "Check the readable ID and workspace slug.", true, "plane-cli work get "+strings.ToUpper(ref)+" --format json")
		}
		return workItemSummary{}, err
	}
	project := projectSummary{ID: stringFromMap(raw, "project_id", "project"), Identifier: projectIdentifier}
	return mapWorkItem(raw, project), nil
}

func (c planeClient) createWorkItem(ctx context.Context, project projectSummary, changes map[string]any) (workItemSummary, *cliError) {
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/projects/" + url.PathEscape(project.ID) + "/work-items/"
	var raw map[string]any
	if err := c.postJSON(ctx, endpoint, changes, &raw); err != nil {
		return workItemSummary{}, err
	}
	return mapWorkItem(raw, project), nil
}

func (c planeClient) updateWorkItem(ctx context.Context, projectID, workItemID string, changes map[string]any) (workItemSummary, *cliError) {
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/projects/" + url.PathEscape(projectID) + "/work-items/" + url.PathEscape(workItemID) + "/"
	var raw map[string]any
	if err := c.patchJSON(ctx, endpoint, changes, &raw); err != nil {
		if err.Code == "RESOURCE_NOT_FOUND" {
			return workItemSummary{}, newError("WORK_ITEM_NOT_FOUND", "Work item not found: "+workItemID, "Resolve the work item again and retry.", true)
		}
		return workItemSummary{}, err
	}
	return mapWorkItem(raw, projectSummary{ID: projectID}), nil
}

func (c planeClient) createWorkItemComment(ctx context.Context, projectID, workItemID, html string) (commentSummary, *cliError) {
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/projects/" + url.PathEscape(projectID) + "/work-items/" + url.PathEscape(workItemID) + "/comments/"
	var raw map[string]any
	if err := c.postJSON(ctx, endpoint, map[string]any{"comment_html": html}, &raw); err != nil {
		return commentSummary{}, err
	}
	return commentSummary{ID: stringFromMap(raw, "id"), CommentHTML: stringFromMap(raw, "comment_html")}, nil
}

func (c planeClient) listWorkItemComments(ctx context.Context, projectID, workItemID string, limit int) ([]commentSummary, *cliError) {
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/projects/" + url.PathEscape(projectID) + "/work-items/" + url.PathEscape(workItemID) + "/comments/"
	var raw any
	if err := c.getJSON(ctx, endpoint, &raw); err != nil {
		if err.Code == "RESOURCE_NOT_FOUND" {
			return nil, newError("WORK_ITEM_NOT_FOUND", "Work item not found: "+workItemID, "Resolve the work item again and retry.", true)
		}
		return nil, err
	}
	items := extractResultMaps(raw)
	comments := make([]commentSummary, 0, len(items))
	for _, item := range items {
		comments = append(comments, mapComment(item))
		if limit > 0 && len(comments) >= limit {
			break
		}
	}
	return comments, nil
}

func (c planeClient) listWorkItemChildren(ctx context.Context, item workItemSummary, limit int) ([]workItemSummary, *cliError) {
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/projects/" + url.PathEscape(item.ProjectID) + "/work-items/" + url.PathEscape(item.WorkItemID) + "/children/"
	var raw any
	if err := c.getJSON(ctx, endpoint, &raw); err != nil {
		return nil, err
	}
	rawItems := extractResultMaps(raw)
	children := make([]workItemSummary, 0, len(rawItems))
	project := projectSummary{ID: item.ProjectID, Identifier: item.ProjectIdentifier}
	for _, rawItem := range rawItems {
		children = append(children, mapWorkItem(rawItem, project))
		if limit > 0 && len(children) >= limit {
			break
		}
	}
	return children, nil
}

func (c planeClient) updateWorkItemParent(ctx context.Context, child workItemSummary, parentID string) (workItemSummary, *cliError) {
	changes := map[string]any{"parent": parentID}
	if parentID == "" {
		changes["parent"] = nil
	}
	return c.updateWorkItem(ctx, child.ProjectID, child.WorkItemID, changes)
}

func (c planeClient) createWorkItemRelation(ctx context.Context, item workItemSummary, relationType string, related workItemSummary) (relationSummary, *cliError) {
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/projects/" + url.PathEscape(item.ProjectID) + "/work-items/" + url.PathEscape(item.WorkItemID) + "/relations/"
	body := map[string]any{"relation_type": relationType, "related_work_item": related.WorkItemID}
	var raw map[string]any
	if err := c.postJSON(ctx, endpoint, body, &raw); err != nil {
		return relationSummary{}, err
	}
	return mapRelation(raw), nil
}

func (c planeClient) listWorkItemRelations(ctx context.Context, item workItemSummary) ([]relationSummary, *cliError) {
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/projects/" + url.PathEscape(item.ProjectID) + "/work-items/" + url.PathEscape(item.WorkItemID) + "/relations/"
	var raw any
	if err := c.getJSON(ctx, endpoint, &raw); err != nil {
		return nil, err
	}
	items := extractResultMaps(raw)
	relations := make([]relationSummary, 0, len(items))
	for _, rawItem := range items {
		relations = append(relations, mapRelation(rawItem))
	}
	return relations, nil
}

func (c planeClient) deleteWorkItemRelation(ctx context.Context, item workItemSummary, relationID string) *cliError {
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/projects/" + url.PathEscape(item.ProjectID) + "/work-items/" + url.PathEscape(item.WorkItemID) + "/relations/" + url.PathEscape(relationID) + "/"
	return c.deleteJSON(ctx, endpoint, nil)
}

func (c planeClient) firstStateForGroup(ctx context.Context, projectID, group string) (stateSummary, *cliError) {
	states, err := c.listProjectStates(ctx, projectID)
	if err != nil {
		return stateSummary{}, err
	}
	for _, state := range states {
		if sameRef(state.Group, group) && state.Default {
			return state, nil
		}
	}
	for _, state := range states {
		if sameRef(state.Group, group) {
			return state, nil
		}
	}
	return stateSummary{}, newError("STATE_NOT_FOUND", "No state found for group: "+group, "Create a Plane state in that group or choose another lifecycle command.", false)
}

func (c planeClient) verifyWorkItemExists(ctx context.Context, item workItemSummary) (bool, *cliError) {
	if item.ReadableID == "" {
		return item.WorkItemID != "", nil
	}
	verified, err := c.getWorkItemByRef(ctx, item.ReadableID)
	if err != nil {
		return false, err
	}
	return verified.WorkItemID == item.WorkItemID, nil
}

func (c planeClient) verifyWorkItemChanges(ctx context.Context, item workItemSummary, changes map[string]any) (bool, *cliError) {
	verified, err := c.verifyFreshWorkItem(ctx, item)
	if err != nil {
		return false, err
	}
	for key, want := range changes {
		switch key {
		case "name":
			if verified.Name != fmt.Sprint(want) {
				return false, nil
			}
		case "description_html":
			if verified.DescriptionHTML != fmt.Sprint(want) {
				return false, nil
			}
		case "priority":
			if verified.Priority != fmt.Sprint(want) {
				return false, nil
			}
		case "labels":
			if !sameStringSet(verified.LabelIDs, anyStringSlice(want)) {
				return false, nil
			}
		case "assignees":
			if !sameStringSet(verified.AssigneeIDs, anyStringSlice(want)) {
				return false, nil
			}
		case "state":
			if verified.StateID != fmt.Sprint(want) {
				return false, nil
			}
		}
	}
	return true, nil
}

func (c planeClient) verifyWorkItemStateGroup(ctx context.Context, item workItemSummary, group string) (bool, *cliError) {
	verified, err := c.verifyFreshWorkItem(ctx, item)
	if err != nil {
		return false, err
	}
	if sameRef(verified.StateGroup, group) {
		return true, nil
	}
	stateID := verified.StateID
	if stateID == "" {
		return false, nil
	}
	states, err := c.listProjectStates(ctx, verified.ProjectID)
	if err != nil {
		return false, err
	}
	for _, state := range states {
		if state.ID == stateID {
			return sameRef(state.Group, group), nil
		}
	}
	return false, nil
}

func (c planeClient) verifyFreshWorkItem(ctx context.Context, item workItemSummary) (workItemSummary, *cliError) {
	if item.ReadableID != "" {
		return c.getWorkItemByRef(ctx, item.ReadableID)
	}
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/projects/" + url.PathEscape(item.ProjectID) + "/work-items/" + url.PathEscape(item.WorkItemID) + "/"
	var raw map[string]any
	if err := c.getJSON(ctx, endpoint, &raw); err != nil {
		return workItemSummary{}, err
	}
	return mapWorkItem(raw, projectSummary{ID: item.ProjectID, Identifier: item.ProjectIdentifier}), nil
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
	return c.requestJSON(ctx, http.MethodGet, endpoint, nil, out, http.StatusOK)
}

func (c planeClient) postJSON(ctx context.Context, endpoint string, body any, out any) *cliError {
	return c.requestJSON(ctx, http.MethodPost, endpoint, body, out, http.StatusOK, http.StatusCreated)
}

func (c planeClient) patchJSON(ctx context.Context, endpoint string, body any, out any) *cliError {
	return c.requestJSON(ctx, http.MethodPatch, endpoint, body, out, http.StatusOK)
}

func (c planeClient) deleteJSON(ctx context.Context, endpoint string, out any) *cliError {
	return c.requestJSON(ctx, http.MethodDelete, endpoint, nil, out, http.StatusOK, http.StatusNoContent)
}

func (c planeClient) requestJSON(ctx context.Context, method, endpoint string, body any, out any, successStatuses ...int) *cliError {
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
	var requestBody *bytes.Reader
	if body == nil {
		requestBody = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			return newError("VALIDATION_FAILED", "Could not encode request body as JSON.", "Check command flags and retry.", false)
		}
		requestBody = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), requestBody)
	if err != nil {
		return newError("API_UNREACHABLE", "Could not build Plane API request.", "Check PLANE_BASE_URL.", true)
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return newError("API_UNREACHABLE", "Could not reach Plane API.", "Check PLANE_BASE_URL and network connectivity.", true)
	}
	defer resp.Body.Close()

	for _, success := range successStatuses {
		if resp.StatusCode == success {
			if out == nil {
				return nil
			}
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				return newError("API_RESPONSE_INVALID", "Plane API returned invalid JSON.", "Retry or check the Plane API endpoint.", true)
			}
			return nil
		}
	}

	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		retryAfter := 0
		if header := strings.TrimSpace(resp.Header.Get("Retry-After")); header != "" {
			if parsed, err := strconv.Atoi(header); err == nil && parsed > 0 {
				retryAfter = parsed
			}
		}
		return newRateLimitError(retryAfter)
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return newError("VALIDATION_FAILED", fmt.Sprintf("Plane API rejected the request with HTTP %d.", resp.StatusCode), "Check command flags and required Plane fields.", false)
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

func mapWorkItem(raw map[string]any, project projectSummary) workItemSummary {
	projectID := stringFromMap(raw, "project_id", "project")
	if projectID == "" {
		projectID = project.ID
	}
	projectIdentifier := project.Identifier
	sequenceID := stringFromMap(raw, "sequence_id", "issue_identifier")
	readableID := stringFromMap(raw, "readable_id", "identifier")
	if readableID == "" && projectIdentifier != "" && sequenceID != "" {
		readableID = projectIdentifier + "-" + sequenceID
	}
	stateID := stringFromMap(raw, "state_id", "state")
	stateName := stringFromMap(raw, "state_name")
	stateGroup := stringFromMap(raw, "state_group")
	if state, ok := raw["state"].(map[string]any); ok {
		if stateID == "" {
			stateID = stringFromMap(state, "id")
		}
		if stateName == "" {
			stateName = stringFromMap(state, "name")
		}
		stateGroup = stringFromMap(state, "group")
	}
	if state, ok := raw["state_detail"].(map[string]any); ok {
		if stateID == "" {
			stateID = stringFromMap(state, "id")
		}
		if stateName == "" {
			stateName = stringFromMap(state, "name")
		}
		if stateGroup == "" {
			stateGroup = stringFromMap(state, "group")
		}
	}
	return workItemSummary{
		ProjectIdentifier: projectIdentifier,
		ReadableID:        readableID,
		ProjectID:         projectID,
		WorkItemID:        stringFromMap(raw, "id", "work_item_id", "issue_id"),
		SequenceID:        sequenceID,
		Name:              stringFromMap(raw, "name"),
		DescriptionHTML:   stringFromMap(raw, "description_html"),
		StateID:           stateID,
		StateName:         stateName,
		StateGroup:        stateGroup,
		Priority:          stringFromMap(raw, "priority"),
		AssigneeIDs:       stringSliceFromMap(raw, "assignees"),
		LabelIDs:          stringSliceFromMap(raw, "labels"),
		ParentID:          stringFromMap(raw, "parent_id", "parent"),
		CreatedAt:         stringFromMap(raw, "created_at"),
		UpdatedAt:         stringFromMap(raw, "updated_at"),
	}
}

func mapComment(raw map[string]any) commentSummary {
	authorID := ""
	authorName := ""
	if actor, ok := raw["actor"].(map[string]any); ok {
		authorID = stringFromMap(actor, "id")
		authorName = stringFromMap(actor, "display_name", "name", "email")
	}
	if user, ok := raw["user"].(map[string]any); ok {
		if authorID == "" {
			authorID = stringFromMap(user, "id")
		}
		if authorName == "" {
			authorName = stringFromMap(user, "display_name", "name", "email")
		}
	}
	html := stringFromMap(raw, "comment_html", "html", "body")
	return commentSummary{
		ID:          stringFromMap(raw, "id", "comment_id"),
		CommentHTML: html,
		Excerpt:     excerptPlainText(html, 120),
		AuthorID:    firstNonEmpty(stringFromMap(raw, "author_id"), authorID),
		AuthorName:  firstNonEmpty(stringFromMap(raw, "author_name"), authorName),
		CreatedAt:   stringFromMap(raw, "created_at"),
		UpdatedAt:   stringFromMap(raw, "updated_at"),
		Permalink:   stringFromMap(raw, "permalink", "url"),
	}
}

func mapRelation(raw map[string]any) relationSummary {
	return relationSummary{
		ID:                stringFromMap(raw, "id"),
		RelationType:      stringFromMap(raw, "relation_type", "type"),
		RelatedWorkItemID: stringFromMap(raw, "related_work_item", "related_work_item_id", "target"),
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

func stringSliceFromMap(m map[string]any, key string) []string {
	value, ok := m[key]
	if !ok || value == nil {
		return []string{}
	}
	list, ok := value.([]any)
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		switch v := item.(type) {
		case string:
			out = append(out, v)
		case map[string]any:
			if id := stringFromMap(v, "id"); id != "" {
				out = append(out, id)
			}
		}
	}
	return out
}
