package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

type workListData struct {
	WorkspaceSlug string         `json:"workspace_slug"`
	Project       projectSummary `json:"project"`
	WorkItems     any            `json:"work_items"`
	Count         int            `json:"count"`
}

type workGetData struct {
	WorkspaceSlug string          `json:"workspace_slug"`
	WorkItem      workItemSummary `json:"work_item"`
}

type workCommentsData struct {
	WorkspaceSlug string           `json:"workspace_slug"`
	WorkItem      workItemSummary  `json:"work_item"`
	Comments      []commentSummary `json:"comments"`
	Count         int              `json:"count"`
}

type workMutationData struct {
	Operation           mutationOperation `json:"operation"`
	Applied             bool              `json:"applied"`
	Verified            bool              `json:"verified"`
	WorkItem            *workItemSummary  `json:"work_item,omitempty"`
	Comment             *commentSummary   `json:"comment,omitempty"`
	DuplicateCandidates []searchResult    `json:"duplicate_candidates,omitempty"`
}

type mutationOperation struct {
	Kind       string         `json:"kind"`
	Target     string         `json:"target,omitempty"`
	ProjectID  string         `json:"project_id,omitempty"`
	WorkItemID string         `json:"work_item_id,omitempty"`
	Changes    map[string]any `json:"changes"`
	Before     map[string]any `json:"before,omitempty"`
	After      map[string]any `json:"after,omitempty"`
	Reason     string         `json:"reason,omitempty"`
}

type mutationFlags struct {
	DryRun bool
	Apply  bool
	Verify bool
}

type workBatchData struct {
	WorkspaceSlug string            `json:"workspace_slug"`
	Kind          string            `json:"kind"`
	Planned       int               `json:"planned"`
	Applied       int               `json:"applied"`
	Skipped       int               `json:"skipped"`
	Failed        int               `json:"failed"`
	Retryable     int               `json:"retryable_failures"`
	Results       []workBatchResult `json:"results"`
}

type workBatchResult struct {
	Key       string    `json:"key"`
	Applied   bool      `json:"applied"`
	Skipped   bool      `json:"skipped"`
	Error     *cliError `json:"error,omitempty"`
	Verified  bool      `json:"verified"`
	CommentID string    `json:"comment_id,omitempty"`
}

