package main

import (
	"context"
	"fmt"
	"strings"
)

type searchData struct {
	WorkspaceSlug string          `json:"workspace_slug"`
	Query         string          `json:"query"`
	Project       *projectSummary `json:"project,omitempty"`
	Results       []searchResult  `json:"results"`
	Count         int             `json:"count"`
}

type searchResult struct {
	ProjectIdentifier string `json:"project_identifier,omitempty"`
	ReadableID        string `json:"readable_id,omitempty"`
	ProjectID         string `json:"project_id"`
	WorkItemID        string `json:"work_item_id"`
	SequenceID        string `json:"sequence_id,omitempty"`
	Name              string `json:"name"`
	DescriptionHTML   string `json:"description_html,omitempty"`
	StateID           string `json:"state_id,omitempty"`
	StateName         string `json:"state_name,omitempty"`
	StateGroup        string `json:"state_group,omitempty"`
	Priority          string `json:"priority,omitempty"`
	MatchedField      string `json:"matched_field,omitempty"`
	Excerpt           string `json:"excerpt,omitempty"`
}

func (a app) cmdSearch(ctx context.Context, args []string, configCtx configContext) int {
	format, rest, err := parseFormat(args)
	if err != nil {
		return a.writeCLIError(err, "json")
	}
	projectRef, rest, _ := parseStringFlag(rest, "--project")
	limitText, rest, _ := parseStringFlag(rest, "--max-results")
	rest, includeComments := hasFlag(rest, "--include-comments")
	if len(rest) != 1 {
		return a.usageError("search requires exactly one query", format)
	}
	query := rest[0]
	limit := 20
	if limitText != "" {
		parsed, parseErr := parsePositiveInt(limitText)
		if parseErr != nil {
			return a.writeCLIError(parseErr, format)
		}
		limit = parsed
	}
	eff, client, ok := a.configuredPlaneClient(format, configCtx)
	if !ok {
		return exitError
	}
	var project *projectSummary
	var items []searchResult
	if projectRef != "" {
		p, err := client.getProjectByRef(ctx, projectRef)
		if err != nil {
			return a.writeCLIError(err, format)
		}
		project = &p
		projectItems, err := client.listWorkItems(ctx, p, "", limit)
		if err != nil {
			return a.writeCLIError(err, format)
		}
		items, err = client.searchWorkItems(ctx, projectItems, query, limit, includeComments)
		if err != nil {
			return a.writeCLIError(err, format)
		}
	} else {
		projects, err := client.listProjects(ctx)
		if err != nil {
			return a.writeCLIError(err, format)
		}
		for _, p := range projects {
			projectItems, err := client.listWorkItems(ctx, p, "", limit)
			if err != nil {
				return a.writeCLIError(err, format)
			}
			matches, err := client.searchWorkItems(ctx, projectItems, query, limit-len(items), includeComments)
			if err != nil {
				return a.writeCLIError(err, format)
			}
			items = append(items, matches...)
			if len(items) >= limit {
				break
			}
		}
	}
	data := searchData{WorkspaceSlug: eff.WorkspaceSlug.Value, Query: query, Project: project, Results: items, Count: len(items)}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.search.v1", data))
		return exitOK
	}
	for _, item := range items {
		fmt.Fprintf(a.stdout, "%s\t%s\t%s\n", item.ReadableID, item.StateGroup, item.Name)
	}
	return exitOK
}

func parsePositiveInt(text string) (int, *cliError) {
	value := 0
	for _, r := range text {
		if r < '0' || r > '9' {
			return 0, newError("VALIDATION_FAILED", "Value must be a positive integer: "+text, "Use a positive integer.", false)
		}
		value = value*10 + int(r-'0')
	}
	if value < 1 {
		return 0, newError("VALIDATION_FAILED", "Value must be a positive integer: "+text, "Use a positive integer.", false)
	}
	return value, nil
}

func searchWorkItems(items []workItemSummary, query string, limit int) []searchResult {
	results := []searchResult{}
	needle := strings.ToLower(query)
	for _, item := range items {
		if result, ok := matchWorkItem(item, needle); ok {
			results = append(results, result)
			if len(results) >= limit {
				break
			}
		}
	}
	return results
}

func (c planeClient) searchWorkItems(ctx context.Context, items []workItemSummary, query string, limit int, includeComments bool) ([]searchResult, *cliError) {
	if limit <= 0 {
		return nil, nil
	}
	needle := strings.ToLower(query)
	out := []searchResult{}
	for _, item := range items {
		if result, ok := matchWorkItem(item, needle); ok {
			out = append(out, result)
			if len(out) >= limit {
				break
			}
			continue
		}
		if includeComments {
			comments, err := c.listWorkItemComments(ctx, item.ProjectID, item.WorkItemID, 10)
			if err != nil {
				return nil, err
			}
			for _, comment := range comments {
				if strings.Contains(strings.ToLower(stripHTML(comment.CommentHTML)), needle) {
					result := newSearchResult(item, "comment", comment.Excerpt)
					out = append(out, result)
					break
				}
			}
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func matchWorkItem(item workItemSummary, needle string) (searchResult, bool) {
	switch {
	case strings.Contains(strings.ToLower(item.ReadableID), needle):
		return newSearchResult(item, "readable_id", item.ReadableID), true
	case strings.Contains(strings.ToLower(item.Name), needle):
		return newSearchResult(item, "name", item.Name), true
	case strings.Contains(strings.ToLower(stripHTML(item.DescriptionHTML)), needle):
		return newSearchResult(item, "description_html", excerptPlainText(item.DescriptionHTML, 160)), true
	default:
		return searchResult{}, false
	}
}

func newSearchResult(item workItemSummary, matchedField, excerpt string) searchResult {
	return searchResult{
		ProjectIdentifier: item.ProjectIdentifier,
		ReadableID:        item.ReadableID,
		ProjectID:         item.ProjectID,
		WorkItemID:        item.WorkItemID,
		SequenceID:        item.SequenceID,
		Name:              item.Name,
		DescriptionHTML:   item.DescriptionHTML,
		StateID:           item.StateID,
		StateName:         item.StateName,
		StateGroup:        item.StateGroup,
		Priority:          item.Priority,
		MatchedField:      matchedField,
		Excerpt:           excerpt,
	}
}
