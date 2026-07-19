package cli

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/lucasew/workspaced/pkg/logging"
	"github.com/lucasew/workspaced/pkg/taskgroup"
)

func newFetchToolsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fetch-tools REPO",
		Short: "Download latest ReVanced CLI jar and patches into CACHE",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := loadApp(cmd, args)
			if err != nil {
				return err
			}
			ctx := ctxOf(cmd)
			return schedule(ctx, "download ReVanced tools", taskgroup.Control, func(ctx context.Context, s *taskgroup.Status) error {
				s.Update("CLI jar + patches")
				if err := a.FetchTools(ctx); err != nil {
					return err
				}
				logging.GetLogger(ctx).Info("tools ready",
					"cli", a.WS.PatcherJAR(),
					"patches", a.WS.PatchesRVP(),
					"cache", a.WS.Cache,
				)
				return nil
			})
		},
	}
}
