package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sync"

	"github.com/lucasew/revancedbot/internal/app"
	"github.com/lucasew/revancedbot/internal/config"
	"github.com/lucasew/revancedbot/internal/version"
	"github.com/spf13/cobra"
	"workspaced/pkg/logging"
	"workspaced/pkg/taskgroup"
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
			limits := limitsFromArgs(args)
			sess, ctx := taskgroup.Enter(ctx, limits)
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

// limitsFromArgs loads pool_* from REPO/revancedbot.yaml when args[0] is a repo path.
// Unspecified pools use taskgroup defaults.
func limitsFromArgs(args []string) taskgroup.Limits {
	limits := taskgroup.DefaultLimits()
	if len(args) < 1 {
		return limits
	}
	cfg, err := config.LoadFromRepo(args[0], cacheFlag, cfgFile)
	if err != nil {
		return limits
	}
	return poolsFromConfig(cfg)
}

func poolsFromConfig(cfg *config.Config) taskgroup.Limits {
	limits := taskgroup.DefaultLimits()
	if cfg == nil {
		return limits
	}
	if cfg.PoolIO > 0 {
		limits.IO = cfg.PoolIO
	}
	if cfg.PoolCPU > 0 {
		limits.CPU = cfg.PoolCPU
	} else {
		limits.CPU = max(runtime.NumCPU(), 1)
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
