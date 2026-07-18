package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newFDroidInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fdroid-init",
		Short: "Scaffold simple binary F-Droid layout and config.yml",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := loadApp(cmd)
			if err != nil {
				return err
			}
			if err := a.LoadSigning(); err != nil {
				return err
			}
			if err := a.EnsureFDroidConfig(); err != nil {
				return err
			}
			fmt.Println("fdroid root:", a.WS.FDroid)
			return nil
		},
	}
}

func newFDroidUpdateCmd() *cobra.Command {
	var createMeta bool
	c := &cobra.Command{
		Use:   "fdroid-update",
		Short: "Run fdroid update on the workspace F-Droid tree",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := loadApp(cmd)
			if err != nil {
				return err
			}
			if err := a.LoadSigning(); err != nil {
				return err
			}
			if err := a.EnsureFDroidConfig(); err != nil {
				return err
			}
			return a.FDroidUpdate(createMeta)
		},
	}
	c.Flags().BoolVarP(&createMeta, "create-metadata", "c", true, "pass -c to fdroid update")
	return c
}
