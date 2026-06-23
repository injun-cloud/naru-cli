// Command naru is the CLI + MCP server for the Naru platform.
package main

import "github.com/injun-cloud/naru-cli/internal/cmd"

// Set via -ldflags at build time.
var version = "dev"

func main() {
	cmd.Execute(version)
}
