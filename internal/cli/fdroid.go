package cli

import (
	"context"
	"fmt"

	"github.com/lucasew/revancedbot/internal/toolscheck"
	"github.com/spf13/cobra"
	"workspaced/pkg/taskgroup"
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
			ctx := ctxOf(cmd)
			return schedule(ctx, "fdroid-init", taskgroup.IO, func(ctx context.Context, s *taskgroup.Status) error {
				defer s.Unit()()
				s.Update("config")
				if err := a.LoadSigning(); err != nil {
					return err
				}
				if err := a.WriteFDroidConfig(); err != nil {
					return err
				}
				afterWait(ctx, func() error {
					fmt.Println("repo:", a.WS.Repo)
					fmt.Println("config:", a.WS.FDroidConfig())
					fmt.Println("keystore (cache):", a.WS.KeystorePath)
					return nil
				})
				return nil
			})
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
			ctx := ctxOf(cmd)
			return schedule(ctx, "fdroid-update-cmd", taskgroup.Control, func(ctx context.Context, s *taskgroup.Status) error {
				s.Update("setup")
				if err := a.LoadSigning(); err != nil {
					return err
				}
				if err := a.WriteFDroidConfig(); err != nil {
					return err
				}
				return a.FDroidUpdate(ctx, createMeta)
			})
		},
	}
	c.Flags().BoolVarP(&createMeta, "create-metadata", "c", true, "pass -c to fdroid update")
	return c
}
