# plane-cli

A small Go CLI for Plane.so project management integration.

The goal is a trusted Plane.so workflow tool with a fat agent skill and a thin CLI, because Plane still lacks a good public CLI.

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

Configuration is PAT-first via `PLANE_API_KEY`, `PLANE_WORKSPACE_SLUG`, and `PLANE_BASE_URL`. The CLI never stores API keys.
