package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print the CLI version",
		Args:    cobra.NoArgs,
		Example: "  naru version\n  naru version --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printer().Emit(map[string]string{"version": version}, func() {
				fmt.Println("naru version " + version)
			})
		},
	}
}