type workRelationData struct {
	WorkspaceSlug string            `json:"workspace_slug"`
	Operation     mutationOperation `json:"operation"`
	Applied       bool              `json:"applied"`
	Verified      bool              `json:"verified"`
	Relation      *relationSummary  `json:"relation,omitempty"`
	WorkItem      *workItemSummary  `json:"work_item,omitempty"`
	Children      []workItemSummary `json:"children,omitempty"`
	Count         int               `json:"count,omitempty"`
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
	case "comments":
		return a.cmdWorkComments(ctx, format, configCtx, rest)
	case "batch":
		return a.cmdWorkBatch(ctx, format, configCtx, rest)
	case "move":
		return a.cmdWorkMove(ctx, format, configCtx, rest)
	case "children":
		return a.cmdWorkChildren(ctx, format, configCtx, rest)
	case "parent":
		return a.cmdWorkParent(ctx, format, configCtx, rest)
	case "relation":
		return a.cmdWorkRelation(ctx, format, configCtx, rest)
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
	fieldsText, args, _ := parseStringFlag(args, "--fields")
	excerptText, args, _ := parseStringFlag(args, "--description-excerpt")
	latestCommentsText, args, _ := parseStringFlag(args, "--include-comments-latest")
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
	excerptLimit := 0
	if excerptText != "" {
		parsed, err := strconv.Atoi(excerptText)
		if err != nil || parsed < 1 {
			return a.writeCLIError(newError("VALIDATION_FAILED", "--description-excerpt must be a positive integer.", "Use a positive integer such as --description-excerpt 300.", false), format)
		}
		excerptLimit = parsed
	}
	latestComments := 0
	if latestCommentsText != "" {
		parsed, err := strconv.Atoi(latestCommentsText)
		if err != nil || parsed < 1 {
			return a.writeCLIError(newError("VALIDATION_FAILED", "--include-comments-latest must be a positive integer.", "Use a positive integer such as --include-comments-latest 2.", false), format)
		}
		latestComments = parsed
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
	var outputItems any = items
	if fieldsText != "" || excerptLimit > 0 || latestComments > 0 {
		projected, err := client.projectWorkItemsForList(ctx, items, parseCSV(fieldsText), excerptLimit, latestComments)
		if err != nil {
			return a.writeCLIError(err, format)
		}
		outputItems = projected
	}
	data := workListData{WorkspaceSlug: eff.WorkspaceSlug.Value, Project: project, WorkItems: outputItems, Count: len(items)}
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

func (a app) cmdWorkComments(ctx context.Context, format string, configCtx configContext, args []string) int {
	if len(args) == 0 {
		return a.usageError("work comments requires a work item reference", format)
	}
	ref := args[0]
	args = args[1:]
	limitText, args, _ := parseStringFlag(args, "--limit")
	if len(args) != 0 {
		return a.usageError("work comments takes one reference plus flags", format)
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
	item, err := client.getWorkItemByRef(ctx, ref)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	comments, err := client.listWorkItemComments(ctx, item.ProjectID, item.WorkItemID, limit)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	data := workCommentsData{WorkspaceSlug: eff.WorkspaceSlug.Value, WorkItem: item, Comments: comments, Count: len(comments)}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.work.comments.v1", data))
		return exitOK
	}
	for _, comment := range comments {
		fmt.Fprintf(a.stdout, "%s\t%s\n", comment.ID, comment.CommentHTML)
	}
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
	descriptionHTML, args, descriptionInline := parseStringFlag(args, "--description-html")
	descriptionFile, args, descriptionFileFlag := parseStringFlag(args, "--description-file")
	descriptionMDFile, args, descriptionMDFileFlag := parseStringFlag(args, "--description-md-file")
	priority, args, _ := parseStringFlag(args, "--priority")
	dedupeQuery, args, _ := parseStringFlag(args, "--dedupe-query")
	if len(args) != 0 {
		return a.usageError("work create takes flags only", format)
	}
	body, bodyErr := readBodyInput(descriptionHTML, descriptionInline, "--description-html", descriptionFile, descriptionFileFlag, "--description-file")
	if bodyErr != nil {
		return a.writeCLIError(bodyErr, format)
	}
	if descriptionMDFileFlag {
		if descriptionInline || descriptionFileFlag {
			return a.writeCLIError(newError("VALIDATION_FAILED", "--description-md-file cannot be combined with HTML description flags.", "Pass only one description input.", false), format)
		}
		md, err := readBodyInput("", false, "", descriptionMDFile, true, "--description-md-file")
		if err != nil {
			return a.writeCLIError(err, format)
		}
		body = markdownToHTML(md)
	}
	descriptionHTML = body
	eff, client, configured := a.configuredPlaneClient(format, configCtx)
	if !configured {
		return exitError
	}
	project, err := client.getProjectByRef(ctx, projectRef)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	duplicateCandidates := []searchResult{}
	if dedupeQuery != "" {
		projectItems, err := client.listWorkItems(ctx, project, "", 50)
		if err != nil {
			return a.writeCLIError(err, format)
		}
		duplicateCandidates = searchWorkItems(projectItems, dedupeQuery, 10)
		if flags.Apply && len(duplicateCandidates) > 0 {
			return a.writeCLIError(newError("DUPLICATE_CANDIDATES_FOUND", "Strong duplicate candidates were found for this work item.", "Review duplicate_candidates with --dry-run before applying.", false), format)
		}
	}
	changes := map[string]any{"name": title}
	if descriptionHTML != "" {
		changes["description_html"] = descriptionHTML
	}
	if priority != "" {
		changes["priority"] = priority
	}
	op := mutationOperation{Kind: "work.create", ProjectID: project.ID, Changes: changes}
	data := workMutationData{Operation: op, Applied: false, Verified: false, DuplicateCandidates: duplicateCandidates}
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
	descriptionHTML, args, descriptionInline := parseStringFlag(args, "--description-html")
	descriptionFile, args, descriptionFileFlag := parseStringFlag(args, "--description-file")
	appendDescriptionHTML, args, appendDescriptionSet := parseStringFlag(args, "--append-description-html")
	priority, args, _ := parseStringFlag(args, "--priority")
	labelsAddText, args, _ := parseStringFlag(args, "--labels-add")
	labelsRemoveText, args, _ := parseStringFlag(args, "--labels-remove")
	assigneesAddText, args, _ := parseStringFlag(args, "--assignees-add")
	assigneesRemoveText, args, _ := parseStringFlag(args, "--assignees-remove")
	if len(args) != 0 {
		return a.usageError("work edit takes one reference plus flags", format)
	}
	if appendDescriptionSet && (descriptionInline || descriptionFileFlag) {
		return a.writeCLIError(newError("VALIDATION_FAILED", "--append-description-html cannot be combined with replacement description flags.", "Choose append or replacement description editing.", false), format)
	}
	body, bodyErr := readBodyInput(descriptionHTML, descriptionInline, "--description-html", descriptionFile, descriptionFileFlag, "--description-file")
	if bodyErr != nil {
		return a.writeCLIError(bodyErr, format)
	}
	descriptionHTML = body
	eff, client, ok := a.configuredPlaneClient(format, configCtx)
	if !ok {
		return exitError
	}
	item, err := client.getWorkItemByRef(ctx, ref)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	changes := map[string]any{}
	if title != "" {
		changes["name"] = title
	}
	if appendDescriptionSet {
		descriptionHTML = item.DescriptionHTML + appendDescriptionHTML
	}
	if descriptionHTML != "" {
		changes["description_html"] = descriptionHTML
	}
	if priority != "" {
		changes["priority"] = priority
	}
	if labelsAddText != "" || labelsRemoveText != "" {
		labels, err := client.resolveLabelSet(ctx, item.ProjectID, item.LabelIDs, parseCSV(labelsAddText), parseCSV(labelsRemoveText))
		if err != nil {
			return a.writeCLIError(err, format)
		}
		changes["labels"] = labels
	}
	if assigneesAddText != "" || assigneesRemoveText != "" {
		assignees, err := client.resolveAssigneeSet(ctx, item.ProjectID, item.AssigneeIDs, parseCSV(assigneesAddText), parseCSV(assigneesRemoveText))
		if err != nil {
			return a.writeCLIError(err, format)
		}
		changes["assignees"] = assignees
	}
	if len(changes) == 0 {
		return a.writeCLIError(newError("VALIDATION_FAILED", "work edit requires at least one changed field.", "Pass --title, --description-html, --append-description-html, --priority, labels, or assignees.", false), format)
	}
	op := mutationOperation{Kind: "work.edit", Target: item.ReadableID, ProjectID: item.ProjectID, WorkItemID: item.WorkItemID, Changes: changes}
	op.Before = mutationBefore(item, changes)
	op.After = mutationAfter(item, changes)
	data := workMutationData{Operation: op}
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
	html, args, inline := parseStringFlag(args, "--html")
	htmlFile, args, fileFlag := parseStringFlag(args, "--html-file")
	mdFile, args, mdFileFlag := parseStringFlag(args, "--md-file")
	if len(args) != 0 {
		return a.usageError("work comment takes one reference plus flags", format)
	}
	if mdFileFlag && (inline || fileFlag) {
		return a.writeCLIError(newError("VALIDATION_FAILED", "--md-file cannot be combined with HTML comment flags.", "Pass only one comment input.", false), format)
	}
	body, bodyErr := readBodyInput(html, inline, "--html", htmlFile, fileFlag, "--html-file")
	if bodyErr != nil {
		return a.writeCLIError(bodyErr, format)
	}
	if mdFileFlag {
		md, err := readBodyInput("", false, "", mdFile, true, "--md-file")
		if err != nil {
			return a.writeCLIError(err, format)
		}
		body = markdownToHTML(md)
	}
	html = body
	if html == "" {
		return a.writeCLIError(newError("VALIDATION_FAILED", "work comment requires --html <html> or --html-file <path>.", "Plane comments are HTML; pass safe HTML markup.", false), format)
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

func (a app) cmdWorkMove(ctx context.Context, format string, configCtx configContext, args []string) int {
	flags, args := parseMutationFlags(args)
	if len(args) == 0 {
		return a.usageError("work move requires a work item reference", format)
	}
	ref := args[0]
	args = args[1:]
	stateRef, args, ok := parseStringFlag(args, "--state")
	if !ok || stateRef == "" {
		return a.writeCLIError(newError("VALIDATION_FAILED", "work move requires --state <state>.", "Pass a state name, group, or UUID.", false), format)
	}
	if len(args) != 0 {
		return a.usageError("work move takes one reference plus flags", format)
	}
	eff, client, configured := a.configuredPlaneClient(format, configCtx)
	if !configured {
		return exitError
	}
	item, err := client.getWorkItemByRef(ctx, ref)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	state, err := client.resolveState(ctx, item.ProjectID, stateRef)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	changes := map[string]any{"state": state.ID, "state_group": state.Group}
	op := mutationOperation{Kind: "work.move", Target: item.ReadableID, ProjectID: item.ProjectID, WorkItemID: item.WorkItemID, Changes: changes}
	op.Before = map[string]any{"state": item.StateID, "state_group": item.StateGroup}
	op.After = map[string]any{"state": state.ID, "state_group": state.Group}
	data := workMutationData{Operation: op}
	if flags.Apply {
		updated, err := client.updateWorkItem(ctx, item.ProjectID, item.WorkItemID, map[string]any{"state": state.ID})
		if err != nil {
			return a.writeCLIError(err, format)
		}
		updated = preserveWorkItemIdentity(updated, item)
		updated.StateGroup = state.Group
		data.Applied = true
		data.WorkItem = &updated
		if flags.Verify {
			verified, err := client.verifyWorkItemStateGroup(ctx, updated, state.Group)
			if err != nil {
				return a.writeCLIError(err, format)
			}
			data.Verified = verified
		}
	}
	return a.writeWorkMutation(format, "plane.work.move.v1", eff.WorkspaceSlug.Value, data)
}

type batchEditInput struct {
	Key             string   `json:"key"`
	Title           string   `json:"title"`
	Priority        string   `json:"priority"`
	DescriptionHTML string   `json:"description_html"`
	DescriptionFile string   `json:"description_file"`
	LabelsAdd       []string `json:"labels_add"`
	LabelsRemove    []string `json:"labels_remove"`
}

type batchCommentInput struct {
	Key      string `json:"key"`
	HTML     string `json:"html"`
	HTMLFile string `json:"html_file"`
}

func (a app) cmdWorkBatch(ctx context.Context, format string, configCtx configContext, args []string) int {
	if len(args) == 0 {
		return a.usageError("work batch requires edit or comment", format)
	}
	kind := args[0]
	args = args[1:]
	switch kind {
	case "edit":
		return a.cmdWorkBatchEdit(ctx, format, configCtx, args)
	case "comment":
		return a.cmdWorkBatchComment(ctx, format, configCtx, args)
	default:
		return a.usageError("unknown work batch subcommand: "+kind, format)
	}
}

func (a app) cmdWorkBatchEdit(ctx context.Context, format string, configCtx configContext, args []string) int {
	flags, args := parseMutationFlags(args)
	filePath, args, ok := parseStringFlag(args, "--file")
	concurrency, args, _ := parseStringFlag(args, "--concurrency")
	if concurrency != "" && concurrency != "1" {
		return a.writeCLIError(newError("VALIDATION_FAILED", "work batch edit currently supports --concurrency 1 only.", "Use the default serial execution.", false), format)
	}
	if !ok || filePath == "" {
		return a.writeCLIError(newError("VALIDATION_FAILED", "work batch edit requires --file <updates.json>.", "Pass a JSON file of update entries.", false), format)
	}
	if len(args) != 0 {
		return a.usageError("work batch edit takes flags only", format)
	}
	var inputs []batchEditInput
	if err := readJSONFile(filePath, &inputs); err != nil {
		return a.writeCLIError(err, format)
	}
	eff, client, configured := a.configuredPlaneClient(format, configCtx)
	if !configured {
		return exitError
	}
	data := workBatchData{WorkspaceSlug: eff.WorkspaceSlug.Value, Kind: "edit", Planned: len(inputs), Results: []workBatchResult{}}
	for _, input := range inputs {
		result := workBatchResult{Key: input.Key}
		if !flags.Apply {
			result.Skipped = true
			data.Skipped++
			data.Results = append(data.Results, result)
			continue
		}
		item, err := client.getWorkItemByRef(ctx, input.Key)
		if err != nil {
			result.Error = err
			data.Failed++
			if err.Retryable {
				data.Retryable++
			}
			data.Results = append(data.Results, result)
			continue
		}
		changes := map[string]any{}
		if input.Title != "" {
			changes["name"] = input.Title
		}
		if input.Priority != "" {
			changes["priority"] = input.Priority
		}
		if input.DescriptionHTML != "" {
			changes["description_html"] = input.DescriptionHTML
		}
		if input.DescriptionFile != "" {
			body, err := readBodyInput("", false, "", input.DescriptionFile, true, "description_file")
			if err != nil {
				result.Error = err
				data.Failed++
				data.Results = append(data.Results, result)
				continue
			}
			changes["description_html"] = body
		}
		if len(input.LabelsAdd) > 0 || len(input.LabelsRemove) > 0 {
			labels, err := client.resolveLabelSet(ctx, item.ProjectID, item.LabelIDs, input.LabelsAdd, input.LabelsRemove)
			if err != nil {
				result.Error = err
				data.Failed++
				data.Results = append(data.Results, result)
				continue
			}
			changes["labels"] = labels
		}
		if len(changes) == 0 {
			result.Skipped = true
			data.Skipped++
			data.Results = append(data.Results, result)
			continue
		}
		updated, err := client.updateWorkItem(ctx, item.ProjectID, item.WorkItemID, changes)
		if err != nil {
			result.Error = err
			data.Failed++
			if err.Retryable {
				data.Retryable++
			}
			data.Results = append(data.Results, result)
			continue
		}
		updated = preserveWorkItemIdentity(updated, item)
		result.Applied = true
		data.Applied++
		if flags.Verify {
			verified, err := client.verifyWorkItemChanges(ctx, updated, changes)
			if err != nil {
				result.Error = err
				data.Failed++
				if err.Retryable {
					data.Retryable++
				}
			}
			result.Verified = verified
		}
		data.Results = append(data.Results, result)
	}
	return a.writeWorkBatch(format, "plane.work.batch.edit.v1", data)
}

func (a app) cmdWorkBatchComment(ctx context.Context, format string, configCtx configContext, args []string) int {
	flags, args := parseMutationFlags(args)
	filePath, args, ok := parseStringFlag(args, "--file")
	concurrency, args, _ := parseStringFlag(args, "--concurrency")
	if concurrency != "" && concurrency != "1" {
		return a.writeCLIError(newError("VALIDATION_FAILED", "work batch comment currently supports --concurrency 1 only.", "Use the default serial execution.", false), format)
	}
	if !ok || filePath == "" {
		return a.writeCLIError(newError("VALIDATION_FAILED", "work batch comment requires --file <comments.json>.", "Pass a JSON file of comment entries.", false), format)
	}
	if len(args) != 0 {
		return a.usageError("work batch comment takes flags only", format)
	}
	var inputs []batchCommentInput
	if err := readJSONFile(filePath, &inputs); err != nil {
		return a.writeCLIError(err, format)
	}
	eff, client, configured := a.configuredPlaneClient(format, configCtx)
	if !configured {
		return exitError
	}
	data := workBatchData{WorkspaceSlug: eff.WorkspaceSlug.Value, Kind: "comment", Planned: len(inputs), Results: []workBatchResult{}}
	for _, input := range inputs {
		result := workBatchResult{Key: input.Key}
		if !flags.Apply {
			result.Skipped = true
			data.Skipped++
			data.Results = append(data.Results, result)
			continue
		}
		body, err := readBodyInput(input.HTML, input.HTML != "", "html", input.HTMLFile, input.HTMLFile != "", "html_file")
		if err != nil {
			result.Error = err
			data.Failed++
			data.Results = append(data.Results, result)
			continue
		}
		item, err := client.getWorkItemByRef(ctx, input.Key)
		if err != nil {
			result.Error = err
			data.Failed++
			if err.Retryable {
				data.Retryable++
			}
			data.Results = append(data.Results, result)
			continue
		}
		comment, err := client.createWorkItemComment(ctx, item.ProjectID, item.WorkItemID, body)
		if err != nil {
			result.Error = err
			data.Failed++
			if err.Retryable {
				data.Retryable++
			}
			data.Results = append(data.Results, result)
			continue
		}
		result.Applied = true
		result.CommentID = comment.ID
		if flags.Verify {
			result.Verified = comment.ID != ""
		}
		data.Applied++
		data.Results = append(data.Results, result)
	}
	return a.writeWorkBatch(format, "plane.work.batch.comment.v1", data)
}

func (a app) cmdWorkChildren(ctx context.Context, format string, configCtx configContext, args []string) int {
	if len(args) == 0 {
		return a.usageError("work children requires a work item reference", format)
	}
	ref := args[0]
	args = args[1:]
	limitText, args, _ := parseStringFlag(args, "--limit")
	if len(args) != 0 {
		return a.usageError("work children takes one reference plus flags", format)
	}
	limit := 50
	if limitText != "" {
		parsed, err := parsePositiveInt(limitText)
		if err != nil {
			return a.writeCLIError(err, format)
		}
		limit = parsed
	}
	eff, client, configured := a.configuredPlaneClient(format, configCtx)
	if !configured {
		return exitError
	}
	item, err := client.getWorkItemByRef(ctx, ref)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	children, err := client.listWorkItemChildren(ctx, item, limit)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	data := workRelationData{WorkspaceSlug: eff.WorkspaceSlug.Value, WorkItem: &item, Children: children, Count: len(children)}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.work.children.v1", data))
		return exitOK
	}
	for _, child := range children {
		fmt.Fprintf(a.stdout, "%s\t%s\n", child.ReadableID, child.Name)
	}
	return exitOK
}

func (a app) cmdWorkParent(ctx context.Context, format string, configCtx configContext, args []string) int {
	if len(args) == 0 {
		return a.usageError("work parent requires set or clear", format)
	}
	action := args[0]
	flags, rest := parseMutationFlags(args[1:])
	if action != "set" && action != "clear" {
		return a.usageError("unknown work parent subcommand: "+action, format)
	}
	if (action == "set" && len(rest) != 2) || (action == "clear" && len(rest) != 1) {
		return a.usageError("work parent "+action+" has invalid arguments", format)
	}
	eff, client, configured := a.configuredPlaneClient(format, configCtx)
	if !configured {
		return exitError
	}
	child, err := client.getWorkItemByRef(ctx, rest[0])
	if err != nil {
		return a.writeCLIError(err, format)
	}
	parentID := ""
	parentRef := ""
	if action == "set" {
		parent, err := client.getWorkItemByRef(ctx, rest[1])
		if err != nil {
			return a.writeCLIError(err, format)
		}
		parentID = parent.WorkItemID
		parentRef = parent.ReadableID
	}
	changes := map[string]any{"parent": parentID}
	op := mutationOperation{Kind: "work.parent." + action, Target: child.ReadableID, ProjectID: child.ProjectID, WorkItemID: child.WorkItemID, Changes: changes, Before: map[string]any{"parent": child.ParentID}, After: map[string]any{"parent": parentID, "parent_ref": parentRef}}
	data := workRelationData{WorkspaceSlug: eff.WorkspaceSlug.Value, Operation: op}
	if flags.Apply {
		updated, err := client.updateWorkItemParent(ctx, child, parentID)
		if err != nil {
			return a.writeCLIError(err, format)
		}
		updated = preserveWorkItemIdentity(updated, child)
		data.Applied = true
		data.WorkItem = &updated
		if flags.Verify {
			fresh, err := client.verifyFreshWorkItem(ctx, updated)
			if err != nil {
				return a.writeCLIError(err, format)
			}
			data.Verified = sameRef(fresh.ParentID, parentID)
		}
	}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.work.parent."+action+".v1", data))
		return exitOK
	}
	fmt.Fprintf(a.stdout, "%s %s\n", op.Kind, appliedText(data.Applied))
	return exitOK
}

func (a app) cmdWorkRelation(ctx context.Context, format string, configCtx configContext, args []string) int {
	if len(args) == 0 {
		return a.usageError("work relation requires add or remove", format)
	}
	action := args[0]
	flags, rest := parseMutationFlags(args[1:])
	if action != "add" && action != "remove" {
		return a.usageError("unknown work relation subcommand: "+action, format)
	}
	if len(rest) == 0 {
		return a.usageError("work relation "+action+" requires a work item reference", format)
	}
	ref := rest[0]
	rest = rest[1:]
	blocksRef, rest, ok := parseStringFlag(rest, "--blocks")
	if !ok || blocksRef == "" {
		return a.writeCLIError(newError("VALIDATION_FAILED", "work relation "+action+" requires --blocks <work-item>.", "Pass the related work item readable ID.", false), format)
	}
	if len(rest) != 0 {
		return a.usageError("work relation "+action+" takes one reference plus flags", format)
	}
	eff, client, configured := a.configuredPlaneClient(format, configCtx)
	if !configured {
		return exitError
	}
	item, err := client.getWorkItemByRef(ctx, ref)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	related, err := client.getWorkItemByRef(ctx, blocksRef)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	op := mutationOperation{Kind: "work.relation." + action, Target: item.ReadableID, ProjectID: item.ProjectID, WorkItemID: item.WorkItemID, Changes: map[string]any{"relation_type": "blocking", "related_work_item": related.WorkItemID}}
	data := workRelationData{WorkspaceSlug: eff.WorkspaceSlug.Value, Operation: op}
	if flags.Apply {
		if action == "add" {
			relation, err := client.createWorkItemRelation(ctx, item, "blocking", related)
			if err != nil {
				return a.writeCLIError(err, format)
			}
			data.Applied = true
			data.Relation = &relation
			if flags.Verify {
				relations, err := client.listWorkItemRelations(ctx, item)
				if err != nil {
					return a.writeCLIError(err, format)
				}
				for _, candidate := range relations {
					if candidate.RelationType == "blocking" && candidate.RelatedWorkItemID == related.WorkItemID {
						data.Verified = true
						break
					}
				}
			}
		} else {
			relations, err := client.listWorkItemRelations(ctx, item)
			if err != nil {
				return a.writeCLIError(err, format)
			}
			var relation relationSummary
			for _, candidate := range relations {
				if candidate.RelationType == "blocking" && candidate.RelatedWorkItemID == related.WorkItemID {
					relation = candidate
					break
				}
			}
			if relation.ID == "" {
				return a.writeCLIError(newError("RELATION_NOT_FOUND", "Blocking relation was not found.", "Inspect work relations and retry if needed.", false), format)
			}
			if err := client.deleteWorkItemRelation(ctx, item, relation.ID); err != nil {
				return a.writeCLIError(err, format)
			}
			data.Applied = true
			data.Relation = &relation
			if flags.Verify {
				after, err := client.listWorkItemRelations(ctx, item)
				if err != nil {
					return a.writeCLIError(err, format)
				}
				data.Verified = true
				for _, candidate := range after {
					if candidate.RelationType == "blocking" && candidate.RelatedWorkItemID == related.WorkItemID {
						data.Verified = false
						break
					}
				}
			}
		}
	}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.work.relation."+action+".v1", data))
		return exitOK
	}
	fmt.Fprintf(a.stdout, "%s %s\n", op.Kind, appliedText(data.Applied))
	return exitOK
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

func readBodyInput(inline string, inlineSet bool, inlineFlag string, filePath string, fileSet bool, fileFlag string) (string, *cliError) {
	if inlineSet && fileSet {
		return "", newError("VALIDATION_FAILED", inlineFlag+" and "+fileFlag+" cannot be used together.", "Pass either inline HTML or a file path, not both.", false)
	}
	if !fileSet {
		return inline, nil
	}
	if filePath == "" {
		return "", newError("VALIDATION_FAILED", fileFlag+" requires a path.", "Pass "+fileFlag+" <path>.", false)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", newError("FILE_NOT_FOUND", "Input file not found: "+filePath, "Check the path and retry.", false)
		}
		return "", newError("FILE_READ_FAILED", "Could not read input file: "+filePath, "Check file permissions and retry.", false)
	}
	return string(data), nil
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
		payload := map[string]any{"workspace_slug": workspaceSlug, "operation": data.Operation, "applied": data.Applied, "verified": data.Verified, "work_item": data.WorkItem, "comment": data.Comment}
		if len(data.DuplicateCandidates) > 0 {
			payload["duplicate_candidates"] = data.DuplicateCandidates
		}
		writeJSON(a.stdout, okEnvelope(schema, payload))
		return exitOK
	}
	mode := "dry-run"
	if data.Applied {
		mode = "applied"
	}
	fmt.Fprintf(a.stdout, "%s %s\n", data.Operation.Kind, mode)
	return exitOK
}

