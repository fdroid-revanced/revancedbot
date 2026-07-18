package cli

import (
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
			return a.RunFull(ctxOf(cmd))
		},
	}
}

func newSmokeCmd() *cobra.Command {
	var maxOK int
	c := &cobra.Command{
		Use:   "smoke REPO",
		Short: "Try packages until N succeed, then fdroid update (TMP e2e helper)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := loadApp(cmd, args)
			if err != nil {
				return err
			}
			n, err := a.RunSmoke(ctxOf(cmd), maxOK)
			if err != nil {
				return err
			}
			cmd.Printf("smoke ok: %d package(s) patched; fdroid updated\n", n)
			return nil
		},
	}
	c.Flags().IntVar(&maxOK, "max", 1, "stop after this many successful patches")
	return c
}
