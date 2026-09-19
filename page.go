package main

import (
	"context"
	"fmt"
)

type pageListData struct {
	WorkspaceSlug string         `json:"workspace_slug"`
	Project       projectSummary `json:"project"`
	Pages         []pageSummary  `json:"pages"`
	Count         int            `json:"count"`
}

type pageGetData struct {
	WorkspaceSlug string         `json:"workspace_slug"`
	Project       projectSummary `json:"project"`
	Page          pageDetail     `json:"page"`
}

func (a app) cmdPage(ctx context.Context, args []string, configCtx configContext) int {
	if len(args) == 0 {
		return a.usageError("page requires a subcommand", "text")
	}
	sub := args[0]
	format, rest, err := parseFormat(args[1:])
	if err != nil {
		return a.writeCLIError(err, "json")
	}
	projectRef, rest, flagErr := parseRequiredStringFlag(rest, "--project", "page "+sub+" requires --project <project>")
	if flagErr != nil {
		return a.writeCLIError(flagErr, format)
	}
	switch sub {
	case "list":
		if len(rest) != 0 {
			return a.usageError("page list takes no positional arguments", format)
		}
		return a.cmdPageList(ctx, format, configCtx, projectRef)
	case "get":
		if len(rest) != 1 {
			return a.usageError("page get requires exactly one page UUID or exact name", format)
		}
		return a.cmdPageGet(ctx, format, configCtx, projectRef, rest[0])
	default:
		return a.usageError("unknown page subcommand: "+sub, format)
	}
}

func (a app) cmdPageList(ctx context.Context, format string, configCtx configContext, projectRef string) int {
	eff, client, ok := a.configuredPlaneClient(format, configCtx)
	if !ok {
		return exitError
	}
	project, err := client.getProjectByRef(ctx, projectRef)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	pages, err := client.listProjectPages(ctx, project.ID)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	data := pageListData{WorkspaceSlug: eff.WorkspaceSlug.Value, Project: project, Pages: pages, Count: len(pages)}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.page.list.v1", data))
		return exitOK
	}
	for _, page := range pages {
		name, _ := boundOutputText(page.Name, maxOutputMetadataRunes)
		fmt.Fprintf(a.stdout, "%s\t%s\n", page.ID, name)
	}
	return exitOK
}

func (a app) cmdPageGet(ctx context.Context, format string, configCtx configContext, projectRef, pageRef string) int {
	eff, client, ok := a.configuredPlaneClient(format, configCtx)
	if !ok {
		return exitError
	}
	project, err := client.getProjectByRef(ctx, projectRef)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	page, err := client.getProjectPageByRef(ctx, project.ID, pageRef)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	data := pageGetData{WorkspaceSlug: eff.WorkspaceSlug.Value, Project: project, Page: page}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.page.get.v1", data))
		return exitOK
	}
	name, _ := boundOutputText(page.Name, maxOutputMetadataRunes)
	description, _ := boundOutputText(page.DescriptionText, maxOutputDetailRunes)
	fmt.Fprintf(a.stdout, "%s\n\n%s\n", name, description)
	return exitOK
}
