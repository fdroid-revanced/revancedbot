package cli

import (
	"context"
	"fmt"

	"github.com/lucasew/revancedbot/internal/download"
	"github.com/lucasew/revancedbot/internal/workspace"
	"github.com/spf13/cobra"
	"workspaced/pkg/taskgroup"
)

func newDownloadCmd() *cobra.Command {
	var pkg, ver string
	c := &cobra.Command{
		Use:   "download REPO",
		Short: "Download one stock APK into CACHE",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := loadApp(cmd, args)
			if err != nil {
				return err
			}
			if pkg == "" {
				return fmt.Errorf("--package is required")
			}
			ctx := ctxOf(cmd)
			return schedule(ctx, "download:"+pkg, taskgroup.Control, func(ctx context.Context, s *taskgroup.Status) error {
				s.Update(pkg)
				path := a.WS.StockAPKPath(pkg, ver)
				if workspace.CacheHit(path) && download.AcceptCached(path) == nil {
					afterWait(ctx, func() error {
						fmt.Printf("cache\t%s\n", path)
						return nil
					})
					return nil
				}
				reg := download.DefaultRegistry()
				order := a.Cfg.DownloaderOrder
				if len(order) == 0 {
					order = download.DefaultOrder
				}
				res, err := download.FetchFirst(ctx, reg, order, download.Request{
					PackageID: pkg,
					Version:   ver,
				}, a.WS.StockAPKs)
				if err != nil {
					return err
				}
				afterWait(ctx, func() error {
					fmt.Printf("%s\t%s\t%s\n", res.SourceID, res.SHA256, res.Path)
					return nil
				})
				return nil
			})
		},
	}
	c.Flags().StringVar(&pkg, "package", "", "stock package id")
	c.Flags().StringVar(&ver, "version", "", "version (empty = latest)")
	return c
}
