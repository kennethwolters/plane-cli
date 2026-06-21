package main

import (
	"context"
	"fmt"
)

type projectListData struct {
	WorkspaceSlug string           `json:"workspace_slug"`
	Projects      []projectSummary `json:"projects"`
	Count         int              `json:"count"`
}

type projectGetData struct {
	WorkspaceSlug string         `json:"workspace_slug"`
	Project       projectSummary `json:"project"`
}

func (a app) cmdProject(ctx context.Context, args []string, configCtx configContext) int {
	if len(args) == 0 {
		return a.usageError("project requires a subcommand", "text")
	}
	sub := args[0]
	format, rest, err := parseFormat(args[1:])
	if err != nil {
		return a.writeCLIError(err, "json")
	}
	switch sub {
	case "list":
		if len(rest) != 0 {
			return a.usageError("project list takes no positional arguments", format)
		}
		return a.cmdProjectList(ctx, format, configCtx)
	case "get":
		if len(rest) != 1 {
			return a.usageError("project get requires exactly one project reference", format)
		}
		return a.cmdProjectGet(ctx, format, configCtx, rest[0])
	default:
		return a.usageError("unknown project subcommand: "+sub, format)
	}
}

func (a app) cmdProjectList(ctx context.Context, format string, configCtx configContext) int {
	eff, client, ok := a.configuredPlaneClient(format, configCtx)
	if !ok {
		return exitError
	}
	projects, err := client.listProjects(ctx)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	data := projectListData{WorkspaceSlug: eff.WorkspaceSlug.Value, Projects: projects, Count: len(projects)}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.project.list.v1", data))
		return exitOK
	}
	for _, project := range projects {
		fmt.Fprintf(a.stdout, "%s\t%s\t%s\n", project.Identifier, project.ID, project.Name)
	}
	return exitOK
}

func (a app) cmdProjectGet(ctx context.Context, format string, configCtx configContext, ref string) int {
	eff, client, ok := a.configuredPlaneClient(format, configCtx)
	if !ok {
		return exitError
	}
	project, err := client.getProjectByRef(ctx, ref)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	data := projectGetData{WorkspaceSlug: eff.WorkspaceSlug.Value, Project: project}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.project.get.v1", data))
		return exitOK
	}
	fmt.Fprintf(a.stdout, "%s\t%s\t%s\n", project.Identifier, project.ID, project.Name)
	return exitOK
}

func (a app) configuredPlaneClient(format string, configCtx configContext) (effectiveConfig, planeClient, bool) {
	if envErr := configCtx.blockingEnvFileError(); envErr != nil {
		a.writeCLIError(envErr, format)
		return effectiveConfig{}, planeClient{}, false
	}
	eff, cfgErr := loadEffectiveConfig(configCtx)
	if cfgErr != nil {
		a.writeCLIError(cfgErr, format)
		return effectiveConfig{}, planeClient{}, false
	}
	if err := validateRequiredConfig(eff); err != nil {
		a.writeCLIError(err, format)
		return effectiveConfig{}, planeClient{}, false
	}
	return eff, newPlaneClient(eff, a.client), true
}
