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
			cfg := optionalConfig(args)
			if err := installLogLevel(cmd, cfg.LogLevelOrDefault()); err != nil {
				return err
			}
			sess, ctx := taskgroup.Enter(cmd.Context(), limitsFromConfig(cfg))
			sessionMu.Lock()
			session = sess
			sessionMu.Unlock()
			cmd.SetContext(ctx)
			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			return CloseSession()
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

func optionalConfig(args []string) *config.Config {
	if len(args) < 1 {
		return nil
	}
	cfg, err := config.LoadFromRepo(args[0], cacheFlag, cfgFile)
	if err != nil {
		return nil
	}
	return cfg
}

func installLogLevel(cmd *cobra.Command, name string) error {
	level, err := config.ParseLogLevel(name)
	if err != nil {
		return err
	}
	h := logging.NewPlainHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	logger := slog.New(h)
	// Keep slog.Default in sync so any leftover stdlib slog calls match.
	slog.SetDefault(logger)
	ctx := cmd.Context()
	if ctx == nil {
		ctx = logging.NewRootContext(logger)
	} else {
		ctx = logging.ContextWithLogger(ctx, logger)
	}
	cmd.SetContext(ctx)
	return nil
}

// limitsFromConfig returns workspaced DefaultLimits with a tighter Internet cap,
// optionally overridden by pool_* in REPO/revancedbot.yaml when present.
//
// Map pool trick: child tasks use PoolKind; Control is unlimited, Internet/IO/CPU
// share the session semaphores. Packages Map stays Control (orchestrate only);
// stock HTTP goes through httpclient.WithProgress as Internet tasks — so this
// Internet limit is what caps concurrent APK downloads/scrapes (not the Map).
// Do not put packages Map on Internet while downloads also take Internet or
// you can deadlock (parent holds a slot, child HTTP wants another).
func limitsFromConfig(cfg *config.Config) taskgroup.Limits {
	limits := taskgroup.DefaultLimits()
	// Prefer fewer parallel store scrapes (403/429). workspaced default is 4.
	limits.Internet = 2
	if cfg == nil {
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

// CloseSession tears down the taskgroup Session (TUI + stderr overlay).
// Cobra skips PersistentPostRun when RunE fails, so main must call this
// after Execute as well. Idempotent.
func CloseSession() error {
	sessionMu.Lock()
	sess := session
	session = nil
	sessionMu.Unlock()
	if sess != nil {
		return sess.Close()
	}
	return nil
}

type loadOpts struct {
	requireDoc bool
}

func loadApp(cmd *cobra.Command, args []string, opts loadOpts) (*app.App, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("missing REPO path (F-Droid simple-binary root): %w", ErrBase)
	}
	var (
		cfg *config.Config
		err error
	)
	if opts.requireDoc {
		cfg, err = config.LoadFromRepoRequired(args[0], cacheFlag, cfgFile)
	} else {
		cfg, err = config.LoadFromRepo(args[0], cacheFlag, cfgFile)
	}
	if err != nil {
		return nil, err
	}
	// PersistentPreRunE may have used a default handler; YAML is authoritative.
	if cmd != nil {
		if err := installLogLevel(cmd, cfg.LogLevelOrDefault()); err != nil {
			return nil, err
		}
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
