package cli

import (
	"context"

	"github.com/lucasew/revancedbot/internal/toolscheck"
	"github.com/spf13/cobra"
	"workspaced/pkg/logging"
	"workspaced/pkg/taskgroup"
)

func newFDroidInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fdroid-init REPO",
		Short: "Seed stage, write config in CACHE, atomically publish layout to REPO",
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
			log := logging.GetLogger(ctx)
			if err := a.LoadSigning(); err != nil {
				return err
			}
			if err := a.PrepareStage(); err != nil {
				return err
			}
			if err := a.PublishStage(); err != nil {
				return err
			}
			log.Info("fdroid init ok",
				"repo", a.WS.Repo,
				"config", a.WS.LiveConfig(),
				"stage", a.WS.Stage,
				"keystore", a.WS.KeystorePath,
			)
			return nil
		},
	}
}

func newFDroidUpdateCmd() *cobra.Command {
	var createMeta bool
	c := &cobra.Command{
		Use:   "fdroid-update REPO",
		Short: "fdroid update in CACHE stage then atomically publish to REPO",
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
			log := logging.GetLogger(ctx)
			return schedule(ctx, "rebuild F-Droid repo", taskgroup.Control, func(ctx context.Context, s *taskgroup.Status) error {
				s.Update("prepare stage")
				if err := a.LoadSigning(); err != nil {
					return err
				}
				if err := a.PrepareStage(); err != nil {
					return err
				}
				if err := a.FDroidUpdate(ctx, createMeta); err != nil {
					return err
				}
				if err := a.PublishStage(); err != nil {
					return err
				}
				log.Info("fdroid update ok", "repo", a.WS.Repo, "stage", a.WS.Stage)
				return nil
			})
		},
	}
	c.Flags().BoolVarP(&createMeta, "create-metadata", "c", true, "pass -c to fdroid update")
	return c
}
