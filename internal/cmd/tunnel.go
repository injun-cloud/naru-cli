package cmd

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/injun-cloud/naru-cli/internal/output"
	"github.com/injun-cloud/naru-server/pkg/apitypes"
)

func newTunnelCmd() *cobra.Command {
	c := &cobra.Command{Use: "tunnel", Short: "Tunnel discovery (use `app tunnel` / `addon tunnel` to connect)"}
	c.AddCommand(&cobra.Command{
		Use: "ls", Aliases: []string{"list"}, Short: "List tunnelable endpoints (app ports + addons)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var ep apitypes.EndpointsResponse
			if err := cl.Get(cmd.Context(), "/v1/projects/"+project+"/endpoints", &ep); err != nil {
				return err
			}
			return printer().Emit(ep, func() {
				rows := [][]string{}
				for _, a := range ep.Apps {
					for _, p := range a.Ports {
						rows = append(rows, []string{"app", a.Name, strconv.Itoa(p.Port), strings.Join(p.Routes, ",")})
					}
				}
				for _, ad := range ep.Addons {
					rows = append(rows, []string{"addon", ad.Name, strconv.Itoa(ad.Port), ad.Type})
				}
				output.Table([]string{"KIND", "NAME", "PORT", "INFO"}, rows)
			})
		},
	})
	return c
}
