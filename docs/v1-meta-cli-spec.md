# V1 Meta CLI Spec

This is the first outside-in slice for `plane-cli`. It deliberately excludes core Plane workflow commands.

## Scope

Build the meta UX that makes later agent-native commands trustworthy:

- binary identity
- configuration discovery
- PAT-based auth status
- environment diagnostics
- readable work item ID resolution

Supported platforms for V1: Unix only — Linux and macOS.

Binary name: `plane-cli`.

## Configuration

Use XDG-style config on Unix:

```text
~/.config/plane-cli/config.json
```

Environment variables override config file values:

```sh
PLANE_API_KEY=...
PLANE_WORKSPACE_SLUG=...
PLANE_BASE_URL=...
```

No default `PLANE_BASE_URL`. The user must provide it explicitly.

Precedence:

1. command flags
2. environment variables
3. config file
4. no implicit default

## Secrets

Never store the PAT/API key in the config file.

`PLANE_API_KEY` is read from the process environment or local `.env` during development only. `config set` must reject attempts to store API keys.

Safe config fields may include:

- `workspace_slug`
- `base_url`
- output preferences later, if needed

## V1 commands

### `plane-cli version`

Show version/build metadata.

Flags:

```sh
--format text|json
```

JSON should include at least:

- version
- commit, if available
- build date, if available
- supported schema versions

### `plane-cli config get`

Show effective non-secret config and where each value came from.

Must redact secrets and should not print `PLANE_API_KEY`.

Flags:

```sh
--format text|json
```

### `plane-cli config set <key> <value>`

Set safe config values only.

Allowed V1 keys:

- `workspace_slug`
- `base_url`

Rejected keys:

- `api_key`
- `pat`
- `token`
- anything secret-like

### `plane-cli auth status`

Check whether auth configuration exists and whether the token works.

Behavior:

- require `PLANE_API_KEY`
- require workspace slug
- require base URL
- call Plane's `GET /api/v1/users/me/`
- return typed errors for missing/invalid config

Flags:

```sh
--format text|json
```

### `plane-cli doctor`

Run environment diagnostics for agents and humans.

Checks:

- binary version
- supported OS
- config file presence
- base URL configured
- workspace slug configured
- API key present without revealing it
- Plane API reachable
- `/api/v1/users/me/` succeeds

Flags:

```sh
--for-agent
--format text|json
```

In JSON mode, diagnostics must be machine-readable and stable.

### `plane-cli resolve <reference>`

Resolve a readable work item reference like `ENG-42` into Plane UUIDs.

Behavior:

- parse project identifier and sequence number
- call Plane's readable identifier lookup
- return project/work item UUIDs and readable identifiers
- on ambiguity or malformed input, return typed errors

Flags:

```sh
--format text|json
--no-cache
```

Caching can be stubbed or omitted initially, but command design should leave room for it.

## Output rules

- stdout is data
- stderr is diagnostics/progress
- no spinners in JSON mode
- JSON uses stable schema strings
- errors use stable error codes
- text output can be friendly, but JSON must not require prose scraping

Success envelope shape:

```json
{
  "ok": true,
  "schema": "plane.<command>.v1",
  "data": {},
  "warnings": [],
  "hints": []
}
```

Error envelope shape:

```json
{
  "ok": false,
  "schema": "plane.error.v1",
  "error": {
    "code": "MISSING_BASE_URL",
    "message": "PLANE_BASE_URL is not configured.",
    "fix": "Set PLANE_BASE_URL or run: plane-cli config set base_url <url>",
    "retryable": true,
    "examples": [
      "PLANE_BASE_URL=https://api.plane.so plane-cli auth status --format json"
    ]
  }
}
```

## Error codes for V1

Initial stable codes:

- `MISSING_API_KEY`
- `MISSING_WORKSPACE_SLUG`
- `MISSING_BASE_URL`
- `INVALID_API_KEY`
- `WORKSPACE_NOT_FOUND`
- `INSUFFICIENT_PERMISSIONS`
- `API_UNREACHABLE`
- `INVALID_REFERENCE`
- `WORK_ITEM_NOT_FOUND`
- `CONFIG_WRITE_REJECTED_SECRET`
- `UNSUPPORTED_FORMAT`

## Acceptance criteria

- `go test ./...` passes.
- `plane-cli version --format json` emits valid JSON.
- `plane-cli config get --format json` never leaks the API key.
- `plane-cli config set api_key ...` fails with `CONFIG_WRITE_REJECTED_SECRET`.
- `plane-cli auth status --format json` reports typed missing-config errors without network calls when config is incomplete.
- `plane-cli doctor --for-agent --format json` gives agents enough information to decide whether they can act.
- `plane-cli resolve ENG-42 --format json` has a tested fake-server path before touching a real Plane workspace.
