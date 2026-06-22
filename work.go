package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type workListData struct {
	WorkspaceSlug string            `json:"workspace_slug"`
	Project       projectSummary    `json:"project"`
	WorkItems     []workItemSummary `json:"work_items"`
	Count         int               `json:"count"`
}

type workGetData struct {
	WorkspaceSlug string          `json:"workspace_slug"`
	WorkItem      workItemSummary `json:"work_item"`
}

type workMutationData struct {
	Operation mutationOperation `json:"operation"`
	Applied   bool              `json:"applied"`
	Verified  bool              `json:"verified"`
	WorkItem  *workItemSummary  `json:"work_item,omitempty"`
	Comment   *commentSummary   `json:"comment,omitempty"`
}

type mutationOperation struct {
	Kind       string         `json:"kind"`
	Target     string         `json:"target,omitempty"`
	ProjectID  string         `json:"project_id,omitempty"`
	WorkItemID string         `json:"work_item_id,omitempty"`
	Changes    map[string]any `json:"changes"`
	Reason     string         `json:"reason,omitempty"`
}

type mutationFlags struct {
	DryRun bool
	Apply  bool
	Verify bool
}

func (a app) cmdWork(ctx context.Context, args []string, configCtx configContext) int {
	if len(args) == 0 {
		return a.usageError("work requires a subcommand", "text")
	}
	sub := args[0]
	format, rest, err := parseFormat(args[1:])
	if err != nil {
		return a.writeCLIError(err, "json")
	}
	switch sub {
	case "list":
		return a.cmdWorkList(ctx, format, configCtx, rest)
	case "get":
		if len(rest) != 1 {
			return a.usageError("work get requires exactly one work item reference", format)
		}
		return a.cmdWorkGet(ctx, format, configCtx, rest[0])
	case "create":
		return a.cmdWorkCreate(ctx, format, configCtx, rest)
	case "edit":
		return a.cmdWorkEdit(ctx, format, configCtx, rest)
	case "comment":
		return a.cmdWorkComment(ctx, format, configCtx, rest)
	case "start", "complete", "reopen", "cancel":
		return a.cmdWorkLifecycle(ctx, format, configCtx, sub, rest)
	default:
		return a.usageError("unknown work subcommand: "+sub, format)
	}
}

func (a app) cmdWorkList(ctx context.Context, format string, configCtx configContext, args []string) int {
	projectRef, args, flagErr := parseRequiredStringFlag(args, "--project", "work list requires --project <project>")
	if flagErr != nil {
		return a.writeCLIError(flagErr, format)
	}
	stateGroup, args, _ := parseStringFlag(args, "--state-group")
	limitText, args, _ := parseStringFlag(args, "--limit")
	if len(args) != 0 {
		return a.usageError("work list takes no positional arguments", format)
	}
	limit := 50
	if limitText != "" {
		parsed, err := strconv.Atoi(limitText)
		if err != nil || parsed < 1 {
			return a.writeCLIError(newError("VALIDATION_FAILED", "--limit must be a positive integer.", "Use a positive integer such as --limit 20.", false), format)
		}
		limit = parsed
	}
	eff, client, ok := a.configuredPlaneClient(format, configCtx)
	if !ok {
		return exitError
	}
	project, err := client.getProjectByRef(ctx, projectRef)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	items, err := client.listWorkItems(ctx, project, stateGroup, limit)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	data := workListData{WorkspaceSlug: eff.WorkspaceSlug.Value, Project: project, WorkItems: items, Count: len(items)}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.work.list.v1", data))
		return exitOK
	}
	for _, item := range items {
		fmt.Fprintf(a.stdout, "%s\t%s\t%s\t%s\n", item.ReadableID, item.StateGroup, item.WorkItemID, item.Name)
	}
	return exitOK
}