func (a app) writeWorkBatch(format, schema string, data workBatchData) int {
	if format == "json" {
		writeJSON(a.stdout, okEnvelope(schema, data))
		return exitOK
	}
	fmt.Fprintf(a.stdout, "%s planned=%d applied=%d failed=%d\n", data.Kind, data.Planned, data.Applied, data.Failed)
	return exitOK
}

func appliedText(applied bool) string {
	if applied {
		return "applied"
	}
	return "dry-run"
}

func parseCSV(text string) []string {
	if text == "" {
		return nil
	}
	parts := strings.Split(text, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func readJSONFile(path string, out any) *cliError {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newError("FILE_NOT_FOUND", "Input file not found: "+path, "Check the path and retry.", false)
		}
		return newError("FILE_READ_FAILED", "Could not read input file: "+path, "Check file permissions and retry.", false)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return newError("VALIDATION_FAILED", "Input file is not valid JSON: "+path, "Fix the JSON file and retry.", false)
	}
	return nil
}

func mutationBefore(item workItemSummary, changes map[string]any) map[string]any {
	out := map[string]any{}
	for key := range changes {
		out[key] = workItemFieldValue(item, key)
	}
	return out
}

func mutationAfter(item workItemSummary, changes map[string]any) map[string]any {
	out := mutationBefore(item, changes)
	for key, value := range changes {
		out[key] = value
	}
	return out
}

