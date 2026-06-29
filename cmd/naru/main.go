// Command naru is the CLI + MCP server for the Naru platform.
package main

import (
	"runtime/debug"

	"github.com/injun-cloud/naru-cli/internal/cmd"
)

// version is set via -ldflags in release builds; for `go install`ed binaries it
// falls back to the module version from the build info.
var version = "dev"

func main() {
	if version == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			version = bi.Main.Version
		}
	}
	cmd.Execute(version)
}
