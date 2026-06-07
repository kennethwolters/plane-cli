---
name: plane
description: Use Plane through plane-cli for project/work-item discovery, software delivery workflow, safe mutations, and release/project-management handoffs.
---

# Plane Skill

Use `plane-cli` as the deterministic substrate for Plane work. Prefer JSON outputs and typed errors over prose scraping.

## Ground Rules

- Run Plane commands with `--format json` whenever a command supports it.
- Never store or print API keys. `plane-cli` uses `PLANE_API_KEY`, `PLANE_BASE_URL`, and `PLANE_WORKSPACE_SLUG` from environment, cwd `.env`, or config according to the CLI contract.
- Before workspace-changing work, run:

```bash
plane-cli doctor --for-agent --format json
```

- Prefer semantic commands over raw API calls.
- Treat mutations as operation plans first: dry-run is the default; use `--apply` only after the intended change is clear; use `--verify` for important changes.
- Keep stdout as data. If you need to explain a decision, do it in chat or PR text, not by parsing human output.

## Session Sweep

Start project-management sessions from the repository's main branch with a clean working tree unless the user asked to operate inside an active branch.

1. Check local git state.
2. Check Plane configuration and auth:

```bash
plane-cli auth status --format json
plane-cli doctor --for-agent --format json
```

3. Discover relevant Plane context:

```bash
plane-cli project list --format json
plane-cli state list --project <project> --format json
plane-cli member list --project <project> --format json
plane-cli work list --project <project> --format json
```

4. Search before creating duplicates:

```bash
plane-cli search "<query>" --project <project> --format json
```

## Software Delivery Workflow

Use Plane work items to keep implementation, review, verification, and release state aligned.

### Finding Work

- Use `plane-cli resolve <PROJECT-123> --format json` for exact readable IDs.
- Use `plane-cli work get <PROJECT-123> --format json` for current details.
- Use `plane-cli search` before creating related work.
- If multiple projects are plausible, list projects first and ask one short disambiguation question.

### Creating Work

Draft the intended work item first:

```bash
plane-cli work create \
  --project <project> \
  --title "<short imperative title>" \
  --description-html "<p>Goal, scope, acceptance criteria.</p>" \
  --format json
```

Apply only when the plan is correct:

```bash
plane-cli work create \
  --project <project> \
  --title "<short imperative title>" \
  --description-html "<p>Goal, scope, acceptance criteria.</p>" \
  --apply --verify --format json
```

### Lifecycle

Use lifecycle commands for semantic state changes:

```bash
plane-cli work start <PROJECT-123> --reason "Implementation started" --apply --verify --format json
plane-cli work complete <PROJECT-123> --evidence "Tests passed; PR merged; release asset verified" --apply --verify --format json
plane-cli work reopen <PROJECT-123> --reason "Verification failed" --apply --verify --format json
plane-cli work cancel <PROJECT-123> --reason "No longer needed" --apply --verify --format json
```

Completion needs evidence. Reopen and cancel need a reason.

### Comments and Status

Add durable status updates as comments when useful for handoff:

```bash
plane-cli work comment <PROJECT-123> \
  --html "<p>Status: CI passed. Release v0.1.0 created. Smoke test pending.</p>" \
  --apply --verify --format json
```

Keep comments factual: what changed, what was verified, what remains.

## PR and Release Handoff

When Plane work maps to a code PR:

- Put goals, non-goals, scope, acceptance criteria, and verification in the PR body.
- Do not over-create Plane items for every tiny implementation step.
- If the PR is the review artifact, keep the Plane item as the tracking artifact.
- On merge, comment with the merged PR URL, commit, CI result, and any local/prod verification.
- For releases, record:
  - tag
  - release URL
  - asset names/checksums if relevant
  - local smoke-test result when real-instance verification is intentionally outside CI

## Error Handling

Use typed error codes to decide next steps:

- `MISSING_API_KEY`, `MISSING_BASE_URL`, `MISSING_WORKSPACE_SLUG`: ask for configuration or run config commands.
- `INVALID_API_KEY`, `INSUFFICIENT_PERMISSIONS`: stop and ask the user to fix access.
- `PROJECT_NOT_FOUND`, `WORK_ITEM_NOT_FOUND`: list/search before retrying.
- `VALIDATION_FAILED`: fix command flags before retrying.
- `VERIFY_FAILED`: stop, inspect current Plane state, and ask before applying another mutation.

Do not silently retry mutations after `VERIFY_FAILED`.
