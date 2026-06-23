package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/injun-cloud/naru-cli/internal/output"
	"github.com/injun-cloud/naru-server/pkg/apitypes"
)

func envPath(project, app string) string {
	return fmt.Sprintf("/v1/projects/%s/apps/%s/env", project, app)
}

func newEnvCmd() *cobra.Command {
	c := &cobra.Command{Use: "env", Short: "Manage app environment variables"}
	c.AddCommand(envLsCmd(), envSetCmd(), envRmCmd(), envLoadCmd())
	return c
}

func envLsCmd() *cobra.Command {
	return &cobra.Command{
		Use: "ls <app>", Aliases: []string{"list"}, Short: "List env keys (values never shown)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var keys apitypes.EnvKeys
			if err := cl.Get(cmd.Context(), envPath(project, args[0]), &keys); err != nil {
				return err
			}
			return printer().Emit(keys, func() {
				for _, k := range keys.Keys {
					fmt.Println(k)
				}
			})
		},
	}
}

func envSetCmd() *cobra.Command {
	return &cobra.Command{
		Use: "set <app> KEY=VALUE...", Short: "Set env vars (merge)", Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			vars, err := parseKV(args[1:])
			if err != nil {
				return err
			}
			if err := cl.Patch(cmd.Context(), envPath(project, args[0]), apitypes.EnvVars{Vars: vars}, nil); err != nil {
				return err
			}
			output.Success(fmt.Sprintf("set %d env var(s) on %s", len(vars), args[0]))
			return nil
		},
	}
}

func envRmCmd() *cobra.Command {
	return &cobra.Command{
		Use: "rm <app> KEY...", Aliases: []string{"unset"}, Short: "Delete env vars", Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			for _, key := range args[1:] {
				if err := cl.Delete(cmd.Context(), envPath(project, args[0])+"/"+key, nil); err != nil {
					return err
				}
			}
			output.Success("deleted env var(s)")
			return nil
		},
	}
}

func envLoadCmd() *cobra.Command {
	var file string
	c := &cobra.Command{
		Use: "load <app>", Short: "Load env vars from a .env file (merge)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			vars, err := parseDotenv(file)
			if err != nil {
				return err
			}
			if len(vars) == 0 {
				return fmt.Errorf("no vars found in %s", file)
			}
			if err := cl.Patch(cmd.Context(), envPath(project, args[0]), apitypes.EnvVars{Vars: vars}, nil); err != nil {
				return err
			}
			output.Success(fmt.Sprintf("loaded %d env var(s) onto %s", len(vars), args[0]))
			return nil
		},
	}
	c.Flags().StringVar(&file, "file", ".env", "dotenv file")
	return c
}

func parseKV(pairs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return nil, fmt.Errorf("invalid KEY=VALUE: %q", p)
		}
		out[k] = v
	}
	return out, nil
}

func parseDotenv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return out, sc.Err()
}
