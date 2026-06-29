<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/logo-dark.svg">
    <img alt="naru" src="docs/logo.svg" width="300">
  </picture>
</p>

<p align="center">
  CLI + MCP client for the <a href="https://naru.injunweb.com">Naru</a> platform —
  manage projects, apps, addons, secrets, and deploys from your terminal or an AI agent.
</p>

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
