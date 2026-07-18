package cli

import (
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Full pipeline: tools → download → patch → fdroid update",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := loadApp(cmd)
			if err != nil {
				return err
			}
			return a.RunFull(ctxOf(cmd))
		},
	}
}
