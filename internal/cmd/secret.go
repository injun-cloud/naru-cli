package cmd

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/injun-cloud/naru-cli/internal/apitypes"
	"github.com/injun-cloud/naru-cli/internal/output"
)

func secretPath(project, app string) string {
	return fmt.Sprintf("/v1/projects/%s/apps/%s/secrets", url.PathEscape(project), url.PathEscape(app))
}

func newSecretCmd() *cobra.Command {
	c := &cobra.Command{Use: "secret", Aliases: []string{"env"}, Short: "Manage app secrets (environment)"}
	c.AddCommand(secretLsCmd(), secretGetCmd(), secretSetCmd(), secretRmCmd(), secretLoadCmd())
	return c
}

func secretGetCmd() *cobra.Command {
	return &cobra.Command{
		Use: "get <app> [KEY...]", Short: "Show secret values (all, or the given keys)", Args: cobra.MinimumNArgs(1),
		Example: "  naru secret get api -p myproj\n  naru secret get api DATABASE_URL -p myproj",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var sv apitypes.SecretVars
			if err := cl.Get(cmd.Context(), secretPath(project, args[0])+"?values=true", &sv); err != nil {
				return err
			}
			out := sv.Vars
			if out == nil {
				out = map[string]string{}
			}
			if len(args) > 1 { // filter to the requested keys
				sel := map[string]string{}
				for _, k := range args[1:] {
					if v, ok := out[k]; ok {
						sel[k] = v
					}
				}
				out = sel
			}
			return printer().Emit(out, func() {
				keys := make([]string, 0, len(out))
				for k := range out {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					fmt.Printf("%s=%s\n", k, out[k])
				}
			})
		},
	}
}

// mergeSecrets PATCHes the given vars onto an app's secret (create-or-merge).
func mergeSecrets(cmd *cobra.Command, app string, vars map[string]string) error {
	cl, project, err := clientAndProject()
	if err != nil {
		return err
	}
	for k := range vars {
		if err := apitypes.ValidSecretKey(k); err != nil {
			return err
		}
	}
	if err := cl.Patch(cmd.Context(), secretPath(project, app), apitypes.SecretVars{Vars: vars}, nil); err != nil {
		return err
	}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	return printer().Emit(map[string]any{"status": "set", "app": app, "keys": keys}, func() {
		output.Success(fmt.Sprintf("set %d secret(s) on %s", len(vars), app))
	})
}

func secretLsCmd() *cobra.Command {
	return &cobra.Command{
		Use: "ls <app>", Aliases: []string{"list"}, Short: "List secret keys (values never shown)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var keys apitypes.SecretKeys
			if err := cl.Get(cmd.Context(), secretPath(project, args[0]), &keys); err != nil {
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

func secretSetCmd() *cobra.Command {
	var file string
	c := &cobra.Command{
		Use: "set <app> [KEY=VALUE...]", Short: "Set secrets (merge), from args and/or a dotenv file", Args: cobra.MinimumNArgs(1),
		Example: "  naru secret set api DATABASE_URL=postgres://... LOG_LEVEL=info -p myproj\n" +
			"  naru secret set api -f .env",
		RunE: func(cmd *cobra.Command, args []string) error {
			vars := map[string]string{}
			if file != "" {
				loaded, err := parseDotenv(file)
				if err != nil {
					return err
				}
				vars = loaded
			}
			// Explicit KEY=VALUE args override file entries.
			kv, err := parseKV(args[1:])
			if err != nil {
				return err
			}
			for k, v := range kv {
				vars[k] = v
			}
			if len(vars) == 0 {
				return fmt.Errorf("nothing to set: pass KEY=VALUE args or -f <dotenv file>")
			}
			return mergeSecrets(cmd, args[0], vars)
		},
	}
	c.Flags().StringVarP(&file, "file", "f", "", "dotenv file to merge (e.g. .env, - for stdin)")
	return c
}

func secretRmCmd() *cobra.Command {
	return &cobra.Command{
		Use: "rm <app> KEY...", Aliases: []string{"unset"}, Short: "Delete secrets", Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			for _, key := range args[1:] {
				if err := cl.Delete(cmd.Context(), secretPath(project, args[0])+"/"+url.PathEscape(key), nil); err != nil {
					return err
				}
			}
			return printer().Emit(map[string]any{"status": "deleted", "app": args[0], "keys": args[1:]}, func() {
				output.Success(fmt.Sprintf("deleted %d secret(s) on %s: %s", len(args[1:]), args[0], strings.Join(args[1:], ", ")))
			})
		},
	}
}

// secretLoadCmd is a hidden back-compat alias for "secret set -f".
func secretLoadCmd() *cobra.Command {
	var file string
	c := &cobra.Command{
		Use: "load <app>", Short: "Deprecated: use `secret set <app> -f <file>`", Hidden: true, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			vars, err := parseDotenv(file)
			if err != nil {
				return err
			}
			if len(vars) == 0 {
				return fmt.Errorf("no vars found in %s", file)
			}
			return mergeSecrets(cmd, args[0], vars)
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
	var r io.Reader = os.Stdin
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		if key == "" || strings.ContainsAny(key, " \t") {
			continue // skip malformed keys rather than send a garbage secret name
		}
		out[key] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return out, sc.Err()
}
