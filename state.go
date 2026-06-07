package main

import (
	"context"
	"fmt"
)

type stateListData struct {
	WorkspaceSlug string         `json:"workspace_slug"`
	Project       projectSummary `json:"project"`
	States        []stateSummary `json:"states"`
	Count         int            `json:"count"`
}

func (a app) cmdState(ctx context.Context, args []string, loadedDotenv map[string]bool) int {
	if len(args) == 0 {
		return a.usageError("state requires a subcommand", "text")
	}
	format, rest, err := parseFormat(args[1:])
	if err != nil {
		return a.writeCLIError(err, "json")
	}
	if args[0] != "list" {
		return a.usageError("unknown state subcommand: "+args[0], format)
	}
	projectRef, rest, flagErr := parseRequiredStringFlag(rest, "--project", "state list requires --project <project>")
	if flagErr != nil {
		return a.writeCLIError(flagErr, format)
	}
	if len(rest) != 0 {
		return a.usageError("state list takes no positional arguments", format)
	}
	return a.cmdStateList(ctx, format, loadedDotenv, projectRef)
}

func (a app) cmdStateList(ctx context.Context, format string, loadedDotenv map[string]bool, projectRef string) int {
	eff, client, ok := a.configuredPlaneClient(format, loadedDotenv)
	if !ok {
		return exitError
	}
	project, err := client.getProjectByRef(ctx, projectRef)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	states, err := client.listProjectStates(ctx, project.ID)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	data := stateListData{WorkspaceSlug: eff.WorkspaceSlug.Value, Project: project, States: states, Count: len(states)}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.state.list.v1", data))
		return exitOK
	}
	for _, state := range states {
		fmt.Fprintf(a.stdout, "%s\t%s\t%s\t%s\n", state.Group, state.Name, state.ID, state.Color)
	}
	return exitOK
}
