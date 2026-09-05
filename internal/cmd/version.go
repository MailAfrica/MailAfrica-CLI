package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/MailAfrica/MailAfrica-CLI/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the mailafrica version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "mailafrica %s\n", version.Version)
		},
	}
}
