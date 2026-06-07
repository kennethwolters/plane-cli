package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var readableRefPattern = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]{0,11})-([0-9]+)$`)

func (a app) cmdResolve(ctx context.Context, args []string, loadedDotenv map[string]bool) int {
	args, _ = hasFlag(args, "--no-cache")
	format, rest, err := parseFormat(args)
	if err != nil {
		return a.writeCLIError(err, "json")
	}
	if len(rest) != 1 {
		return a.usageError("resolve requires exactly one reference", format)
	}
	projectIdentifier, number, parseErr := parseReadableRef(rest[0])
	if parseErr != nil {
		return a.writeCLIError(parseErr, format)
	}
	eff, cfgErr := loadEffectiveConfig(loadedDotenv)
	if cfgErr != nil {
		return a.writeCLIError(cfgErr, format)
	}
	if reqErr := validateRequiredConfig(eff); reqErr != nil {
		return a.writeCLIError(reqErr, format)
	}
	client := newPlaneClient(eff, a.client)
	item, resolveErr := client.resolveWorkItem(ctx, projectIdentifier, number)
	if resolveErr != nil {
		return a.writeCLIError(resolveErr, format)
	}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.resolve.v1", item))
		return exitOK
	}
	fmt.Fprintf(a.stdout, "%s\nwork_item_id: %s\nproject_id: %s\n", item.ReadableID, item.WorkItemID, item.ProjectID)
	return exitOK
}

func parseReadableRef(ref string) (string, string, *cliError) {
	matches := readableRefPattern.FindStringSubmatch(strings.TrimSpace(ref))
	if matches == nil {
		return "", "", newError("INVALID_REFERENCE", "Invalid work item reference: "+ref, "Use a readable Plane work item ID like ENG-42.", false, "plane-cli resolve ENG-42 --format json")
	}
	return strings.ToUpper(matches[1]), matches[2], nil
}
