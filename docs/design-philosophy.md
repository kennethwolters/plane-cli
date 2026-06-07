# Design Philosophy

This is a working note, not an architecture decision. Treat it as a bias document for future design discussions.

## Thesis

Build a Plane-native CLI for agents, not a generic Jira-style CRUD wrapper.

The CLI should expose the shape of project work: uncertainty, blockers, evidence, handoffs, stale context, and safe state transitions. Plane already has a broad API and an MCP server; this CLI should be a judgment layer that is terminal-native, scriptable, and easy for agents to reason about.

## Core rule

Every non-raw command should do at least one of these:

1. Reduce uncertainty.
2. Change Plane state safely.
3. Verify that state changed correctly.

If a command does none of those, it probably belongs under `plane raw`.

## Agent-native CLI principles

### Prefer diagnosis over transport

Weak:

```sh
plane issue get ENG-42
```

Stronger:

```sh
plane diagnose ENG-42 --why-stuck --format json
plane context ENG-42 --capsule agent
```

The CLI should summarize what matters: current state, likely problem, evidence, ambiguity, safe actions, unsafe actions, and suggested next commands.

### Make missing information first-class

Agents often need to know what is absent, not just what exists.

Examples:

```sh
plane missing acceptance-criteria --project WEB
plane missing blocker-link --cycle current
plane missing owner --cycle current
plane missing evidence --state done
```

### Use semantic work operations

Avoid mirroring raw API mutation as the default UX.

Prefer:

```sh
plane work unblock ENG-42 --because "OAuth redirect URI approved" --dry-run
plane work prove-done ENG-42 --pr 123 --note "Verified in staging" --dry-run
plane work handoff ENG-42 --to @alice --include context,risks,next-actions
```

Over:

```sh
plane raw work-item update ENG-42 --state done
```

### Dry-run, apply, verify

Mutations should feel like patches:

```sh
plane work unblock ENG-42 --because "..." --dry-run
plane apply op_123 --verify
plane verify last
```

Dry-runs should produce reviewable operation objects. Reverts should produce compensating operations, not magic undo.

### Treat ambiguity as typed output

Do not let agents guess.

```sh
plane resolve "oauth bug" --format json
```

If multiple matches exist, return `AMBIGUOUS_REFERENCE` with candidates and disambiguators.

### Emit agent-readable output

Default human output can exist, but every important command should support deterministic JSON. Long-running scans should support NDJSON events.

Common envelope shape:

```json
{
  "ok": true,
  "schema": "plane.command.v1",
  "request_id": "req_...",
  "data": {},
  "warnings": [],
  "hints": [],
  "budget": {
    "api_calls_used": 0,
    "context_tokens_estimate": null
  },
  "suggested_next_commands": []
}
```

Errors should include stable codes, a fix, retryability, and examples.

### Keep budgets visible

Plane API calls, pagination, result counts, and context-token estimates should be surfaced in-band where relevant.

```sh
plane budget status
plane search "oauth" --max-api-calls 5 --max-results 10
plane context project WEB --token-budget 4000
```

### Use MCP for breadth, CLI for judgment

The CLI should not compete with Plane MCP. Use MCP or raw REST for broad API access; use the CLI for curated workflows, deterministic output, dry-run/apply/verify, shell scripting, and CI.

A possible layering:

```text
Agent
  -> Plane skill / project instructions
  -> plane CLI: diagnosis, workflows, safety, verification
  -> Plane REST API or Plane MCP
  -> Plane workspace
```

### Plane MCP alpha to exploit

Plane's MCP server is a useful map of the raw surface. It exposes 100+ tools across roughly 20 modules, including users, workspaces, projects, work items, activities, comments, links, relations, custom properties, work item types, worklogs, states, labels, cycles, modules, epics, milestones, initiatives, intake, and pages.

Design implications:

- **Identifier translation is core.** Plane has readable work item IDs like `ENG-42`, but most tools require UUIDs for `project_id`, `work_item_id`, `cycle_id`, etc. The CLI should make `plane resolve ENG-42` and internal UUID resolution first-class, cacheable, and visible in JSON output.
- **MCP's breadth creates multi-call workflows.** A simple request like "mark ENG-88 done and comment" requires resolving the work item, listing states to find the Done UUID, updating the item, then creating an HTML comment. The CLI can compress these into one semantic operation with dry-run/apply/verify.
- **Workspace search is powerful but conditional.** MCP `list_work_items` switches to advanced search when filters are present and can search workspace-wide. The CLI should expose that deliberately: `plane search`, `plane resolve`, and `plane missing` should show whether they used project list mode or workspace search mode.
- **State groups are more stable than state names.** Plane state groups include `backlog`, `unstarted`, `started`, `completed`, and `cancelled`; local state names vary by project. Agent-facing commands should reason in groups when possible and resolve project-specific state UUIDs at apply time.
- **Relations are project graph primitives.** MCP relation types include `blocking`, `blocked_by`, `duplicate_of`, `duplicate`, and `relates_to`. This validates making `plane graph`, `plane work unblock`, `plane work merge`, and `plane diagnose --why-stuck` central, not optional extras.
- **Comments are HTML.** Semantic communication commands (`ask`, `nudge`, `handoff`, `escalate`, `prove-done`) should own safe HTML rendering instead of letting agents hand-roll comment markup.
- **Worklogs are minutes.** Time logging should avoid ambiguous human strings at the API boundary. Accept friendly input if useful, but emit normalized minutes and show exactly what will be logged.
- **Cycles and modules are first-class planning containers.** MCP has transfer/add/remove/list tools for cycles and modules. The CLI should support what-if planning and diffable movement between cycles/modules, not just item mutation.
- **Intake and initiatives matter.** Intake is the triage queue; initiatives are workspace-scoped strategy. `plane triage`, `plane intake`, and `plane initiative` may become useful higher-level surfaces later.
- **Pages are context sources.** Plane pages can be workspace- or project-scoped. Context capsules should be able to cite relevant pages without dumping full page bodies by default.
- **Auth mode shapes UX.** MCP supports hosted HTTP with OAuth, HTTP with PAT headers, local stdio via `PLANE_API_KEY`/`PLANE_WORKSPACE_SLUG`/`PLANE_BASE_URL`, and legacy SSE. The CLI should support PAT/workspace/base-url cleanly for automation and self-hosting; OAuth can be added if human login becomes important.
- **Self-hosting has two URLs.** MCP distinguishes `PLANE_BASE_URL` from `PLANE_INTERNAL_BASE_URL` for server-to-server calls. The CLI should at least model public API base URL explicitly and not assume Plane Cloud.
- **Troubleshooting should be typed.** MCP propagates SDK/API errors for invalid key, expired OAuth token, missing workspace slug, wrong slug, insufficient permissions, not found, validation errors, Redis/token storage issues, and network errors. The CLI should map these to stable error codes and `plane doctor` checks.

