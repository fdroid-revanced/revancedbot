package cli

import (
	"fmt"

	"github.com/lucasew/revancedbot/internal/toolscheck"
	"github.com/spf13/cobra"
)

func newFDroidInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fdroid-init REPO",
		Short: "Ensure REPO layout and write generated config.yml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := toolscheck.Check(toolscheck.KeysOnly()); err != nil {
				return err
			}
			a, err := loadApp(cmd, args)
			if err != nil {
				return err
			}
			if err := a.LoadSigning(); err != nil {
				return err
			}
			if err := a.WriteFDroidConfig(); err != nil {
				return err
			}
			fmt.Println("repo:", a.WS.Repo)
			fmt.Println("config:", a.WS.FDroidConfig())
			fmt.Println("keystore (cache):", a.WS.KeystorePath)
			return nil
		},
	}
}

func newFDroidUpdateCmd() *cobra.Command {
	var createMeta bool
	c := &cobra.Command{
		Use:   "fdroid-update REPO",
		Short: "Run fdroid update on REPO (host fdroid/apksigner/aapt required)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := toolscheck.Check(toolscheck.DefaultRun()); err != nil {
				return err
			}
			a, err := loadApp(cmd, args)
			if err != nil {
				return err
			}
			if err := a.LoadSigning(); err != nil {
				return err
			}
			if err := a.WriteFDroidConfig(); err != nil {
				return err
			}
			return a.FDroidUpdate(createMeta)
		},
	}
	c.Flags().BoolVarP(&createMeta, "create-metadata", "c", true, "pass -c to fdroid update")
	return c
}
