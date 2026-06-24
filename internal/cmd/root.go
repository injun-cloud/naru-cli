// Package cmd implements the naru CLI command tree.
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/injun-cloud/naru-cli/internal/client"
	"github.com/injun-cloud/naru-cli/internal/config"
	"github.com/injun-cloud/naru-cli/internal/output"
	"github.com/injun-cloud/naru-server/pkg/apitypes"
)

var (
	flagProject string
	flagServer  string
	flagToken   string
	flagJSON    bool
	flagJQ      string
	flagFields  []string
	flagNoInput bool

	version = "dev"
)

// Execute runs the root command.
func Execute(v string) {
	version = v
	root := newRoot()
	if err := root.Execute(); err != nil {
		output.Errf("%v", err)
		os.Exit(exitCode(err))
	}
}

// exitCode maps an error to a process exit code so a calling agent can branch:
// 0 success, 2 retryable (conflict / rate-limit / server-side), 1 everything else.
func exitCode(err error) int {
	var ae *client.APIError
	if errors.As(err, &ae) {
		switch {
		case ae.Status == http.StatusUnauthorized || ae.Status == http.StatusForbidden:
			return 3 // auth — run `naru login` (or check access)
		case ae.Status == http.StatusConflict || ae.Status == http.StatusTooManyRequests || ae.Status >= 500:
			return 2 // retryable
		}
	}
	return 1
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "naru",
		Short: "Naru platform CLI",
		Long: `Naru platform CLI — manage projects, apps, addons, env, and deploys.

Commands are noun then verb, e.g. "naru app create", "naru addon apply",
"naru project ls". Run "naru <noun> --help" to list a resource's verbs.

Output: human tables by default; pass --json or --jq '<expr>' for machine output,
and "get -o yaml" for an editable spec. Data goes to stdout, status/errors to stderr.
Exit codes: 0 ok, 2 retryable (conflict/rate-limit/server), 3 auth (run login), 1 other.

Apps and addons are declarative: "get -o yaml" to read a spec, change it, then
"apply -f" (or "edit"). Run "naru schema" for the project-spec field reference.`,
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
	pf.StringSliceVar(&flagFields, "fields", nil, "JSON output with only these fields, e.g. --fields name,status")
	pf.BoolVar(&flagNoInput, "no-input", false, "never prompt or open an editor (for CI/agents)")

	root.AddCommand(
		newLoginCmd(), newLogoutCmd(), newWhoamiCmd(), newSchemaCmd(),
		newProjectCmd(), newMemberCmd(), newAppCmd(), newAddonCmd(), newSecretCmd(), newTunnelCmd(),
		newMCPCmd(),
	)
	return root
}

// newSchemaCmd prints the project-spec JSON schema — the field reference an agent
// needs to build `app/addon apply` specs. The endpoint is public (token optional).
func newSchemaCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "schema",
		Short:   "Print the project-spec JSON schema (field reference for apply/edit)",
		Example: "  naru schema\n  naru schema | jq '.properties.applications.items.properties'",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := config.Resolve()
			if err != nil {
				return err
			}
			url := g.ServerURL
			if flagServer != "" {
				url = flagServer
			}
			var out apitypes.SchemaResponse
			if err := client.New(url, g.Token).Get(cmd.Context(), "/v1/schema", &out); err != nil {
				return err
			}
			b, err := json.MarshalIndent(out.JSONSchema, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		},
	}
}

// printer builds the output printer from global flags.
func printer() *output.Printer {
	return &output.Printer{JSON: flagJSON, JQ: flagJQ, Fields: flagFields}
}

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