func (a app) cmdWorkGet(ctx context.Context, format string, configCtx configContext, ref string) int {
	eff, client, ok := a.configuredPlaneClient(format, configCtx)
	if !ok {
		return exitError
	}
	item, err := client.getWorkItemByRef(ctx, ref)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	data := workGetData{WorkspaceSlug: eff.WorkspaceSlug.Value, WorkItem: item}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.work.get.v1", data))
		return exitOK
	}
	fmt.Fprintf(a.stdout, "%s\t%s\t%s\n", item.ReadableID, item.StateGroup, item.Name)
	return exitOK
}

func (a app) cmdWorkCreate(ctx context.Context, format string, configCtx configContext, args []string) int {
	flags, args := parseMutationFlags(args)
	projectRef, args, flagErr := parseRequiredStringFlag(args, "--project", "work create requires --project <project>")
	if flagErr != nil {
		return a.writeCLIError(flagErr, format)
	}
	title, args, ok := parseStringFlag(args, "--title")
	if !ok || title == "" {
		return a.writeCLIError(newError("VALIDATION_FAILED", "work create requires --title <title>.", "Pass a concise work item title.", false), format)
	}
	descriptionHTML, args, _ := parseStringFlag(args, "--description-html")
	priority, args, _ := parseStringFlag(args, "--priority")
	if len(args) != 0 {
		return a.usageError("work create takes flags only", format)
	}
	eff, client, configured := a.configuredPlaneClient(format, configCtx)
	if !configured {
		return exitError
	}
	project, err := client.getProjectByRef(ctx, projectRef)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	changes := map[string]any{"name": title}
	if descriptionHTML != "" {
		changes["description_html"] = descriptionHTML
	}
	if priority != "" {
		changes["priority"] = priority
	}
	op := mutationOperation{Kind: "work.create", ProjectID: project.ID, Changes: changes}
	data := workMutationData{Operation: op, Applied: false, Verified: false}
	if flags.Apply {
		item, err := client.createWorkItem(ctx, project, changes)
		if err != nil {
			return a.writeCLIError(err, format)
		}
		data.Applied = true
		data.WorkItem = &item
		data.Operation.Target = item.ReadableID
		data.Operation.WorkItemID = item.WorkItemID
		if flags.Verify {
			verified, err := client.verifyWorkItemExists(ctx, item)
			if err != nil {
				return a.writeCLIError(err, format)
			}
			data.Verified = verified
		}
	}
	return a.writeWorkMutation(format, "plane.work.create.v1", eff.WorkspaceSlug.Value, data)
}

func (a app) cmdWorkEdit(ctx context.Context, format string, configCtx configContext, args []string) int {
	flags, args := parseMutationFlags(args)
	if len(args) == 0 {
		return a.usageError("work edit requires a work item reference", format)
	}
	ref := args[0]
	args = args[1:]
	title, args, _ := parseStringFlag(args, "--title")
	descriptionHTML, args, _ := parseStringFlag(args, "--description-html")
	priority, args, _ := parseStringFlag(args, "--priority")
	if len(args) != 0 {
		return a.usageError("work edit takes one reference plus flags", format)
	}
	changes := map[string]any{}
	if title != "" {
		changes["name"] = title
	}
	if descriptionHTML != "" {
		changes["description_html"] = descriptionHTML
	}
	if priority != "" {
		changes["priority"] = priority
	}
	if len(changes) == 0 {
		return a.writeCLIError(newError("VALIDATION_FAILED", "work edit requires at least one changed field.", "Pass --title, --description-html, or --priority.", false), format)
	}
	eff, client, ok := a.configuredPlaneClient(format, configCtx)
	if !ok {
		return exitError
	}
	item, err := client.getWorkItemByRef(ctx, ref)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	data := workMutationData{Operation: mutationOperation{Kind: "work.edit", Target: item.ReadableID, ProjectID: item.ProjectID, WorkItemID: item.WorkItemID, Changes: changes}}
	if flags.Apply {
		updated, err := client.updateWorkItem(ctx, item.ProjectID, item.WorkItemID, changes)
		if err != nil {
			return a.writeCLIError(err, format)
		}
		updated = preserveWorkItemIdentity(updated, item)
		data.Applied = true
		data.WorkItem = &updated
		if flags.Verify {
			verified, err := client.verifyWorkItemChanges(ctx, updated, changes)
			if err != nil {
				return a.writeCLIError(err, format)
			}
			data.Verified = verified
		}
	}
	return a.writeWorkMutation(format, "plane.work.edit.v1", eff.WorkspaceSlug.Value, data)
}

