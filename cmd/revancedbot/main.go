package main

import (
	"log/slog"
	"os"

	"github.com/lucasew/revancedbot/internal/cli"
	_ "github.com/lucasew/revancedbot/internal/drivers"
)

func main() {
	err := cli.NewRoot().Execute()
	// RunE errors skip PersistentPostRun; Close anyway so the TUI overlay
	// does not swallow slog.Error on the way out.
	if closeErr := cli.CloseSession(); err == nil {
		err = closeErr
	}
	if err != nil {
		slog.Error("command failed", "err", err)
		os.Exit(1)
	}
}
