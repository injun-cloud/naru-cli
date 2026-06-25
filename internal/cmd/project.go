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
				output.Success("created project " + args[0])
				return nil
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
			Use: "rm <name>", Aliases: []string{"delete"}, Short: "Delete a project", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cl, err := newClient()
				if err != nil {
					return err
				}
				if err := cl.Delete(cmd.Context(), "/v1/projects/"+url.PathEscape(args[0]), nil); err != nil {
					return err
				}
				output.Success("deleted project " + args[0])
				return nil
			},
		},
		&cobra.Command{
			Use: "link <name>", Short: "Link this directory to a project (.naru)", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := config.SaveLink(args[0]); err != nil {
					return err
				}
				output.Success("linked to project " + args[0])
				return nil
			},
		},
	)
	return c
}
