# GitHub Issue Implementation Plans

Generated from all GitHub issues in `kennethwolters/plane-cli` on 2026-06-22.

Scope: issues #5, #6, #7, #8, and #10.

Planning rule: every implementation slice starts outside-in. First add or update the user-visible behavior spec, then add acceptance tests that execute the real `plane-cli` binary against a fake Plane HTTP server, then confirm the new test fails for the intended reason, then implement the smallest code that makes it pass. Unit tests are added for pure parsing, mapping, diffing, and validation helpers after the acceptance behavior is pinned.

Common verification loop for every issue:

1. Add the failing acceptance test in an existing `*_acceptance_test.go` file or a focused new one.
2. Run the targeted test and confirm the red phase is caused by the missing behavior, not a fixture error.
3. Implement the smallest behavior slice.
4. Add unit/table tests for helper logic introduced by the slice.
5. Run `go test ./...`.
6. For non-mutating commands, run a representative local command manually.
7. For mutating commands, verify dry-run output first, then apply/verify against the fake server, then optionally smoke test against an approved real Plane workspace.

## #5: `work list --state-group` fails when items only carry state IDs

Status: open

URL: https://github.com/kennethwolters/plane-cli/issues/5

Target outcome: `work list --state-group <group>` works when Plane returns work items with `state` or `state_id` UUIDs but no inlined state group. JSON output should include a normalized `state_group` when the CLI can resolve it.

### Acceptance tests first

Add `TestWorkListStateGroupResolvesStateIDOnlyItems`:

- Fake `/projects/` returns project `BACKEND`.
- Fake `/projects/project-backend/work-items/` returns at least two work items with only state UUIDs, for example one backlog and one started item.
- Fake `/projects/project-backend/states/` maps those UUIDs to state groups.
- Run:

```sh
plane-cli work list --project BACKEND --state-group backlog --limit 3 --format json
```

- Assert exit code `0`, schema `plane.work.list.v1`, exactly the backlog item is returned, and the returned item has `state_group: "backlog"`.

Add `TestWorkListEnrichesMissingStateGroupFromStateID`:

- Run list without a `--state-group` filter against the same fake server.
- Assert returned JSON includes resolved `state_group` for items that only had `state_id` in the raw API response.

Add a small table test for the state enrichment helper:

- Missing state ID stays unchanged.
- Unknown state ID stays unchanged.
- Known state ID fills group/name-derived fields without overwriting an already present `state_group`.

### Implementation slices

1. Introduce a helper that builds `map[stateID]stateSummary` from `listProjectStates`.
2. In `listWorkItems`, map raw work items first, detect whether any item needs state-group enrichment, and fetch project states once only when needed.
3. Fill missing `StateGroup` from the state map before filtering.
4. Apply `--state-group` after enrichment so the limit applies to matched items.
5. Keep state resolution inside the Plane client boundary so `search` benefits automatically because it already calls `listWorkItems`.

### Verification

- Targeted red/green command:

```sh
go test ./... -run 'TestWorkListStateGroup'
```

- Full suite:

```sh
go test ./...
```

- Optional real smoke test against the reported self-hosted shape:

```sh
plane-cli work list --project MOBILEAPP --state-group backlog --limit 3 --format json
```

## #6: lifecycle mutation output loses `readable_id` after successful update

Status: open

URL: https://github.com/kennethwolters/plane-cli/issues/6

Target outcome: lifecycle mutation responses preserve `data.work_item.readable_id` even when Plane PATCH responses omit readable/project identifier fields.

### Acceptance tests first

Add `TestWorkLifecycleApplyPreservesReadableIDWhenPatchOmitsIt`:

- Fake `GET /work-items/BACKEND-42/` returns a complete item with `readable_id` reconstructable from project identifier and sequence.
- Fake state list returns a cancelled or completed state.
- Fake `PATCH /projects/project-backend/work-items/work-42/` returns only raw UUID fields, for example `id`, `project`, `state`, and `state_group`, with no `readable_id`, `identifier`, or `sequence_id`.
- Run:

```sh
plane-cli work cancel BACKEND-42 --reason "not needed" --apply --verify --format json
```

- Assert exit code `0`, schema `plane.work.cancel.v1`, `applied: true`, `verified: true`, and `data.work_item.readable_id == "BACKEND-42"`.
- Assert `data.operation.target == "BACKEND-42"` so scripts can chain either field.

Add a unit/table test for identity preservation:

