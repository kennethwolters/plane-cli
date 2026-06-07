# Engineering Philosophy

How we build `plane-cli`. This is a bias document, not a rigid process. If a PR conflicts with this document, the conflict is the conversation.

Audience: humans and agents working on this repo.

## Thesis

Build a small, trustworthy Go CLI with a fat agent skill and a thin executable.

The CLI should be boring infrastructure: deterministic output, explicit errors, safe mutations, and strong tests. The skill can carry workflow judgment and prose. The binary should be the dependable substrate agents can call without guessing.

## Mechanical sympathy

Design with the real substrate in mind. For this project, the substrate is:

- **Plane API / MCP**: workspaces, projects, work items, cycles, modules, blockers, pagination, rate limits, auth, and API drift.
- **Agents**: limited context, no screen unless explicitly given screenshots, brittle interpretation of prose, high benefit from stable JSON and examples.
- **Terminals and shells**: pipes, exit codes, stdout/stderr separation, CI usage, non-interactive operation.
- **Go**: simple binaries, explicit errors, easy cross-compilation, strong standard library, no hidden runtime magic.
- **GitHub / CI**: public repo, reproducible builds, release artifacts, reviewable diffs.

Abstractions can hide these, but they cannot remove them. Prefer code that makes the substrate visible where it matters.

## Essential vs accidental complexity

Essential complexity:

- Modeling Plane's project-management domain correctly.
- Handling ambiguity, stale state, blockers, evidence, pagination, and rate limits.
- Making mutations reviewable and reversible.
- Producing outputs agents can parse reliably.
- Keeping auth and secrets safe.

Accidental complexity:

- Recreating a full Plane SDK before there is a caller.
- Building a framework before we have commands.
- Adding concurrency, caching, config layers, plugin systems, or MCP support prematurely.
- Clever abstractions around simple HTTP calls.
- Human-pretty output that machines have to scrape.

Three similar functions are better than one premature abstraction. Add structure after the second real use case, not before the first.

## Vendor seam discipline

Use native APIs where they are the product boundary; keep our consumer surface stable.

- Plane REST/MCP can change; our CLI output schemas should not churn casually.
- The agent skill can evolve quickly; the CLI should preserve command and JSON contracts.
- Keep low-level escape hatches under `plane raw`; keep agent-native workflows under semantic commands.
- Do not leak provider-specific weirdness into every call site. Localize it at the client boundary.

The seam we own is the CLI contract: command names, flags, exit codes, JSON envelopes, operation logs, and verification semantics.

## Outside-in TDD

Tests define wanted behavior before implementation.

Default development order:

1. Write or update a behavior spec: short prose, examples, or Gherkin-like scenarios.
2. Write acceptance/CLI tests: command, flags, stdout/stderr, exit code, JSON schema.
3. Write integration tests around HTTP client behavior, pagination, auth, and operation persistence.
4. Write unit tests for pure logic: resolving references, diff planning, validation, formatting, error mapping.
5. Run tests and confirm the new tests fail for the right reason.
6. Implement the smallest code that makes them pass.
7. Refactor with tests green.

A feature without tests is not done. A test that never failed may not test anything.

## Uncle Bob-shaped TDD bias for agents

Use agents as transformation workhorses, but keep each transformation reviewable:

```text
informal intent
  -> hardened task spec
  -> Gherkin / acceptance scenarios
  -> failing acceptance tests
  -> failing unit tests
  -> implementation
  -> refactor
  -> property tests / fuzz tests where useful
  -> mutation or adversarial tests for critical logic
  -> full suite
  -> human spot check
```

Do not skip the red phase. Do not let the coder agent invent acceptance criteria while coding. Keep the acceptance surface close to user-visible behavior.

Spend real time hardening tests. If agents make us 2x faster and 30-40% of that time goes into better tests, that is not waste; it is how we buy confidence and future refactorability.

## UI and agent verification

Agents cannot reliably know whether something works on screen just because tests pass. If we later build TUI output, browser docs, screenshots, or any visual surface, add UI-level checks.

UI tests are fragile because UI changes often. That is less fatal with agents: agents can maintain and rewrite fragile tests. Still:

- Keep most logic below the UI and test it there.
- Add UI tests for flows where screen behavior is the product.
- Prefer stable selectors, snapshots with intent, and small focused UI tests.
- Treat UI tests as verification, not as the only specification.

## Test pyramid, adapted for a CLI

