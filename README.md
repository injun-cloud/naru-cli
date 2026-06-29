# naru

CLI + MCP client for the [Naru](https://naru.injunweb.com) platform — manage
projects, apps, addons, secrets, and deploys from your terminal or an AI agent.

## Install

```sh
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/injun-cloud/naru-cli/main/install.sh | sh

# any platform with Go
go install github.com/injun-cloud/naru-cli/cmd/naru@latest
```

Windows: grab the `.zip` from the [latest release](https://github.com/injun-cloud/naru-cli/releases/latest).

Update anytime with `naru upgrade`.

## Quickstart

```sh
naru login                  # sign in with GitHub
naru project ls             # your projects
naru app ls -p myproj       # apps in a project
naru app logs api -p myproj -f
naru --help                 # everything else
```

Add `--json` to any command for machine-readable output.

## AI agents (MCP)

Point your agent at `naru mcp`:

```json
{ "command": "naru", "args": ["mcp"] }
```