The raw MCP workflows are good evidence that the CLI's value is not access. Access already exists. The value is reducing four or five brittle primitive calls into one reviewable operation with typed ambiguity and verification.

## Extra design bias

### From Mario Zechner-style agent tooling

Useful biases from Mario Zechner's writing on pi and CLI-vs-MCP tooling:

- Prefer simple, predictable tools over spaceship interfaces.
- Keep context engineering explicit: only put into the model what it needs.
- Use progressive disclosure: document the CLI well, let agents read docs when needed, avoid dumping huge tool schemas into context.
- Make tool output token-efficient and clean; protocol choice matters less than whether the tool helps the agent complete the task.
- Preserve inspectability: sessions, operations, and outputs should be easy to review and post-process.
- Prefer shell composability where possible; a good CLI can be piped, filtered, scripted, tested, and later wrapped by MCP if needed.

### From Armin Ronacher-style API/CLI design

Useful biases from Armin Ronacher's projects and writing culture around Flask, Jinja, Click, and API design:

- Small, sharp interfaces age better than broad magical ones.
- Good defaults matter, but hidden behavior should be rare and explainable.
- Documentation is part of the interface; examples should teach the safe path.
- Design escape hatches deliberately. `plane raw` is useful, but should feel like an escape hatch, not the happy path.
- Prefer composable primitives over a giant framework surface.
- Security and trust boundaries should be explicit in the API shape, especially for commands that mutate durable project state.

### From Go's design philosophy

Useful Go-shaped biases for this project:

- Be boring and explicit.
- Prefer a small orthogonal command surface over clever abstractions.
- Return errors as values with stable codes and actionable fixes.
- Favor composition: context, diagnosis, operation planning, application, and verification should compose cleanly.
- Keep formatting deterministic. Machines should not have to scrape prose.
- Optimize for maintainability over novelty; surprise is a cost.

## Command-surface inspiration

Not a commitment, but useful examples:

```sh
# setup / health
plane whoami --format json
plane doctor --for-agent
plane auth status --format json
plane budget status

# resolution / context
plane resolve ENG-42 --format json
plane resolve "oauth bug" --format json
plane context ENG-42 --capsule agent
plane context project WEB --token-budget 4000

# diagnosis
plane diagnose ENG-42 --why-stuck
plane diagnose cycle current --scope blockers,orphans,duplicates
plane missing acceptance-criteria --project WEB
plane missing blocker-link --cycle current

# graph
plane graph blockers ENG-42
plane graph critical-path --cycle current
plane graph unblock-plan --project WEB
plane graph duplicates --project WEB

# planning containers
plane cycle transfer-incomplete --from "Sprint 14" --to "Sprint 15" --dry-run
plane module move ENG-55 --to "Checkout Redesign" --dry-run

# semantic operations
plane work clarify ENG-42 --field acceptance-criteria --dry-run
plane work unblock ENG-42 --because "..." --dry-run
plane work prove-done ENG-42 --pr 123 --note "..." --dry-run
plane work split ENG-42 --strategy by-acceptance-criteria --dry-run
plane work merge ENG-42 ENG-77 --because duplicate --dry-run
plane work defer ENG-42 --until next-cycle --reason "..." --dry-run

# social operations
plane ask ENG-42 --for "decision from design" --dry-run
plane nudge ENG-42 --style polite --dry-run
plane escalate ENG-42 --because "blocked 5 days" --dry-run
plane handoff ENG-42 --to @alice --include context,risks,next-actions
plane worklog ENG-42 --minutes 90 --note "Implemented retry logic" --dry-run

# operation lifecycle
plane apply op_123 --verify
plane verify last
plane ops list --actor agent
plane ops revert op_123 --dry-run

# raw escape hatch
plane raw project list
plane raw work-item get ENG-42
plane raw work-item update ENG-42 --state done --unsafe-reason "manual override"
```

## References

- Supplied agent-native CLI report in the initial project notes.
- Plane MCP server spec: <https://developers.plane.so/dev-tools/mcp-server>
- Mario Zechner: <https://mariozechner.at/posts/2025-11-30-pi-coding-agent/>
- Mario Zechner: <https://mariozechner.at/posts/2025-08-15-mcp-vs-cli/>
- Armin Ronacher: <https://lucumr.pocoo.org/>
- Go documentation: <https://go.dev/doc/effective_go>