- **Acceptance / CLI tests**: highest value. Run the binary as users and agents will run it. Assert exit code, stdout, stderr, and JSON.
- **Integration tests**: fake Plane server first; real Plane sandbox later. Cover auth, pagination, rate limits, retries, idempotency, and API error mapping.
- **Unit tests**: pure domain logic, schema validation, operation diff generation, reference resolution.
- **Property / fuzz tests**: parsers, reference resolution, pagination cursors, JSON envelope stability, operation-revert round trips.
- **Mutation/adversarial tests**: critical safety gates such as `prove-done`, `apply`, `raw`, and destructive operations.

For early Go work, start with `go test ./...`. Add a CLI acceptance harness before adding real Plane mutations.

## Done means verified

"Implemented" is not done. "Merged" is not done. "CI is green" is not done.

Done means there is evidence that the behavior works:

- CLI command: run it and capture expected output/exit code.
- JSON command: validate the envelope and schema version.
- Mutation: dry-run, apply, then verify.
- Error handling: exercise at least one failure path.
- Plane integration: test against a fake server first, then a real sandbox/workspace when available.
- Docs: examples match actual commands.

Default to anxious. If you cannot name a cheap falsifying test, you probably do not understand the change yet.

## Root-cause fixes

Do not bypass safety to make red go green.

No force-pushing shared work, no deleting unknown files, no skipping tests to land faster, no hiding API drift with broad catch-alls. If a bug belongs in a dependency or upstream API assumption, isolate it and fix the seam.

## Minimal viable engineering

Launch ugly, ship fast, iterate later — but not recklessly.

- No speculative frameworks.
- No generic plugin system until a plugin exists.
- No custom config language.
- No broad SDK until commands need it.
- No comments that restate code.
- No validation for impossible internal states; validate at boundaries.

The code should be easy to delete. The CLI contract should be hard to break.

## Go-shaped rules

- Prefer standard library first.
- Keep packages small and names plain.
- Return explicit errors; map them to stable CLI error codes at the boundary.
- Separate stdout for data from stderr for diagnostics.
- Use context-aware HTTP calls.
- Avoid globals except build metadata and immutable defaults.
- Keep JSON structs explicit and versioned.
- Favor table-driven tests.
- Let `gofmt` decide style.

## CLI contract rules

- Every command has deterministic exit behavior.
- Machine-readable output must not require scraping prose.
- JSON schemas are versioned: `plane.<thing>.v1`.
- Errors include stable `code`, human `message`, actionable `fix`, retryability, and examples where useful.
- Destructive or durable changes support dry-run unless impossible.
- Raw commands exist, but semantic commands are the happy path.
- Long-running commands should eventually support NDJSON events.

## Scope discipline

Prefer small PRs with one behavioral claim and tests proving it.

Good PR shape:

- One command or one slice of infrastructure.
- Tests first or in the same commit series.
- README/docs examples updated only if the behavior exists.
- No unrelated cleanup.

If the PR needs a long planning doc to make sense, it is probably too large or too vague.

## Dependency discipline

This is a Go project, so dependency pressure should be low.

- Start with stdlib.
- Add dependencies deliberately, with a clear reason.
- Prefer stable, boring libraries for CLI parsing and tests only when stdlib becomes painful.
- Avoid transitive dependency sprawl.
- Review `go.mod` and `go.sum` diffs as part of the change.

## Agent operating rules

Agents working here should:

- Read the relevant docs before designing a command.
- Prefer tests and executable examples over prose claims.
- Use `go test ./...` before reporting completion.
- For CLI behavior, run the command manually too.
- Keep the working tree clean.
- Never treat artifact creation as progress by itself.
- Surface uncertainty instead of smoothing it over.

## Review checklist

Ask these before merging:

- Did the new test fail before the implementation?
- Is the command output deterministic and agent-readable?
- Are stdout and stderr used correctly?
- Are errors typed and actionable?
- Does mutation require dry-run/apply/verify where appropriate?
- Is this essential complexity, or did we invent a framework?
- Can this be refactored safely later because tests are strong enough?

## Related

- [`design-philosophy.md`](./design-philosophy.md) — agent-native CLI design principles.
- Aedna service engineering philosophy, especially mechanical sympathy, outside-in TDD, vendor seams, dev/prod parity, and done-means-verified.
- Uncle Bob Martin's recent agent/TDD workflow notes: informal spec → Gherkin → acceptance tests → unit tests → code → refactor → property/mutation tests → human spot check.
