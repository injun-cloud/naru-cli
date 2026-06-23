// Package cmd implements the naru CLI command tree.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/injun-cloud/naru-cli/internal/client"
	"github.com/injun-cloud/naru-cli/internal/config"
	"github.com/injun-cloud/naru-cli/internal/output"
)

var (
	flagProject string
	flagServer  string
	flagToken   string
	flagJSON    bool
	flagJQ      string

	version = "dev"
)

// Execute runs the root command.
func Execute(v string) {
	version = v
	root := newRoot()
	if err := root.Execute(); err != nil {
		output.Errf("%v", err)
		os.Exit(1)
	}
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "naru",
		Short:         "Naru platform CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}
	pf := root.PersistentFlags()
	pf.StringVarP(&flagProject, "project", "p", "", "project (overrides $NARU_PROJECT and .naru)")
	pf.StringVar(&flagServer, "server", "", "server URL (overrides config)")
	pf.StringVar(&flagToken, "token", "", "bearer token (overrides config/$NARU_TOKEN)")
	pf.BoolVar(&flagJSON, "json", false, "output JSON")
	pf.StringVar(&flagJQ, "jq", "", "filter JSON output with a jq expression")

	root.AddCommand(
		newLoginCmd(), newLogoutCmd(), newWhoamiCmd(),
		newProjectCmd(), newAppCmd(), newAddonCmd(), newEnvCmd(), newTunnelCmd(),
		newMCPCmd(),
	)
	return root
}

// printer builds the output printer from global flags.
func printer() *output.Printer { return &output.Printer{JSON: flagJSON, JQ: flagJQ} }

// newClient builds an authenticated client from config + flags.
func newClient() (*client.Client, error) {
	g, err := config.Resolve()
	if err != nil {
		return nil, err
	}
	if flagServer != "" {
		g.ServerURL = flagServer
	}
	token := g.Token
	if flagToken != "" {
		token = flagToken
	}
	if token == "" {
		return nil, fmt.Errorf("not logged in — run: naru login")
	}
	return client.New(g.ServerURL, token), nil
}

// clientAndProject builds an authenticated client and resolves the project.
func clientAndProject() (*client.Client, string, error) {
	cl, err := newClient()
	if err != nil {
		return nil, "", err
	}
	project, err := resolveProject()
	if err != nil {
		return nil, "", err
	}
	return cl, project, nil
}

// resolveProject returns the project from --project, $NARU_PROJECT, or .naru.
func resolveProject() (string, error) {
	if flagProject != "" {
		return flagProject, nil
	}
	if v := os.Getenv("NARU_PROJECT"); v != "" {
		return v, nil
	}
	if p := config.LinkedProject(); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("no project set — use --project, $NARU_PROJECT, or run: naru project link")
}
