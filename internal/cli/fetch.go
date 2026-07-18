package cli

import (
	"github.com/spf13/cobra"
	"workspaced/pkg/logging"
)

func newFetchToolsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fetch-tools",
		Short: "Download latest ReVanced CLI jar and patches rvp",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := loadApp(cmd)
			if err != nil {
				return err
			}
			ctx := ctxOf(cmd)
			if err := a.FetchTools(ctx); err != nil {
				return err
			}
			logging.GetLogger(ctx).Info("tools ready", "cli", a.WS.PatcherJAR(), "patches", a.WS.PatchesRVP())
			return nil
		},
	}
}
