package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/injun-cloud/naru-cli/internal/client"
	"github.com/injun-cloud/naru-cli/internal/output"
	"github.com/injun-cloud/naru-server/pkg/apitypes"
)

func addonPath(project, addon string) string {
	return fmt.Sprintf("/v1/projects/%s/addons/%s", project, addon)
}

func newAddonCmd() *cobra.Command {
	c := &cobra.Command{Use: "addon", Short: "Manage addons (databases/caches)"}
	c.AddCommand(addonListCmd(), addonCreateCmd(), addonGetCmd(), addonEditCmd(), addonApplyCmd(), addonRmCmd(), addonConnCmd(), addonTunnelCmd())
	return c
}

// upsertAddon creates the addon if absent, otherwise replaces it. The addon type
// is immutable. Returns "created" or "updated".
func upsertAddon(cmd *cobra.Command, cl *client.Client, project string, spec apitypes.AddonSpec) (string, error) {
	if spec.Name == "" {
		return "", fmt.Errorf("spec is missing 'name'")
	}
	if spec.Type == "" || spec.Version == "" {
		return "", fmt.Errorf("spec is missing 'type'/'version'")
	}
	if spec.Size == "" {
		spec.Size = "1Gi"
	}
	req := apitypes.AddonCreateRequest{Name: spec.Name, Type: spec.Type, Version: spec.Version, Size: spec.Size, Resources: spec.Resources}
	if spec.Port > 0 {
		req.Port = &spec.Port
	}
	var out apitypes.AddonSpec
	err := cl.Get(cmd.Context(), addonPath(project, spec.Name), &apitypes.AddonSpec{})
	if err == nil {
		if err := cl.Put(cmd.Context(), addonPath(project, spec.Name), req, &out); err != nil {
			return "", err
		}
		return "updated", nil
	}
	if !client.NotFound(err) {
		return "", err
	}
	if err := cl.Post(cmd.Context(), "/v1/projects/"+project+"/addons", req, &out); err != nil {
		return "", err
	}
	return "created", nil
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
	var typ, version, size string
	var port int
	c := &cobra.Command{
		Use: "create <name>", Short: "Create an addon (minimal bootstrap)", Args: cobra.ExactArgs(1),
		Long: "Bootstrap a database/cache. For resources and other fields use\n" +
			"`naru addon edit` or `naru addon apply -f`.",
		Example: "  naru addon create db --type postgres --version 16 -p myproj\n" +
			"  naru addon create cache --type redis --version 7 --size 2Gi -p myproj",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			spec := apitypes.AddonSpec{Name: args[0], Type: typ, Version: version, Size: size, Port: port}
			action, err := upsertAddon(cmd, cl, project, spec)
			if err != nil {
				return err
			}
			output.Success(action + " addon " + args[0])
			return nil
		},
	}
	c.Flags().StringVar(&typ, "type", "", "mysql|postgres|mongo|redis (required)")
	c.Flags().StringVar(&version, "version", "", "image version (required)")
	c.Flags().StringVar(&size, "size", "", "storage size (default: 1Gi)")
	c.Flags().IntVar(&port, "port", 0, "port (default: type default)")
	_ = c.MarkFlagRequired("type")
	_ = c.MarkFlagRequired("version")
	return c
}

func addonGetCmd() *cobra.Command {
	var outFmt string
	c := &cobra.Command{
		Use: "get <name>", Short: "Show an addon (-o yaml for the editable spec)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var a apitypes.AddonSpec
			if err := cl.Get(cmd.Context(), addonPath(project, args[0]), &a); err != nil {
				return err
			}
			if outFmt == "yaml" {
				b, err := marshalSpecYAML(a)
				if err != nil {
					return err
				}
				fmt.Print(string(b))
				return nil
			}
			return printer().Emit(a, func() {
				fmt.Printf("%s  type=%s version=%s port=%d size=%s\n", a.Name, a.Type, a.Version, a.Port, a.Size)
			})
		},
	}
	c.Flags().StringVarP(&outFmt, "output", "o", "", "output format: yaml")
	return c
}

func addonEditCmd() *cobra.Command {
	return &cobra.Command{
		Use: "edit <name>", Short: "Edit an addon's full spec in $EDITOR", Args: cobra.ExactArgs(1),
		Example: "  naru addon edit db -p myproj",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagNoInput {
				return fmt.Errorf("edit needs an interactive editor; use `naru addon apply -f` instead")
			}
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var cur apitypes.AddonSpec
			if err := cl.Get(cmd.Context(), addonPath(project, args[0]), &cur); err != nil {
				return err
			}
			initial, err := marshalSpecYAML(cur)
			if err != nil {
				return err
			}
			edited, err := editInEditor(initial, "yaml")
			if err != nil {
				return err
			}
			if edited == nil {
				output.Info("no changes")
				return nil
			}
			var spec apitypes.AddonSpec
			if err := yamlUnmarshal(edited, &spec); err != nil {
				return err
			}
			spec.Name, spec.Type = args[0], cur.Type // name + type are immutable
			action, err := upsertAddon(cmd, cl, project, spec)
			if err != nil {
				return err
			}
			output.Success(action + " addon " + args[0])
			return nil
		},
	}
}

func addonApplyCmd() *cobra.Command {
	var file string
	c := &cobra.Command{
		Use: "apply", Short: "Create or update an addon from a spec file (-f)",
		Example: "  naru addon apply -f db.yaml -p myproj",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, project, err := clientAndProject()
			if err != nil {
				return err
			}
			var spec apitypes.AddonSpec
			if err := loadSpecFile(file, &spec); err != nil {
				return err
			}
			action, err := upsertAddon(cmd, cl, project, spec)
			if err != nil {
				return err
			}
			output.Success(action + " addon " + spec.Name)
			return nil
		},
	}
	c.Flags().StringVarP(&file, "file", "f", "", "spec file (YAML/JSON, - for stdin)")
	_ = c.MarkFlagRequired("file")
	return c
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
		Use: "conn <name>", Short: "Show connection info, including the password", Args: cobra.ExactArgs(1),
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
				fmt.Printf("type:     %s\nhost:     %s\nport:     %d\n", dto.Type, dto.Host, dto.Port)
				if dto.Username != "" {
					fmt.Printf("username: %s (default superuser)\n", dto.Username)
				}
				fmt.Printf("password: %s\n", dto.Password)
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