- Updated response missing readable ID inherits `ReadableID`, `ProjectIdentifier`, `ProjectID`, `WorkItemID`, and `SequenceID` from the pre-update item.
- Updated response does not overwrite non-empty updated fields with stale pre-update values.

### Implementation slices

1. Add a small `preserveWorkItemIdentity(updated, original workItemSummary) workItemSummary` helper.
2. Use it after `client.updateWorkItem` in `cmdWorkLifecycle`.
3. Use the same helper in `cmdWorkEdit` as a low-risk consistency improvement, because edit mutations have the same PATCH response risk.
4. Ensure verification uses the preserved readable ID when available, falling back to project/work UUIDs only when it is not.
5. Keep the JSON schema unchanged and only improve field population.

### Verification

```sh
go test ./... -run 'TestWorkLifecycleApplyPreservesReadableID|TestPreserveWorkItemIdentity'
go test ./...
```

Manual chaining check after a fake or real verified mutation:

```sh
plane-cli work cancel BACKEND-42 --reason "not needed" --apply --verify --format json
```

Then confirm `.data.work_item.readable_id` can be passed to `work get`.

## #7: help output omits supported work flags used by agents

Status: open

URL: https://github.com/kennethwolters/plane-cli/issues/7

Target outcome: `plane-cli help` documents the supported create/edit/lifecycle flags that agent workflows rely on.

### Acceptance tests first

Add `TestHelpIncludesAgentCriticalWorkFlags`:

- Run:

```sh
plane-cli help
```

- Assert exit code `0`.
- Assert stdout contains:
  - `work create`
  - `--description-html`
  - `--priority`
  - `work edit`
  - `work start|complete|reopen|cancel`
  - `--reason`
  - `--evidence`
  - `--pr`

Add a focused README/help consistency check only for command examples that are intentionally duplicated. Keep it minimal to avoid brittle prose coupling:

- Assert every work flag shown in README core examples or docs as currently supported appears in help.

### Implementation slices

1. Update `printUsage` in `cli.go` so work create includes `--description-html` and `--priority`.
2. Update work edit usage so it includes `--description-html` and `--priority`.
3. Update lifecycle usage so it includes `--reason`, `--evidence`, and `--pr`.
4. Review README and `docs/main-spec.md` for contradictions. Update only examples that describe already implemented behavior.
5. Avoid adding future flags to help until the command actually supports them.

### Verification

```sh
go test ./... -run TestHelpIncludesAgentCriticalWorkFlags
go test ./...
go run . help
```

## #8: Support deterministic env-file discovery across worktrees and monorepos

Status: closed

URL: https://github.com/kennethwolters/plane-cli/issues/8

Target outcome: no new feature work is required unless the issue is reopened. The repo already has env-file discovery and tests for the accepted behavior. The implementation plan is a closeout and regression-hardening plan.

### Existing evidence

Current implementation already includes:

- Global `--env-file` parsing before command dispatch.
- `PLANE_CLI_ENV_FILE`.
- Cwd `.env` and nearest ancestor `.env` discovery.
- Process environment precedence over env-file values.
- `doctor --for-agent --format json` source/path reporting without secret values.
- Typed explicit-env-file failures.

Current tests already cover:

- `TestEnvFileFlagAuthStatus`.
- `TestEnvFileEnvVarAuthStatus`.
- `TestAncestorDotenvDiscovery`.
- `TestProcessEnvOverridesEnvFile`.
- `TestDoctorReportsEnvFileSourcesAndRedacts`.
- `TestMissingExplicitEnvFileIsTyped`.

### Regression tests to add if this area changes

Add tests before any future config changes:

- Explicit `--env-file` beats cwd `.env` when both define the same key.
- `PLANE_CLI_ENV_FILE` beats cwd `.env`.
- Cwd `.env` beats ancestor `.env`.
- Missing explicit env file blocks `config get`, `auth status`, and `doctor` consistently while still redacting all secret-like strings.
- Unreadable explicit env file reports `ENV_FILE_READ_ERROR` with the file path and no secret content.

### Implementation slices

No implementation slice is needed for the closed issue right now. If reopened, add the failing regression first and keep precedence logic in `config.go` rather than spreading source selection into individual commands.

### Verification

```sh
go test ./... -run 'TestEnvFile|TestAncestorDotenv|TestProcessEnvOverrides|TestDoctorReportsEnvFile|TestMissingExplicitEnvFile'
go test ./...
```

## #10: Agent/operator ergonomics for work editing, comments, batches, and board sweeps

Status: open

