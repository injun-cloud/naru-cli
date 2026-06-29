# naru

The CLI and MCP client for the [Naru](https://naru.injunweb.com) platform — manage
projects, apps, addons, secrets, deploys, and tunnels from your terminal or an AI agent.

## Install

**Script (macOS / Linux):**

```sh
curl -fsSL https://raw.githubusercontent.com/injun-cloud/naru-cli/main/install.sh | sh
```

**Go (any platform with Go ≥ 1.26):**

```sh
go install github.com/injun-cloud/naru-cli/cmd/naru@latest
```

**Windows / manual:** download the archive for your OS/arch from the
[latest release](https://github.com/injun-cloud/naru-cli/releases/latest)
(`naru_windows_amd64.zip`, etc.) and put `naru` on your `PATH` — or use `go install` above.

## Upgrade

```sh
naru upgrade        # download the latest release for this OS/arch and replace the binary in place
naru version        # show the installed version
```

`naru upgrade` verifies the release checksum before swapping the binary, and is a
no-op when already on the latest version. (Re-running the install script or
`go install ...@latest` works too.)

## Usage

```sh
naru login                       # authenticate (GitHub OAuth)
naru project ls                  # your projects
naru app ls -p myproj            # apps in a project
naru app logs api -p myproj -f   # follow logs
naru schema                      # project-spec field reference
naru --help                      # all commands
```

Output is human tables by default; pass `--json` (or `--jq '<expr>'`) for machine
output. Apps and addons are declarative — `get -o yaml` a spec, edit it, then
`apply -f`.

### MCP server

`naru mcp` exposes the platform to AI agents over stdio. Point your agent at:

```json
{ "command": "naru", "args": ["mcp"] }
```

## Release

Tag-driven via [GoReleaser](https://goreleaser.com): push a `vX.Y.Z` tag and CI
builds cross-platform binaries and publishes a GitHub Release.
