package main

import (
	"log/slog"
	"os"

	"github.com/lucasew/revancedbot/internal/cli"
	_ "github.com/lucasew/revancedbot/internal/drivers"
)

func main() {
	if err := cli.NewRoot().Execute(); err != nil {
		slog.Error("command failed", "err", err)
		os.Exit(1)
	}
}
