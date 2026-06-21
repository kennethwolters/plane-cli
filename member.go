package main

import (
	"context"
	"fmt"
)

type memberListData struct {
	WorkspaceSlug string          `json:"workspace_slug"`
	Project       projectSummary  `json:"project"`
	Members       []memberSummary `json:"members"`
	Count         int             `json:"count"`
}

func (a app) cmdMember(ctx context.Context, args []string, configCtx configContext) int {
	if len(args) == 0 {
		return a.usageError("member requires a subcommand", "text")
	}
	format, rest, err := parseFormat(args[1:])
	if err != nil {
		return a.writeCLIError(err, "json")
	}
	if args[0] != "list" {
		return a.usageError("unknown member subcommand: "+args[0], format)
	}
	projectRef, rest, flagErr := parseRequiredStringFlag(rest, "--project", "member list requires --project <project>")
	if flagErr != nil {
		return a.writeCLIError(flagErr, format)
	}
	if len(rest) != 0 {
		return a.usageError("member list takes no positional arguments", format)
	}
	return a.cmdMemberList(ctx, format, configCtx, projectRef)
}

func (a app) cmdMemberList(ctx context.Context, format string, configCtx configContext, projectRef string) int {
	eff, client, ok := a.configuredPlaneClient(format, configCtx)
	if !ok {
		return exitError
	}
	project, err := client.getProjectByRef(ctx, projectRef)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	members, err := client.listProjectMembers(ctx, project.ID)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	data := memberListData{WorkspaceSlug: eff.WorkspaceSlug.Value, Project: project, Members: members, Count: len(members)}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.member.list.v1", data))
		return exitOK
	}
	for _, member := range members {
		fmt.Fprintf(a.stdout, "%s\t%s\t%s\n", member.ID, member.Email, member.DisplayName)
	}
	return exitOK
}
