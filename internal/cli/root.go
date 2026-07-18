package cli

import (
	"context"
	"log/slog"
	"os"
	"sync"

	"github.com/lucasew/revancedbot/internal/app"
	"github.com/lucasew/revancedbot/internal/config"
	"github.com/lucasew/revancedbot/internal/version"
	"github.com/spf13/cobra"
	"workspaced/pkg/logging"
	"workspaced/pkg/taskgroup"
)

var (
	cfgFile       string
	workspaceFlag string

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
			h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
			ctx := logging.NewRootContext(slog.New(h))
			sess, ctx := taskgroup.Enter(ctx, taskgroup.DefaultLimits())
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

	root.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./revancedbot.yaml)")
	root.PersistentFlags().StringVar(&workspaceFlag, "workspace", "", "workspace directory (overrides config)")

	root.AddCommand(
		newKeysCmd(),
		newFetchToolsCmd(),
		newListJobsCmd(),
		newDownloadCmd(),
		newPatchCmd(),
		newFDroidInitCmd(),
		newFDroidUpdateCmd(),
		newRunCmd(),
	)
	return root
}

func loadApp(cmd *cobra.Command) (*app.App, error) {
	cfg, err := config.Load(cfgFile, workspaceFlag)
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
