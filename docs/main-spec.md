# Main CLI Spec

This is the high-level command-surface spec for `plane-cli` after the V1 meta slice. It defines the Pareto-optimal commands an agent needs to operate a Plane workspace safely without mirroring the entire REST API.

## Design constraints

- Binary name: `plane-cli`.
- Go standard library only unless a future design decision explicitly allows a dependency.
- Plane PAT auth uses `X-API-Key`.
- Never store API keys; read `PLANE_API_KEY` from process env, explicit env files, cwd/ancestor `.env`, or safe config sources according to precedence.
- Every important command supports deterministic JSON via `--format json`.
- stdout is data; stderr is diagnostics/progress.
- Mutations must be reviewable and verifiable: plan/dry-run first, then apply, then verify.
- Prefer state groups (`backlog`, `unstarted`, `started`, `completed`, `cancelled`) over project-local state names where possible.
- Prefer semantic operations over raw CRUD. Raw API access is an escape hatch, not the primary UX.

## Engineering philosophy compliance

This spec follows [`docs/engineering-philosophy.md`](./engineering-philosophy.md):

- **Small, trustworthy CLI:** commands are grouped around user-visible behavior, not an internal SDK or framework.
- **Mechanical sympathy:** command groups map to real Plane concepts: projects, work items, states, cycles, modules, relations, intake, pages, and worklogs.
- **Vendor seam discipline:** Plane REST details stay behind stable command contracts and versioned JSON schemas.
- **Outside-in TDD:** each implementation slice should start by turning one command group into acceptance tests before adding internals.
- **Done means verified:** every mutation command is specified as dry-run/apply/verify-capable.
- **Stdlib-first Go:** no dependency is required by this spec.
- **Scope discipline:** the Pareto implementation order below is the intended PR slicing guide; do not implement this entire spec in one PR.

## Common command conventions

### Output

All JSON commands use a stable envelope:

```json
{
  "ok": true,
  "schema": "plane.<command>.v1",
  "data": {},
  "warnings": [],
  "hints": [],
  "suggested_next_commands": []
}
```

Errors use:

```json
{
  "ok": false,
  "schema": "plane.error.v1",
  "error": {
    "code": "STABLE_CODE",
    "message": "Human-readable message.",
    "fix": "Actionable fix.",
    "retryable": false,
    "examples": []
  }
}
```

### References

Commands should accept stable human references where possible:

- work item: `ENG-42`
- project: project identifier (`ENG`) or project UUID
- user: `@handle`, email, or user UUID
- state: state group first, then state name/UUID when necessary
- cycle/module: name, UUID, or aliases like `current`, `upcoming`, `none`

Ambiguity must return typed errors with candidates, never silent guessing.

### Mutation lifecycle

All mutating semantic commands must support:

```sh
--dry-run        # produce an operation plan, no mutation
--apply          # apply immediately, normally only after agent review
--verify         # verify resulting Plane state after apply
--reason <text>  # why this change is being made, where applicable
```

Dry-run output produces an operation object that can be applied later:

```sh
plane-cli apply <operation-id-or-file> --verify --format json
plane-cli verify <operation-id-or-last> --format json
plane-cli ops log --format json
```

## Command surface

### 1. Meta and configuration

Already implemented or specified in the V1 meta slice.

```sh
plane-cli version [--format text|json]
plane-cli --version
plane-cli config get [--format text|json]
plane-cli config set <base_url|workspace_slug> <value> [--format text|json]
plane-cli auth status [--format text|json]
plane-cli doctor [--for-agent] [--format text|json]
plane-cli resolve <reference> [--format text|json] [--no-cache]
```

Purpose:

- establish trust in the binary/config/auth
- resolve readable work item IDs to UUIDs
- give agents typed diagnostics before touching workspace state

### 2. Discovery and search

Agents need to find the right object before they mutate anything.

```sh
plane-cli workspace info [--format text|json]
plane-cli project list [--archived] [--format text|json]
plane-cli project get <project> [--format text|json]
plane-cli member list [--project <project>] [--format text|json]
plane-cli state list [--project <project>] [--format text|json]
plane-cli label list [--project <project>] [--format text|json]
plane-cli search <query> [--project <project>] [--state-group <group>] [--assignee <user>] [--max-results <n>] [--format text|json]
```

Must answer:

- what workspace am I in?
- what projects exist?
- who can own work?
- which state/label identifiers are safe to use?
- which item best matches a vague reference?

### 3. Work item read model

Agents need compact, normalized issue context without hand-stitching many API responses.

```sh
plane-cli work get <reference> [--include comments,relations,links,worklogs,activity] [--format text|json]
plane-cli work list [--project <project>] [--cycle <cycle>] [--module <module>] [--state-group <group>] [--assignee <user>] [--label <label>] [--limit <n>] [--format text|json]
plane-cli work activity <reference> [--limit <n>] [--format text|json]
```

Output should normalize:

- readable ID and UUIDs
- title/description HTML summary
- project/state/state group
- assignees, labels, priority
- cycle/module membership
- blockers and blocked-by edges
- recent comments/activity
- timestamps and stale signals

