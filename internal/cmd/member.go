package cmd

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/injun-cloud/naru-cli/internal/output"
	"github.com/injun-cloud/naru-server/pkg/apitypes"
)

func newMemberCmd() *cobra.Command {
	c := &cobra.Command{Use: "member", Aliases: []string{"members", "owner"}, Short: "Manage project owners"}
	c.AddCommand(
		&cobra.Command{
			Use: "ls", Aliases: []string{"list"}, Short: "List project owners",
			RunE: func(cmd *cobra.Command, args []string) error {
				cl, project, err := clientAndProject()
				if err != nil {
					return err
				}
				var out apitypes.MembersResponse
				if err := cl.Get(cmd.Context(), "/v1/projects/"+url.PathEscape(project)+"/members", &out); err != nil {
					return err
				}
				return printer().Emit(out, func() {
					rows := make([][]string, 0, len(out.Owners))
					for _, m := range out.Owners {
						rows = append(rows, []string{m.Username, strconv.FormatInt(m.GithubID, 10)})
					}
					output.Table([]string{"USERNAME", "GITHUB ID"}, rows)
				})
			},
		},
		&cobra.Command{
			Use: "add <username>", Short: "Add an owner by GitHub username", Args: cobra.ExactArgs(1),
			Example: "  naru member add octocat -p myproj",
			RunE: func(cmd *cobra.Command, args []string) error {
				cl, project, err := clientAndProject()
				if err != nil {
					return err
				}
				var out apitypes.MemberInfo
				if err := cl.Post(cmd.Context(), "/v1/projects/"+url.PathEscape(project)+"/members", apitypes.AddMemberRequest{Username: args[0]}, &out); err != nil {
					return err
				}
				output.Success("added owner " + out.Username + " to " + project)
				return nil
			},
		},
		&cobra.Command{
			Use: "rm <username>", Aliases: []string{"remove"}, Short: "Remove an owner by GitHub username", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cl, project, err := clientAndProject()
				if err != nil {
					return err
				}
				if err := cl.Delete(cmd.Context(), "/v1/projects/"+url.PathEscape(project)+"/members/"+url.PathEscape(args[0]), nil); err != nil {
					return err
				}
				output.Success("removed owner " + args[0] + " from " + project)
				return nil
			},
		},
	)
	return c
}
