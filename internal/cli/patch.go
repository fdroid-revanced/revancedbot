package cli

import (
	"context"
	"fmt"

	"github.com/lucasew/revancedbot/internal/revanced"
	"github.com/lucasew/revancedbot/internal/toolscheck"
	"github.com/spf13/cobra"
	"github.com/lucasew/workspaced/pkg/logging"
	"github.com/lucasew/workspaced/pkg/taskgroup"
)

func newPatchCmd() *cobra.Command {
	var in, out string
	c := &cobra.Command{
		Use:   "patch REPO",
		Short: "Patch one APK with ReVanced into CACHE/work (uses operator keystore)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := toolscheck.Check([]toolscheck.Requirement{{Name: "java"}}); err != nil {
				return err
			}
			a, err := loadApp(cmd, args)
			if err != nil {
				return err
			}
			if in == "" || out == "" {
				return fmt.Errorf("--in and --out are required")
			}
			ctx := ctxOf(cmd)
			log := logging.GetLogger(ctx)
			return schedule(ctx, "patch APK", taskgroup.Control, func(ctx context.Context, s *taskgroup.Status) error {
				s.Update("prepare")
				if err := a.LoadSigning(); err != nil {
					return err
				}
				if err := a.FetchTools(ctx); err != nil {
					return err
				}
				return taskgroup.GoIsolated(ctx, "run ReVanced CLI", taskgroup.CPU, func(ctx context.Context, s *taskgroup.Status) error {
					defer s.Unit()()
					s.Update("ReVanced CLI")
					patches, err := revanced.Patch(revanced.PatchOptions{
						CLIJar:                  a.WS.PatcherJAR(),
						PatchesRVP:              a.WS.PatchesRVP(),
						InputAPK:                in,
						OutputAPK:               out,
						KeystorePath:            a.WS.KeystorePath,
						Blob:                    a.Blob,
						EnableChangePackageName: true,
					})
					if err != nil {
						return err
					}
					log.Info("patch ok", "out", out, "patches", len(patches))
					for _, p := range patches {
						log.Info("patch applied", "name", p)
					}
					return nil
				})
			})
		},
	}
	c.Flags().StringVar(&in, "in", "", "input APK")
	c.Flags().StringVar(&out, "out", "", "output APK")
	return c
}