func (a app) cmdWorkComment(ctx context.Context, format string, configCtx configContext, args []string) int {
	flags, args := parseMutationFlags(args)
	if len(args) == 0 {
		return a.usageError("work comment requires a work item reference", format)
	}
	ref := args[0]
	args = args[1:]
	html, args, ok := parseStringFlag(args, "--html")
	if !ok || html == "" {
		return a.writeCLIError(newError("VALIDATION_FAILED", "work comment requires --html <html>.", "Plane comments are HTML; pass safe HTML markup.", false), format)
	}
	if len(args) != 0 {
		return a.usageError("work comment takes one reference plus flags", format)
	}
	eff, client, configured := a.configuredPlaneClient(format, configCtx)
	if !configured {
		return exitError
	}
	item, err := client.getWorkItemByRef(ctx, ref)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	changes := map[string]any{"comment_html": html}
	data := workMutationData{Operation: mutationOperation{Kind: "work.comment", Target: item.ReadableID, ProjectID: item.ProjectID, WorkItemID: item.WorkItemID, Changes: changes}}
	if flags.Apply {
		comment, err := client.createWorkItemComment(ctx, item.ProjectID, item.WorkItemID, html)
		if err != nil {
			return a.writeCLIError(err, format)
		}
		data.Applied = true
		data.Comment = &comment
		if flags.Verify {
			data.Verified = comment.ID != ""
		}
	}
	return a.writeWorkMutation(format, "plane.work.comment.v1", eff.WorkspaceSlug.Value, data)
}

func (a app) cmdWorkLifecycle(ctx context.Context, format string, configCtx configContext, action string, args []string) int {
	flags, args := parseMutationFlags(args)
	if len(args) == 0 {
		return a.usageError("work "+action+" requires a work item reference", format)
	}
	ref := args[0]
	args = args[1:]
	reason, args, _ := parseStringFlag(args, "--reason")
	evidence, args, _ := parseStringFlag(args, "--evidence")
	pr, args, _ := parseStringFlag(args, "--pr")
	if action == "complete" && evidence == "" {
		return a.writeCLIError(newError("VALIDATION_FAILED", "work complete requires --evidence <text>.", "Provide evidence that the work is done.", false), format)
	}
	if (action == "reopen" || action == "cancel") && reason == "" {
		return a.writeCLIError(newError("VALIDATION_FAILED", "work "+action+" requires --reason <text>.", "Explain why this state transition is being made.", false), format)
	}
	if len(args) != 0 {
		return a.usageError("work "+action+" takes one reference plus flags", format)
	}
	eff, client, configured := a.configuredPlaneClient(format, configCtx)
	if !configured {
		return exitError
	}
	item, err := client.getWorkItemByRef(ctx, ref)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	targetGroup := lifecycleTargetGroup(action)
	state, err := client.firstStateForGroup(ctx, item.ProjectID, targetGroup)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	changes := map[string]any{"state": state.ID, "state_group": targetGroup}
	if evidence != "" {
		changes["evidence"] = evidence
	}
	if pr != "" {
		changes["pr"] = pr
	}
	if reason != "" {
		changes["reason"] = reason
	}
	data := workMutationData{Operation: mutationOperation{Kind: "work." + action, Target: item.ReadableID, ProjectID: item.ProjectID, WorkItemID: item.WorkItemID, Changes: changes, Reason: reason}}
	if flags.Apply {
		updated, err := client.updateWorkItem(ctx, item.ProjectID, item.WorkItemID, map[string]any{"state": state.ID})
		if err != nil {
			return a.writeCLIError(err, format)
		}
		updated = preserveWorkItemIdentity(updated, item)
		if evidence != "" || reason != "" || pr != "" {
			html := lifecycleCommentHTML(action, evidence, reason, pr)
			if html != "" {
				_, err := client.createWorkItemComment(ctx, item.ProjectID, item.WorkItemID, html)
				if err != nil {
					return a.writeCLIError(err, format)
				}
			}
		}
		updated.StateGroup = targetGroup
		data.Applied = true
		data.WorkItem = &updated
		if flags.Verify {
			verified, err := client.verifyWorkItemStateGroup(ctx, updated, targetGroup)
			if err != nil {
				return a.writeCLIError(err, format)
			}
			data.Verified = verified
			if !verified {
				return a.writeCLIError(newError("VERIFY_FAILED", "Work item state did not verify as "+targetGroup+".", "Inspect the work item and retry if safe.", true), format)
			}
		}
	}
	return a.writeWorkMutation(format, "plane.work."+action+".v1", eff.WorkspaceSlug.Value, data)
}