URL: https://github.com/kennethwolters/plane-cli/issues/10

Target outcome: make common operator board-maintenance workflows possible from the CLI without falling back to the Plane UI or brittle shell loops. This issue is an epic. It should not be implemented in one PR.

Dependencies:

- Fix #5 early because compact board sweeps and search need reliable state-group resolution.
- Fix #6 early because all mutation output should preserve chainable readable IDs.
- Fix #7 early because users and agents need help output that matches the real command surface.

### Slice 1: file inputs for create, edit, and comment

Acceptance tests first:

- `work create --description-file body.html --dry-run --format json` reads the file once, does not POST, and returns content length plus a safe excerpt.
- `work comment BACKEND-42 --html-file comment.html --apply --verify --format json` posts exact file content.
- Missing file returns a typed JSON error that includes the path and no partial mutation.
- Inline `--html` and file input are mutually exclusive with a typed validation error.

Implementation:

- Add file-reading helpers near CLI boundary, not in the HTTP client.
- Add flags:
  - `work create --description-file`
  - `work edit --description-file`
  - `work comment --html-file`
- Defer Markdown support to a follow-up unless a small stdlib-only conversion policy is explicitly specified. HTML is the Plane API boundary today.

Verification:

```sh
go test ./... -run 'TestWork.*File'
go test ./...
```

### Slice 2: description editing, append behavior, and verification

Acceptance tests first:

- `work edit BACKEND-42 --description-html '<p>new</p>' --apply --verify --format json` PATCHes `description_html` and verifies by reading the item back.
- `work edit BACKEND-42 --append-description-html '<hr><p>note</p>' --dry-run --format json` reads the current item, shows before/after metadata, and does not PATCH.
- `--description-html` and `--append-description-html` together produce a typed validation error.

Implementation:

- Extend `verifyWorkItemChanges` to verify `description_html`.
- Add append planning by reading the current description from `getWorkItemByRef`.
- Include before/after metadata for description changes without dumping huge bodies by default.

Verification:

```sh
go test ./... -run 'TestWorkEdit.*Description'
go test ./...
```

### Slice 3: list and inspect comments

Acceptance tests first:

- `work comments BACKEND-42 --format json` returns schema `plane.work.comments.v1`.
- `work comments BACKEND-42 --limit 5 --format json` passes through or locally applies the limit deterministically.
- JSON includes comment ID, author fields when present, created/updated timestamps, raw HTML, and an excerpt.

Implementation:

- Add `cmdWorkComments`.
- Add `planeClient.listWorkItemComments`.
- Add a `commentSummary` expansion without breaking existing `work comment` output.
- Keep optional edit/delete comment commands out of the first slice unless Plane API behavior is confirmed.

Verification:

```sh
go test ./... -run 'TestWorkComments'
go test ./...
```

### Slice 4: compact board-sweep output

Acceptance tests first:

- `work list --project BACKEND --fields readable_id,name,state,state_group,priority,updated_at,assignees,labels,description_excerpt --description-excerpt 120 --format json` returns only requested fields or a documented stable projection.
- HTML descriptions are stripped/normalized in `description_excerpt`.
- `--include-comments-latest 2` adds latest comment excerpts and does not mutate.

Implementation:

- First decide whether `--fields` means sparse JSON objects or a stable full schema with omitted optional fields. Document that before coding.
- Add HTML-to-text normalization using stdlib token rules or a deliberately small local stripper with unit tests.
- Resolve states via #5's state enrichment.
- Add comment fetching only when `--include-comments-latest` is requested.

Verification:

```sh
go test ./... -run 'TestWorkList.*Fields|TestDescriptionExcerpt'
go test ./...
```

### Slice 5: metadata editing for priority, labels, assignees, and state

Acceptance tests first:

- Priority edit verifies final priority.
- Label add/remove resolves names to IDs, PATCHes additive/removal changes without replacing unrelated labels, and verifies final label set.
- Assignee add/remove resolves email/display name/UUID, does not replace unrelated assignees, and verifies final assignee set.
- `work move BACKEND-42 --state "Ready" --apply --verify --format json` resolves state by group/name/UUID and verifies final state.
- Ambiguous names return typed errors with candidates.

Implementation:

- Keep priority as the first small slice because the current command already accepts it.
- Add label discovery before label mutation if no label list client exists yet.
- Add resolver helpers for state/member/label names with table tests.
- Prefer additive PATCH semantics only after confirming Plane API accepts partial list updates. If not, read current set, compute full desired set, and include before/after in dry-run.

