package cli

import (
	"github.com/lucasew/workspaced/pkg/logging"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run REPO",
		Short: "Full pipeline: tools → download → patch → fdroid update (REPO in/out)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := loadApp(cmd, args)
			if err != nil {
				return err
			}
			// RunFull owns Each/GoIsolated (packages, patch, fdroid); no outer Unit shell.
			return a.RunFull(ctxOf(cmd))
		},
	}
}

func newSmokeCmd() *cobra.Command {
	var maxOK int
	c := &cobra.Command{
		Use:   "smoke REPO",
		Short: "Try packages in random order until N succeed, then fdroid update",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := loadApp(cmd, args)
			if err != nil {
				return err
			}
			ctx := ctxOf(cmd)
			log := logging.GetLogger(ctx)
			n, err := a.RunSmoke(ctx, maxOK)
			if err != nil {
				return err
			}
			log.Info("smoke ok", "packages", n)
			return nil
		},
	}
	c.Flags().IntVar(&maxOK, "max", 1, "stop after this many successful patches")
	return c
}
