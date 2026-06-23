package cmd

import (
	"github.com/spf13/cobra"

	"github.com/injun-cloud/naru-cli/internal/mcp"
)

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run the Naru MCP server (stdio) for AI agents",
		Long:  "Starts a Model Context Protocol server over stdio, exposing Naru operations as tools.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcp.Serve(version)
		},
	}
}