Verification:

```sh
go test ./... -run 'TestWorkEdit.*Priority|TestWorkEdit.*Label|TestWorkEdit.*Assignee|TestWorkMove'
go test ./...
```

### Slice 6: better dry-run diffs and verification output

Acceptance tests first:

- Dry-run mutation output includes `before`, `after`, and `changes`.
- Large description/comment changes show length and safe excerpts, not entire unbounded content.
- `--verify` output includes a final normalized work snapshot.
- Mutation output consistently includes readable ID, internal IDs, changed fields, and verification status.

Implementation:

- Introduce a mutation plan/result model behind the existing JSON envelope.
- Keep schema names stable if the structure is backward-compatible. If not, introduce a new schema version intentionally.
- Reuse #6 identity preservation in all mutation results.

Verification:

```sh
go test ./... -run 'TestWork.*DryRun|TestWork.*Verify'
go test ./...
```

### Slice 7: batch edit and batch comment

Acceptance tests first:

- `work batch edit --file updates.json --dry-run --format json` validates every entry, reads referenced files, and performs no mutations.
- `work batch comment --file comments.json --apply --verify --format json` applies serially by default and returns applied/skipped/failed counts.
- A fake server returning HTTP 429 with `Retry-After` produces a retryable result with retry-after metadata.
- Partial failure does not hide successful operations and exits with a documented status.

Implementation:

- Start with `--concurrency 1`.
- Parse JSON files into explicit structs with validation.
- Add retry/backoff only around retryable API calls.
- Extend `cliError` if needed to carry `retry_after_seconds`.
- Do not add parallel mutation until serial summaries are stable.

Verification:

```sh
go test ./... -run 'TestWorkBatch'
go test ./...
```

### Slice 8: relationships

Acceptance tests first:

- `work children BACKEND-42 --format json` lists children if Plane exposes them.
- `work parent set CHILD-43 BACKEND-42 --apply --verify --format json` resolves both items and verifies the relationship.
- `work relation add BACKEND-42 --blocks BACKEND-43 --apply --verify --format json` verifies relation graph after apply.

Implementation:

- Do a short API discovery spike first and record exact endpoints/payloads in a test fixture.
- Keep parent/child separate from arbitrary relation types if the API models them separately.
- Return typed `API_UNSUPPORTED` if a self-hosted Plane version lacks the endpoint.

Verification:

```sh
go test ./... -run 'TestWork.*Relation|TestWork.*Parent|TestWork.*Children'
go test ./...
```

### Slice 9: stronger search and duplicate detection

Acceptance tests first:

- `search "oauth" --project BACKEND --include-comments --max-results 20 --format json` returns matched field and excerpt.
- State filtering in search works when work items only have state IDs, using #5 enrichment.
- `work create --project BACKEND --title "OAuth bug" --dedupe-query "oauth" --dry-run --format json` reports duplicate candidates and does not mutate.

Implementation:

- Extend `searchData` with match metadata rather than only returning work item summaries.
- Fetch comments only when `--include-comments` is requested.
- For create dedupe, make the first slice warn in dry-run. Decide later whether apply should fail by default or require `--allow-duplicate`.

Verification:

```sh
go test ./... -run 'TestSearch|TestWorkCreate.*Dedupe'
go test ./...
```

### Slice 10: more actionable machine-readable errors

Acceptance tests first:

- HTTP 429 maps to `RATE_LIMITED`, `retryable: true`, and `retry_after_seconds`.
- HTTP validation errors include stable code, message, fix, and accepted values when the CLI can know them locally.
- Name-resolution failures include candidates for ambiguous matches.

Implementation:

- Extend `cliError` carefully so existing error JSON remains compatible.
- Read response bodies only up to a safe limit when extracting validation details.
- Add resolver-specific errors for missing/ambiguous state, label, and member names.

Verification:

```sh
go test ./... -run 'Test.*Error|Test.*RateLimit|Test.*Ambiguous'
go test ./...
```

### Epic closeout verification

Before closing #10, run an end-to-end fake-server operator workflow:

1. Compact list a project board.
2. Inspect a work item and latest comments.
3. Edit a description from a file.
4. Add a comment from a file.
5. Update priority/labels/state.
6. Run a small batch comment or edit.
7. Verify every mutation from read-back JSON.

Final evidence should include:

```sh
go test ./...
```

And, when an approved real workspace is available, a non-destructive real Plane smoke pass using dry-run first and apply/verify only on disposable test work items.
