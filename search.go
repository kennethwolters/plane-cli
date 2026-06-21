package main

import (
	"context"
	"fmt"
	"strings"
)

type searchData struct {
	WorkspaceSlug string            `json:"workspace_slug"`
	Query         string            `json:"query"`
	Project       *projectSummary   `json:"project,omitempty"`
	Results       []workItemSummary `json:"results"`
	Count         int               `json:"count"`
}

func (a app) cmdSearch(ctx context.Context, args []string, configCtx configContext) int {
	format, rest, err := parseFormat(args)
	if err != nil {
		return a.writeCLIError(err, "json")
	}
	projectRef, rest, _ := parseStringFlag(rest, "--project")
	limitText, rest, _ := parseStringFlag(rest, "--max-results")
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
	var items []workItemSummary
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
		items = filterWorkItems(projectItems, query, limit)
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
			items = append(items, filterWorkItems(projectItems, query, limit-len(items))...)
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

func filterWorkItems(items []workItemSummary, query string, limit int) []workItemSummary {
	if limit <= 0 {
		return nil
	}
	needle := strings.ToLower(query)
	out := []workItemSummary{}
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.ReadableID), needle) || strings.Contains(strings.ToLower(item.Name), needle) || strings.Contains(strings.ToLower(item.DescriptionHTML), needle) {
			out = append(out, item)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}
