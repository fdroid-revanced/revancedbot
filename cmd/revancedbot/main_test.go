package main

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/lucasew/revancedbot/internal/cli"
	"github.com/lucasew/workspaced/pkg/logging"
)

func TestInstallLog_plainHandlerBeforeExecute(t *testing.T) {
	t.Setenv("CI", "1")
	t.Setenv("NO_COLOR", "1")
	t.Cleanup(func() {
		if err := cli.CloseSession(); err != nil {
			t.Errorf("CloseSession: %v", err)
		}
	})

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = old
	})

	installLog()
	if _, ok := slog.Default().Handler().(*logging.PlainHandler); !ok {
		t.Fatalf("handler %T, want *logging.PlainHandler", slog.Default().Handler())
	}

	root := cli.NewRoot()
	root.SetArgs([]string{"definitely-not-a-command"})
	root.SetErr(io.Discard)
	execErr := root.Execute()
	if execErr == nil {
		t.Fatal("expected unknown command error")
	}
	if err := cli.CloseSession(); err != nil {
		t.Fatal(err)
	}
	slog.Error("command failed", "err", execErr)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	gotb, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	got := string(gotb)
	if strings.Contains(got, "level=ERROR") || strings.Contains(got, "time=") {
		t.Fatalf("stdlib TextHandler format: %q", got)
	}
	if !strings.Contains(got, "command failed") {
		t.Fatalf("missing message: %q", got)
	}
}
