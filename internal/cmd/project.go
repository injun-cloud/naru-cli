package cmd

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/injun-cloud/naru-cli/internal/config"
	"github.com/injun-cloud/naru-cli/internal/output"
	"github.com/injun-cloud/naru-server/pkg/apitypes"
)

func newProjectCmd() *cobra.Command {
	c := &cobra.Command{Use: "project", Aliases: []string{"proj", "p"}, Short: "Manage projects"}
	c.AddCommand(
		&cobra.Command{
			Use: "ls", Aliases: []string{"list"}, Short: "List projects",
			RunE: func(cmd *cobra.Command, args []string) error {
				cl, err := newClient()
				if err != nil {
					return err
				}
				var out []apitypes.ProjectSummary
				if err := cl.Get(cmd.Context(), "/v1/projects", &out); err != nil {
					return err
				}
				return printer().Emit(out, func() {
					rows := make([][]string, 0, len(out))
					for _, p := range out {
						rows = append(rows, []string{p.Name, strconv.Itoa(p.AppCount), strconv.Itoa(p.AddonCount)})
					}
					output.Table([]string{"NAME", "APPS", "ADDONS"}, rows)
				})
			},
		},
		&cobra.Command{
			Use: "create <name>", Short: "Create a project", Args: cobra.ExactArgs(1),
			Example: "  naru project create myproj",
			RunE: func(cmd *cobra.Command, args []string) error {
				cl, err := newClient()
				if err != nil {
					return err
				}
				var out apitypes.ProjectSummary
				if err := cl.Post(cmd.Context(), "/v1/projects", apitypes.ProjectCreateRequest{Name: args[0]}, &out); err != nil {
					return err
				}
				return printer().Emit(out, func() {
					output.Success("created project " + args[0])
				})
			},
		},
		&cobra.Command{
			Use: "get <name>", Short: "Show a project", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cl, err := newClient()
				if err != nil {
					return err
				}
				var p apitypes.Project
				if err := cl.Get(cmd.Context(), "/v1/projects/"+url.PathEscape(args[0]), &p); err != nil {
					return err
				}
				return printer().Emit(p, func() {
					fmt.Printf("project: %s\n", p.Name)
					fmt.Printf("apps:    %d\n", len(p.Applications))
					fmt.Printf("addons:  %d\n", len(p.Addons))
				})
			},
		},
		&cobra.Command{
			Use: "rm <name>", Aliases: []string{"delete", "remove"}, Short: "Delete a project and everything in it — apps, addons, all data (irreversible)", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cl, err := newClient()
				if err != nil {
					return err
				}
				if err := confirmDestroy("project", args[0], true); err != nil {
					return err
				}
				if err := cl.Delete(cmd.Context(), "/v1/projects/"+url.PathEscape(args[0]), nil); err != nil {
					return err
				}
				return printer().Emit(map[string]string{"status": "deleted", "name": args[0]}, func() {
					output.Success("deleted project " + args[0])
				})
			},
		},
		&cobra.Command{
			Use: "link <name>", Short: "Link this directory to a project (.naru)", Args: cobra.ExactArgs(1),
			Example: "  naru project link myproj   # subsequent commands use it without -p",
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := config.SaveLink(args[0]); err != nil {
					return err
				}
				return printer().Emit(map[string]string{"status": "linked", "name": args[0]}, func() {
					output.Success("linked to project " + args[0])
				})
			},
		},
		&cobra.Command{
			Use: "current", Aliases: []string{"cur"}, Short: "Show the resolved project and its source", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				project, source := resolveProjectSource()
				return printer().Emit(map[string]string{"project": project, "source": source}, func() {
					if project == "" {
						output.Info("no project set — use --project, $NARU_PROJECT, or run: naru project link")
						return
					}
					fmt.Printf("project: %s\nsource:  %s\n", project, source)
				})
			},
		},
		&cobra.Command{
			Use: "unlink", Short: "Remove this directory's project link (.naru)", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				removed, err := config.RemoveLink()
				if err != nil {
					return err
				}
				status := "unlinked"
				if !removed {
					status = "noop"
				}
				return printer().Emit(map[string]any{"status": status, "removed": removed}, func() {
					if removed {
						output.Success("unlinked this directory")
					} else {
						output.Info("no .naru link in this directory")
					}
				})
			},
		},
	)
	return c
}