func preserveWorkItemIdentity(updated, original workItemSummary) workItemSummary {
	if updated.ProjectIdentifier == "" {
		updated.ProjectIdentifier = original.ProjectIdentifier
	}
	if updated.ReadableID == "" {
		updated.ReadableID = original.ReadableID
	}
	if updated.ProjectID == "" {
		updated.ProjectID = original.ProjectID
	}
	if updated.WorkItemID == "" {
		updated.WorkItemID = original.WorkItemID
	}
	if updated.SequenceID == "" {
		updated.SequenceID = original.SequenceID
	}
	return updated
}

func parseMutationFlags(args []string) (mutationFlags, []string) {
	args, dryRun := hasFlag(args, "--dry-run")
	args, apply := hasFlag(args, "--apply")
	args, verify := hasFlag(args, "--verify")
	if !apply {
		dryRun = true
	}
	return mutationFlags{DryRun: dryRun, Apply: apply, Verify: verify}, args
}

func lifecycleTargetGroup(action string) string {
	switch action {
	case "start", "reopen":
		return "started"
	case "complete":
		return "completed"
	case "cancel":
		return "cancelled"
	default:
		return "started"
	}
}

func lifecycleCommentHTML(action, evidence, reason, pr string) string {
	parts := []string{}
	if evidence != "" {
		parts = append(parts, "<p><strong>Evidence:</strong> "+escapeHTML(evidence)+"</p>")
	}
	if reason != "" {
		parts = append(parts, "<p><strong>Reason:</strong> "+escapeHTML(reason)+"</p>")
	}
	if pr != "" {
		parts = append(parts, "<p><strong>PR:</strong> "+escapeHTML(pr)+"</p>")
	}
	if len(parts) == 0 {
		return ""
	}
	return "<p><strong>plane-cli work " + escapeHTML(action) + "</strong></p>" + strings.Join(parts, "")
}

func escapeHTML(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return replacer.Replace(s)
}

func (a app) writeWorkMutation(format, schema, workspaceSlug string, data workMutationData) int {
	if format == "json" {
		writeJSON(a.stdout, okEnvelope(schema, map[string]any{"workspace_slug": workspaceSlug, "operation": data.Operation, "applied": data.Applied, "verified": data.Verified, "work_item": data.WorkItem, "comment": data.Comment}))
		return exitOK
	}
	mode := "dry-run"
	if data.Applied {
		mode = "applied"
	}
	fmt.Fprintf(a.stdout, "%s %s\n", data.Operation.Kind, mode)
	return exitOK
}