### 4. Work item creation and safe updates

These are the core direct changes an agent must be able to make.

```sh
plane-cli work create --project <project> --title <title> [--description-html <html>] [--assignee <user>] [--label <label>] [--priority <priority>] [--dry-run|--apply] [--verify] [--format text|json]
plane-cli work edit <reference> [--title <title>] [--description-html <html>] [--priority <priority>] [--assignee <user>] [--unassign <user>] [--label <label>] [--remove-label <label>] [--dry-run|--apply] [--verify] [--format text|json]
plane-cli work comment <reference> --html <html> [--dry-run|--apply] [--verify] [--format text|json]
```

Rules:

- descriptions/comments are HTML at the API boundary
- dry-run must show exact fields to be changed
- output must never require scraping Plane's raw response shape

### 5. Semantic work state operations

Agents most often need to move work through a lifecycle with evidence.

```sh
plane-cli work start <reference> [--reason <text>] [--dry-run|--apply] [--verify] [--format text|json]
plane-cli work complete <reference> --evidence <text> [--pr <url-or-number>] [--dry-run|--apply] [--verify] [--format text|json]
plane-cli work reopen <reference> --reason <text> [--dry-run|--apply] [--verify] [--format text|json]
plane-cli work cancel <reference> --reason <text> [--dry-run|--apply] [--verify] [--format text|json]
plane-cli work handoff <reference> --to <user> --summary <text> [--include context,risks,next-actions] [--dry-run|--apply] [--verify] [--format text|json]
```

Behavior:

- resolve project-specific state UUIDs from stable state groups
- include comment/evidence when completing, reopening, cancelling, or handing off
- verify final state group after apply

### 6. Blockers, relations, and graph operations

Blocker handling is one of the highest-value agent workflows.

```sh
plane-cli graph blockers [--project <project>] [--cycle <cycle>] [--format text|json]
plane-cli work block <reference> --by <reference> [--reason <text>] [--dry-run|--apply] [--verify] [--format text|json]
plane-cli work unblock <reference> --blocked-by <reference> --reason <text> [--dry-run|--apply] [--verify] [--format text|json]
plane-cli work relate <reference> --to <reference> --type blocking|blocked_by|duplicate_of|duplicate|relates_to [--dry-run|--apply] [--verify] [--format text|json]
plane-cli work unlink <reference> --to <reference> --type <type> [--dry-run|--apply] [--verify] [--format text|json]
```

Must support:

- resolving both sides of a relation
- detecting duplicate/existing relations
- explaining why a work item is blocked
- verifying relation graph after apply

### 7. Planning containers: cycles and modules

Cycles/modules drive most operational planning. The CLI should make movement diffable.

```sh
plane-cli cycle list [--project <project>] [--current] [--upcoming] [--format text|json]
plane-cli cycle get <cycle> [--project <project>] [--include work] [--format text|json]
plane-cli cycle add <cycle> <reference...> [--dry-run|--apply] [--verify] [--format text|json]
plane-cli cycle remove <cycle> <reference...> [--dry-run|--apply] [--verify] [--format text|json]
plane-cli module list [--project <project>] [--format text|json]
plane-cli module get <module> [--project <project>] [--include work] [--format text|json]
plane-cli module add <module> <reference...> [--dry-run|--apply] [--verify] [--format text|json]
plane-cli module remove <module> <reference...> [--dry-run|--apply] [--verify] [--format text|json]
```

Output should show before/after membership for mutations.

### 8. Triage and intake

Agents need to convert incoming vague work into actionable Plane items.

```sh
plane-cli intake list [--project <project>] [--format text|json]
plane-cli intake get <intake-item> [--format text|json]
plane-cli intake accept <intake-item> [--project <project>] [--dry-run|--apply] [--verify] [--format text|json]
plane-cli intake reject <intake-item> --reason <text> [--dry-run|--apply] [--verify] [--format text|json]
plane-cli triage <reference-or-intake-item> [--format text|json]
```

`triage` should report missing owner, unclear acceptance criteria, duplicates, blockers, and suggested next commands.

### 9. Context and diagnosis

This is the agent-native layer. It should reduce uncertainty instead of just returning raw objects.

```sh
plane-cli context <reference> [--token-budget <n>] [--include comments,relations,links,pages,activity] [--format text|json]
plane-cli context project <project> [--token-budget <n>] [--format text|json]
plane-cli diagnose <reference> [--why-stuck] [--format text|json]
plane-cli missing <acceptance-criteria|owner|blocker-link|evidence|estimate> [--project <project>] [--cycle <cycle>] [--format text|json]
```

Must provide:

- compact context capsule
- uncertainty and missing information
- likely blockers/staleness
- evidence needed to complete work
- suggested next commands

### 10. Worklogs and proof of work

Worklogs are useful for accountability but must be normalized.

