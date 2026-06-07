package main

import (
	"context"
	"fmt"
)

type authStatusData struct {
	Authenticated bool       `json:"authenticated"`
	BaseURL       string     `json:"base_url"`
	WorkspaceSlug string     `json:"workspace_slug"`
	User          meResponse `json:"user"`
}

func (a app) cmdAuth(ctx context.Context, args []string, loadedDotenv map[string]bool) int {
	if len(args) == 0 {
		return a.usageError("auth requires a subcommand", "text")
	}
	format, rest, err := parseFormat(args[1:])
	if err != nil {
		return a.writeCLIError(err, "json")
	}
	if args[0] != "status" {
		return a.usageError("unknown auth subcommand: "+args[0], format)
	}
	if len(rest) != 0 {
		return a.usageError("auth status takes no positional arguments", format)
	}
	return a.cmdAuthStatus(ctx, format, loadedDotenv)
}

func (a app) cmdAuthStatus(ctx context.Context, format string, loadedDotenv map[string]bool) int {
	eff, cfgErr := loadEffectiveConfig(loadedDotenv)
	if cfgErr != nil {
		return a.writeCLIError(cfgErr, format)
	}
	if err := validateRequiredConfig(eff); err != nil {
		return a.writeCLIError(err, format)
	}
	client := newPlaneClient(eff, a.client)
	me, err := client.getMe(ctx)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	data := authStatusData{Authenticated: true, BaseURL: eff.BaseURL.Value, WorkspaceSlug: eff.WorkspaceSlug.Value, User: me}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.auth.status.v1", data))
		return exitOK
	}
	fmt.Fprintf(a.stdout, "authenticated: true\nuser: %s\nworkspace_slug: %s\n", displayUser(me), eff.WorkspaceSlug.Value)
	return exitOK
}

func validateRequiredConfig(eff effectiveConfig) *cliError {
	if !eff.APIKey.Present {
		return newError("MISSING_API_KEY", "PLANE_API_KEY is not configured.", "Set PLANE_API_KEY in the environment or local .env file.", true, "PLANE_API_KEY=... plane-cli auth status --format json")
	}
	if !eff.WorkspaceSlug.Present {
		return newError("MISSING_WORKSPACE_SLUG", "PLANE_WORKSPACE_SLUG is not configured.", "Set PLANE_WORKSPACE_SLUG or run: plane-cli config set workspace_slug <slug>", true)
	}
	if !eff.BaseURL.Present {
		return newError("MISSING_BASE_URL", "PLANE_BASE_URL is not configured.", "Set PLANE_BASE_URL or run: plane-cli config set base_url <url>", true)
	}
	return nil
}

func displayUser(me meResponse) string {
	if me.DisplayName != "" {
		return me.DisplayName
	}
	if me.Email != "" {
		return me.Email
	}
	if me.ID != "" {
		return me.ID
	}
	return "<unknown>"
}
