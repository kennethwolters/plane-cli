# plane-cli

A small Go CLI for Plane.so project management integration.

The goal is a trusted Plane.so workflow tool with a fat agent skill and a thin CLI, because Plane still lacks a good public CLI.

## Install

Download a release asset from GitHub Releases for your OS/architecture, verify it with `checksums.txt`, and put `plane-cli` on your `PATH`.

```sh
curl -LO https://github.com/kennethwolters/plane-cli/releases/download/v0.4.0/plane-cli_0.4.0_linux_amd64.tar.gz
curl -LO https://github.com/kennethwolters/plane-cli/releases/download/v0.4.0/checksums.txt
sha256sum --check --ignore-missing checksums.txt
tar -xzf plane-cli_0.4.0_linux_amd64.tar.gz
sudo install -m 0755 plane-cli_0.4.0_linux_amd64/plane-cli /usr/local/bin/plane-cli
```

Supported release targets:

- Linux amd64
- Linux arm64
- macOS amd64
- macOS arm64

Windows is out of scope for V1.

## Configuration

Configuration is PAT-first via `PLANE_API_KEY`, `PLANE_WORKSPACE_SLUG`, and `PLANE_BASE_URL`. The CLI never stores API keys.

`PLANE_BASE_URL` must be explicit and include a URL scheme, for example `https://app.plane.so` or your self-hosted Plane URL.

Config precedence is: process environment, explicit env file (`--env-file <path>` or `PLANE_CLI_ENV_FILE`), cwd/nearest ancestor `.env`, then CLI config file.

## Development

```sh
mise install
go test ./...
```

## V1 meta commands

```sh
plane-cli version --format json
plane-cli config get --format json
plane-cli auth status --format json
plane-cli doctor --for-agent --format json
plane-cli resolve ENG-42 --format json
```

## Core work commands

```sh
plane-cli project list --format json
plane-cli work list --project ENG --format json
plane-cli work list --project ENG --fields readable_id,name,state_group,priority,description_excerpt --description-excerpt 240 --format json
plane-cli work get ENG-42 --format json
plane-cli work create --project ENG --title "Fix login" --format json
plane-cli work create --project ENG --title "Fix login" --description-file ./body.html --dedupe-query "Fix login" --apply --verify --format json
plane-cli work edit ENG-42 --append-description-html "<hr><p>Updated scope.</p>" --apply --verify --format json
plane-cli work edit ENG-42 --labels-add tracking --assignees-add alice@example.test --apply --verify --format json
plane-cli work move ENG-42 --state "Ready" --apply --verify --format json
plane-cli work comments ENG-42 --limit 5 --format json
plane-cli work batch comment --file ./comments.json --apply --verify --format json
plane-cli work relation add ENG-42 --blocks ENG-43 --apply --verify --format json
plane-cli work complete ENG-42 --evidence "Tests passed and PR merged" --apply --verify --format json
plane-cli search "login" --project ENG --include-comments --format json
```

Mutations dry-run by default. Use `--apply` to change Plane and `--verify` when the command should confirm the resulting state.

## Agent skills

Project-local skills are limited to repo-specific additions. Shared skills such as `plane`, `github`, `librarian`, `summarize`, and `web-browser` should come from the parent/global skill directory to avoid name conflicts.

## Release process

CI runs on PRs and `main` pushes. Releases are created from `v*` tags on commits contained in `main`; the release workflow builds Linux/macOS assets and publishes `checksums.txt` to GitHub Releases.

Real Plane verification is intentionally not part of GitHub Actions. Run it locally against an approved playground workspace before or after release as appropriate.

See `docs/main-spec.md` for the planned core command surface.