```sh
plane-cli worklog add <reference> --minutes <n> --note <text> [--dry-run|--apply] [--verify] [--format text|json]
plane-cli worklog list <reference> [--format text|json]
plane-cli work prove-done <reference> --evidence <text> [--pr <url-or-number>] [--dry-run|--apply] [--verify] [--format text|json]
```

Rules:

- emit minutes, not ambiguous human strings
- proof commands should attach evidence as comments/links and verify completed state when requested

### 11. Pages and links as context sources

Agents often need project knowledge without dumping entire pages into prompts.

```sh
plane-cli page list [--project <project>] [--format text|json]
plane-cli page get <page> [--project <project>] [--summary] [--max-chars <n>] [--format text|json]
plane-cli link add <reference> --url <url> [--title <title>] [--dry-run|--apply] [--verify] [--format text|json]
plane-cli link list <reference> [--format text|json]
plane-cli link remove <reference> --url <url> [--dry-run|--apply] [--verify] [--format text|json]
```

### 12. Operations, verification, and audit

Operation commands are the safety rail for all mutations.

```sh
plane-cli apply <operation-id-or-file> [--verify] [--format text|json]
plane-cli verify <operation-id-or-last> [--format text|json]
plane-cli ops log [--limit <n>] [--format text|json]
plane-cli ops show <operation-id> [--format text|json]
plane-cli ops revert <operation-id> [--dry-run|--apply] [--verify] [--format text|json]
```

A revert should create a compensating operation, not pretend to erase history.

### 13. Budgeting and raw escape hatch

```sh
plane-cli budget status [--format text|json]
plane-cli raw get <path> [--format json]
plane-cli raw post <path> --body <json-file-or-stdin> [--format json]
plane-cli raw patch <path> --body <json-file-or-stdin> [--format json]
plane-cli raw delete <path> [--format json]
```

`raw` is for debugging and API gaps. It should still apply auth, base URL config, redaction, and error mapping.

## Pareto-optimal 80% target

The goal is not full Plane API parity. The goal is an 80%-useful CLI for agent-driven workspace operation: agents can safely find, understand, create, update, move, and verify most work without raw API calls.

Required capability groups for that state:

1. **Work item core**
   - `work list`
   - `work get`
   - `work create`
   - `work edit`
   - `work comment`

2. **Safe lifecycle operations**
   - `work start`
   - `work complete --evidence`
   - `work reopen --reason`
   - `work cancel --reason`
   - state-group-driven transitions, not brittle local state-name guesses

3. **Relations and blockers**
   - `graph blockers`
   - `work block`
   - `work unblock`
   - `work relate`
   - `work unlink`

4. **Planning containers**
   - `cycle list/get/add/remove`
   - `module list/get/add/remove`
   - before/after membership diffs for mutations

5. **Search and ambiguity handling**
   - `search <query>`
   - improved `resolve` for vague references and candidate lists
   - typed `AMBIGUOUS_REFERENCE` output instead of guessing

6. **Context and diagnosis**
   - `context <work-item>`
   - `diagnose <work-item> --why-stuck`
   - `missing acceptance-criteria|owner|evidence|blocker-link`

7. **Operation safety**
   - operation plans for mutations
   - `--dry-run`, `--apply`, `--verify`
   - `apply`
   - `verify`
   - minimal local operation log

8. **Pagination and error hardening**
   - cursor pagination helper
   - stable API error mapping
   - validation errors exposed cleanly
   - no secret leakage tests around every command family

At that point, the CLI is likely 70-80% useful for real agent operation even though it is not API-complete.

## Pareto implementation order

The smallest command sequence that unlocks most agent value:

1. `project list`, `project get`, `state list`, `member list`
2. `work get`, `work list`, `search`
3. `work create`, `work edit`, `work comment`
4. `work start`, `work complete`, `work reopen`, `work cancel`
5. mutation operation model: `--dry-run`, `--apply`, `--verify`, `apply`, `verify`
6. `graph blockers`, `work block`, `work unblock`, `work relate`, `work unlink`
7. `cycle list/get/add/remove`
8. `module list/get/add/remove`
9. `context`, `diagnose`, `missing`
10. `worklog add/list`, `work prove-done`
11. `intake` and `triage`
12. `page` and `link` commands
13. `ops` and `raw`

## Stable error families

Future commands should extend V1 error codes with families like:

- `AMBIGUOUS_REFERENCE`
- `PROJECT_NOT_FOUND`
- `USER_NOT_FOUND`
- `STATE_NOT_FOUND`
- `LABEL_NOT_FOUND`
- `CYCLE_NOT_FOUND`
- `MODULE_NOT_FOUND`
- `RELATION_ALREADY_EXISTS`
- `RELATION_NOT_FOUND`
- `VALIDATION_FAILED`
- `DRY_RUN_REQUIRED`
- `VERIFY_FAILED`
- `OPERATION_NOT_FOUND`

## Non-goals

- Full Plane API parity in the happy-path command surface.
- OAuth in the first core endpoint pass.
- Hidden defaults for workspace/base URL.
- Storing secrets.
- Agent prose generation inside the CLI; the CLI returns facts, plans, checks, and typed suggestions.
