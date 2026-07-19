package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/lucasew/revancedbot/internal/app"
	"github.com/lucasew/revancedbot/internal/config"
	"github.com/lucasew/revancedbot/internal/version"
	"github.com/lucasew/workspaced/pkg/logging"
	"github.com/lucasew/workspaced/pkg/taskgroup"
	"github.com/spf13/cobra"
)

var (
	cfgFile   string
	cacheFlag string

	sessionMu sync.Mutex
	session   *taskgroup.Session
)

// NewRoot builds the revancedbot command tree.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "revancedbot",
		Short:         "Build a simple binary F-Droid repo of ReVanced-patched apps",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			h := logging.NewPlainHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
			ctx := logging.NewRootContext(slog.New(h))
			// Keep slog.Default in sync so any leftover stdlib slog calls match.
			slog.SetDefault(slog.New(h))
			sess, ctx := taskgroup.Enter(ctx, limitsFromArgs(args))
			sessionMu.Lock()
			session = sess
			sessionMu.Unlock()
			cmd.SetContext(ctx)
			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			sessionMu.Lock()
			sess := session
			session = nil
			sessionMu.Unlock()
			if sess != nil {
				return sess.Close()
			}
			return nil
		},
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "", "override path to revancedbot.yaml (default: REPO/revancedbot.yaml)")
	root.PersistentFlags().StringVar(&cacheFlag, "cache", "", "cache directory (default: mkdtemp; tools/stock/signing)")

	root.AddCommand(
		newKeysCmd(),
		newFetchToolsCmd(),
		newListJobsCmd(),
		newDownloadCmd(),
		newPatchCmd(),
		newFDroidInitCmd(),
		newFDroidUpdateCmd(),
		newRunCmd(),
		newSmokeCmd(),
	)
	return root
}

// limitsFromArgs returns workspaced DefaultLimits with a tighter Internet cap,
// optionally overridden by pool_* in REPO/revancedbot.yaml when present.
//
// Map pool trick: child tasks use PoolKind; Control is unlimited, Internet/IO/CPU
// share the session semaphores. Packages Map stays Control (orchestrate only);
// stock HTTP goes through httpclient.WithProgress as Internet tasks — so this
// Internet limit is what caps concurrent APK downloads/scrapes (not the Map).
// Do not put packages Map on Internet while downloads also take Internet or
// you can deadlock (parent holds a slot, child HTTP wants another).
func limitsFromArgs(args []string) taskgroup.Limits {
	limits := taskgroup.DefaultLimits()
	// Prefer fewer parallel store scrapes (403/429). workspaced default is 4.
	limits.Internet = 2
	if len(args) < 1 {
		return limits
	}
	cfg, err := config.LoadFromRepo(args[0], cacheFlag, cfgFile)
	if err != nil {
		return limits
	}
	if cfg.PoolIO > 0 {
		limits.IO = cfg.PoolIO
	}
	if cfg.PoolCPU > 0 {
		limits.CPU = cfg.PoolCPU
	}
	if cfg.PoolInternet > 0 {
		limits.Internet = cfg.PoolInternet
	}
	return limits
}

func loadApp(cmd *cobra.Command, args []string) (*app.App, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("missing REPO path (F-Droid simple-binary root)")
	}
	cfg, err := config.LoadFromRepo(args[0], cacheFlag, cfgFile)
	if err != nil {
		return nil, err
	}
	return app.New(cfg)
}

func ctxOf(cmd *cobra.Command) context.Context {
	ctx := cmd.Context()
	if ctx == nil {
		return logging.NewRootContext(nil)
	}
	if !logging.ContextHasLogger(ctx) {
		ctx = logging.ContextWithLogger(ctx, slog.Default())
	}
	return ctx
}

// schedule runs fn as a named isolated task and waits for it (error returns to RunE).
// Prefer this for subcommands that should show progress bars. Do not use for pure
// stdout producers (keys generate) — short Unit tasks + TUI teardown can deadlock.
func schedule(ctx context.Context, name string, pool taskgroup.PoolKind, fn func(context.Context, *taskgroup.Status) error) error {
	return taskgroup.GoIsolated(ctx, name, pool, fn)
}