func workItemFieldValue(item workItemSummary, key string) any {
	switch key {
	case "name":
		return item.Name
	case "description_html":
		return item.DescriptionHTML
	case "priority":
		return item.Priority
	case "labels":
		return item.LabelIDs
	case "assignees":
		return item.AssigneeIDs
	case "state":
		return item.StateID
	case "state_group":
		return item.StateGroup
	default:
		return nil
	}
}

func (c planeClient) projectWorkItemsForList(ctx context.Context, items []workItemSummary, fields []string, excerptLimit int, latestComments int) ([]map[string]any, *cliError) {
	fieldSet := map[string]bool{}
	for _, field := range fields {
		fieldSet[field] = true
	}
	if len(fieldSet) == 0 {
		fieldSet = map[string]bool{"readable_id": true, "work_item_id": true, "name": true, "state_id": true, "state_name": true, "state_group": true, "priority": true, "assignees": true, "labels": true, "description_excerpt": excerptLimit > 0, "latest_comments": latestComments > 0}
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row := map[string]any{}
		for field := range fieldSet {
			switch field {
			case "readable_id":
				row[field] = item.ReadableID
			case "work_item_id":
				row[field] = item.WorkItemID
			case "name":
				row[field] = item.Name
			case "state", "state_id":
				row["state_id"] = item.StateID
			case "state_name":
				row[field] = item.StateName
			case "state_group":
				row[field] = item.StateGroup
			case "priority":
				row[field] = item.Priority
			case "assignees":
				row[field] = item.AssigneeIDs
			case "labels":
				row[field] = item.LabelIDs
			case "created_at":
				row[field] = item.CreatedAt
			case "updated_at":
				row[field] = item.UpdatedAt
			case "description_excerpt":
				limit := excerptLimit
				if limit <= 0 {
					limit = 300
				}
				row[field] = excerptPlainText(item.DescriptionHTML, limit)
			case "latest_comments":
				if latestComments > 0 {
					comments, err := c.listWorkItemComments(ctx, item.ProjectID, item.WorkItemID, latestComments)
					if err != nil {
						return nil, err
					}
					row[field] = comments
				}
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func (c planeClient) resolveState(ctx context.Context, projectID, ref string) (stateSummary, *cliError) {
	states, err := c.listProjectStates(ctx, projectID)
	if err != nil {
		return stateSummary{}, err
	}
	matches := []stateSummary{}
	for _, state := range states {
		if sameRef(state.ID, ref) || sameRef(state.Name, ref) || sameRef(state.Group, ref) || sameRef(state.Slug, ref) {
			matches = append(matches, state)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	accepted := make([]string, 0, len(states))
	for _, state := range states {
		accepted = append(accepted, state.Name)
	}
	if len(matches) > 1 {
		err := newError("AMBIGUOUS_STATE", "State reference is ambiguous: "+ref, "Use a state UUID or exact state name.", false)
		err.AcceptedValues = accepted
		return stateSummary{}, err
	}
	err = newError("STATE_NOT_FOUND", "State not found: "+ref, "Use plane-cli state list --project <project> --format json to find valid states.", false)
	err.AcceptedValues = accepted
	return stateSummary{}, err
}

func (c planeClient) resolveLabelSet(ctx context.Context, projectID string, current []string, addRefs, removeRefs []string) ([]string, *cliError) {
	labels, err := c.listProjectLabels(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := stringSet(current)
	for _, ref := range addRefs {
		label, err := resolveLabel(labels, ref)
		if err != nil {
			return nil, err
		}
		out[label.ID] = true
	}
	for _, ref := range removeRefs {
		label, err := resolveLabel(labels, ref)
		if err != nil {
			return nil, err
		}
		delete(out, label.ID)
	}
	return setStrings(out), nil
}

func resolveLabel(labels []labelSummary, ref string) (labelSummary, *cliError) {
	for _, label := range labels {
		if sameRef(label.ID, ref) || sameRef(label.Name, ref) {
			return label, nil
		}
	}
	err := newError("LABEL_NOT_FOUND", "Label not found: "+ref, "Use a valid label name or UUID.", false)
	for _, label := range labels {
		err.AcceptedValues = append(err.AcceptedValues, label.Name)
	}
	return labelSummary{}, err
}

func (c planeClient) resolveAssigneeSet(ctx context.Context, projectID string, current []string, addRefs, removeRefs []string) ([]string, *cliError) {
	members, err := c.listProjectMembers(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := stringSet(current)
	for _, ref := range addRefs {
		member, err := resolveMember(members, ref)
		if err != nil {
			return nil, err
		}
		out[member.ID] = true
	}
	for _, ref := range removeRefs {
		member, err := resolveMember(members, ref)
		if err != nil {
			return nil, err
		}
		delete(out, member.ID)
	}
	return setStrings(out), nil
}

func resolveMember(members []memberSummary, ref string) (memberSummary, *cliError) {
	for _, member := range members {
		if sameRef(member.ID, ref) || sameRef(member.Email, ref) || sameRef(member.DisplayName, ref) || sameRef(member.FirstName, ref) || sameRef(member.LastName, ref) {
			return member, nil
		}
	}
	err := newError("MEMBER_NOT_FOUND", "Member not found: "+ref, "Use a member email, display name, or UUID.", false)
	for _, member := range members {
		if member.Email != "" {
			err.AcceptedValues = append(err.AcceptedValues, member.Email)
		}
	}
	return memberSummary{}, err
}

func stringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func setStrings(values map[string]bool) []string {
	out := []string{}
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func markdownToHTML(markdown string) string {
	blocks := strings.Split(strings.TrimSpace(markdown), "\n\n")
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		switch {
		case strings.HasPrefix(block, "# "):
			parts = append(parts, "<h1>"+escapeHTML(strings.TrimSpace(strings.TrimPrefix(block, "# ")))+"</h1>")
		case strings.HasPrefix(block, "## "):
			parts = append(parts, "<h2>"+escapeHTML(strings.TrimSpace(strings.TrimPrefix(block, "## ")))+"</h2>")
		default:
			parts = append(parts, "<p>"+escapeHTML(strings.ReplaceAll(block, "\n", " "))+"</p>")
		}
	}
	return strings.Join(parts, "")
}

func excerptPlainText(html string, limit int) string {
	text := stripHTML(html)
	if limit > 0 && len(text) > limit {
		return text[:limit]
	}
	return text
}

func stripHTML(html string) string {
	var b strings.Builder
	inTag := false
	spacePending := false
	for _, r := range html {
		switch r {
		case '<':
			inTag = true
			if b.Len() > 0 {
				spacePending = true
			}
		case '>':
			inTag = false
		default:
			if inTag {
				continue
			}
			if r == '\n' || r == '\r' || r == '\t' {
				r = ' '
			}
			if r == ' ' {
				spacePending = true
				continue
			}
			if spacePending && b.Len() > 0 {
				b.WriteByte(' ')
			}
			spacePending = false
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(htmlEntityText(b.String()))
}

func htmlEntityText(text string) string {
	replacer := strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'")
	return replacer.Replace(text)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func anyStringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, fmt.Sprint(item))
		}
		return out
	default:
		return nil
	}
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := stringSet(a)
	for _, value := range b {
		if !set[value] {
			return false
		}
	}
	return true
}
