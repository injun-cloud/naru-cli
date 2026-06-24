package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/injun-cloud/naru-cli/internal/output"
	"github.com/injun-cloud/naru-server/pkg/apitypes"
)

func addonPath(project, addon string) string {
	return fmt.Sprintf("/v1/projects/%s/addons/%s", project, addon)
}

func newAddonCmd() *cobra.Command {
	c := &cobra.Command{Use: "addon", Short: "Manage addons (databases/caches)"}
	c.AddCommand(addonListCmd(), addonCreateCmd(), addonSetCmd(), addonGetCmd(), addonRmCmd(), addonConnCmd(), addonTunnelCmd())
	return c
}

func addonListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "ls", Aliases: []string{"list"}, Short: "List addons",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var addons []apitypes.AddonSpec
			if err := cl.Get(cmd.Context(), "/v1/projects/"+project+"/addons", &addons); err != nil {
				return err
			}
			return printer().Emit(addons, func() {
				rows := make([][]string, 0, len(addons))
				for _, a := range addons {
					rows = append(rows, []string{a.Name, a.Type, a.Version, strconv.Itoa(a.Port), a.Size})
				}
				output.Table([]string{"NAME", "TYPE", "VERSION", "PORT", "SIZE"}, rows)
			})
		},
	}
}

func addonCreateCmd() *cobra.Command {
	var typ, version, size, file string
	var cpuReq, memReq, cpuLim, memLim string
	var port int
	c := &cobra.Command{
		Use: "create <name>", Short: "Create an addon", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var req apitypes.AddonCreateRequest
			if file != "" {
				if err := loadSpecFile(file, &req); err != nil {
					return err
				}
			}
			req.Name = args[0]
			if typ != "" {
				req.Type = typ
			}
			if version != "" {
				req.Version = version
			}
			if size != "" {
				req.Size = size
			}
			if req.Size == "" {
				req.Size = "1Gi" // preserve the historic default
			}
			if port > 0 {
				req.Port = &port
			}
			if rs := resourceFromFlags(cpuReq, memReq, cpuLim, memLim); rs != nil {
				req.Resources = rs
			}
			var out apitypes.AddonSpec
			if err := cl.Post(cmd.Context(), "/v1/projects/"+project+"/addons", req, &out); err != nil {
				return err
			}
			output.Success("created addon " + args[0])
			return nil
		},
	}
	c.Flags().StringVarP(&file, "file", "f", "", "spec file (YAML/JSON) for full-schema fields")
	c.Flags().StringVar(&typ, "type", "", "mysql|postgres|mongo|redis (required unless -f)")
	c.Flags().StringVar(&version, "version", "", "image version (required unless -f)")
	c.Flags().StringVar(&size, "size", "", "storage size (default: 1Gi)")
	c.Flags().IntVar(&port, "port", 0, "port (default: type default)")
	addResourceFlags(c, &cpuReq, &memReq, &cpuLim, &memLim)
	return c
}

func addonSetCmd() *cobra.Command {
	var version, size, file string
	var cpuReq, memReq, cpuLim, memLim string
	var port int
	c := &cobra.Command{
		Use: "set <name>", Short: "Update an addon (version/size/port/resources; type is immutable)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var req apitypes.AddonCreateRequest
			if file != "" {
				if err := loadSpecFile(file, &req); err != nil {
					return err
				}
			}
			if version != "" {
				req.Version = version
			}
			if size != "" {
				req.Size = size
			}
			if port > 0 {
				req.Port = &port
			}
			if rs := resourceFromFlags(cpuReq, memReq, cpuLim, memLim); rs != nil {
				req.Resources = rs
			}
			var out apitypes.AddonSpec
			if err := cl.Patch(cmd.Context(), addonPath(project, args[0]), req, &out); err != nil {
				return err
			}
			output.Success("updated addon " + args[0])
			return nil
		},
	}
	c.Flags().StringVarP(&file, "file", "f", "", "spec file (YAML/JSON) for full-schema fields")
	c.Flags().StringVar(&version, "version", "", "image version")
	c.Flags().StringVar(&size, "size", "", "storage size")
	c.Flags().IntVar(&port, "port", 0, "port")
	addResourceFlags(c, &cpuReq, &memReq, &cpuLim, &memLim)
	return c
}

func addonGetCmd() *cobra.Command {
	return &cobra.Command{
		Use: "get <name>", Short: "Show an addon", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var a apitypes.AddonSpec
			if err := cl.Get(cmd.Context(), addonPath(project, args[0]), &a); err != nil {
				return err
			}
			return printer().Emit(a, func() {
				fmt.Printf("%s  type=%s version=%s port=%d size=%s\n", a.Name, a.Type, a.Version, a.Port, a.Size)
			})
		},
	}
}

func addonRmCmd() *cobra.Command {
	return &cobra.Command{
		Use: "rm <name>", Aliases: []string{"delete"}, Short: "Delete an addon", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			if err := cl.Delete(cmd.Context(), addonPath(project, args[0]), nil); err != nil {
				return err
			}
			output.Success("deleted addon " + args[0])
			return nil
		},
	}
}

func addonConnCmd() *cobra.Command {
	return &cobra.Command{
		Use: "conn <name>", Short: "Show connection info (no password)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var dto apitypes.ConnectionDTO
			if err := cl.Get(cmd.Context(), addonPath(project, args[0])+"/connection", &dto); err != nil {
				return err
			}
			return printer().Emit(dto, func() {
				fmt.Printf("type:      %s\nhost:      %s\nport:      %d\n", dto.Type, dto.Host, dto.Port)
				if dto.Database != "" {
					fmt.Printf("database:  %s\nusername:  %s\n", dto.Database, dto.Username)
				}
				fmt.Printf("secretRef: %s (password key: PASSWORD)\nenvPrefix: %s\n", dto.SecretRef, dto.EnvPrefix)
			})
		},
	}
}

func addonTunnelCmd() *cobra.Command {
	var localPort int
	c := &cobra.Command{
		Use: "tunnel <name>", Short: "Tunnel a local port to an addon", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			// Resolve the addon port for the default local port.
			var a apitypes.AddonSpec
			if err := cl.Get(cmd.Context(), addonPath(project, args[0]), &a); err != nil {
				return err
			}
			if localPort == 0 {
				localPort = a.Port
			}
			return runTunnel(cmd, cl, addonPath(project, args[0])+"/tunnel", localPort)
		},
	}
	c.Flags().IntVar(&localPort, "local-port", 0, "local port (default: addon port)")
	return c
}
