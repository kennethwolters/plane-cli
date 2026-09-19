package cli

import (
	"context"
	"net/url"
)

type pageSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ParentID  string `json:"parent_id,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type pageDetail struct {
	pageSummary
	DescriptionText string `json:"description_text"`
}

func (c planeClient) listProjectPages(ctx context.Context, projectID string) ([]pageSummary, *cliError) {
	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/projects/" + url.PathEscape(projectID) + "/pages/"
	var raw any
	if err := c.getJSON(ctx, endpoint, &raw); err != nil {
		if err.Code == "RESOURCE_NOT_FOUND" {
			return nil, newError("PROJECT_NOT_FOUND", "Project not found: "+projectID, "Use plane-cli project list --format json to find a valid project.", true)
		}
		return nil, err
	}
	items := extractResultMaps(raw)
	pages := make([]pageSummary, 0, len(items))
	for _, item := range items {
		pages = append(pages, pageSummary{
			ID:        stringFromMap(item, "id"),
			Name:      stringFromMap(item, "name"),
			ParentID:  stringFromMap(item, "parent_id"),
			CreatedAt: stringFromMap(item, "created_at"),
			UpdatedAt: stringFromMap(item, "updated_at"),
		})
	}
	return pages, nil
}

func (c planeClient) getProjectPageByRef(ctx context.Context, projectID, ref string) (pageDetail, *cliError) {
	pages, err := c.listProjectPages(ctx, projectID)
	if err != nil {
		return pageDetail{}, err
	}
	var match *pageSummary
	for i := range pages {
		if sameRef(pages[i].ID, ref) || sameRef(pages[i].Name, ref) {
			if match != nil {
				return pageDetail{}, newError("AMBIGUOUS_PAGE", "Page reference is ambiguous: "+ref, "Use the page UUID from plane-cli page list.", false)
			}
			match = &pages[i]
		}
	}
	if match == nil {
		return pageDetail{}, newError("PAGE_NOT_FOUND", "Page not found: "+ref, "Use plane-cli page list --project <project> --format json to find the page UUID.", true)
	}

	endpoint := "/api/v1/workspaces/" + url.PathEscape(c.workspaceSlug) + "/projects/" + url.PathEscape(projectID) + "/pages/" + url.PathEscape(match.ID) + "/"
	var raw map[string]any
	if err := c.getJSON(ctx, endpoint, &raw); err != nil {
		if err.Code == "RESOURCE_NOT_FOUND" {
			return pageDetail{}, newError("PAGE_NOT_FOUND", "Page not found: "+ref, "Refresh the page list and retry with its UUID.", true)
		}
		return pageDetail{}, err
	}
	return pageDetail{
		pageSummary: pageSummary{
			ID:        stringFromMap(raw, "id"),
			Name:      stringFromMap(raw, "name"),
			ParentID:  stringFromMap(raw, "parent_id"),
			CreatedAt: stringFromMap(raw, "created_at"),
			UpdatedAt: stringFromMap(raw, "updated_at"),
		},
		DescriptionText: stringFromMap(raw, "description_stripped"),
	}, nil
}
