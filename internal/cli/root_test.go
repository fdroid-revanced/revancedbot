package cli

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasew/workspaced/pkg/logging"
	"github.com/spf13/cobra"
)

func TestPersistentPreRun_appliesYAMLLogLevel(t *testing.T) {
	restoreLogger(t)
	repo := writeRepoYAML(t, "log_level: debug\n")
	root := NewRoot()
	if err := root.PersistentPreRunE(root, []string{repo}); err != nil {
		t.Fatal(err)
	}

	assertEnabled(t, logging.GetLogger(root.Context()), slog.LevelDebug)
	assertEnabled(t, slog.Default(), slog.LevelDebug)
}

func TestPersistentPreRun_warnHidesDebug(t *testing.T) {
	restoreLogger(t)
	repo := writeRepoYAML(t, "log_level: warn\n")
	root := NewRoot()
	if err := root.PersistentPreRunE(root, []string{repo}); err != nil {
		t.Fatal(err)
	}

	log := logging.GetLogger(root.Context())
	assertDisabled(t, log, slog.LevelDebug)
	assertEnabled(t, log, slog.LevelWarn)
	assertDisabled(t, slog.Default(), slog.LevelDebug)
}

func TestPersistentPreRun_invalidLogLevel(t *testing.T) {
	restoreLogger(t)
	repo := writeRepoYAML(t, "log_level: verbose\n")
	root := NewRoot()
	if err := root.PersistentPreRunE(root, []string{repo}); err == nil {
		t.Fatal("expected invalid log_level to fail")
	}
}

func TestLoadApp_reloadsHandlerAfterYAML(t *testing.T) {
	restoreLogger(t)
	repo := t.TempDir()
	root := NewRoot()
	if err := root.PersistentPreRunE(root, []string{repo}); err != nil {
		t.Fatal(err)
	}
	assertDisabled(t, logging.GetLogger(root.Context()), slog.LevelDebug)

	if err := os.WriteFile(filepath.Join(repo, "revancedbot.yaml"), []byte("log_level: debug\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadApp(root, []string{repo}, loadOpts{}); err != nil {
		t.Fatal(err)
	}
	assertEnabled(t, logging.GetLogger(root.Context()), slog.LevelDebug)
	assertEnabled(t, slog.Default(), slog.LevelDebug)
}

func TestInstallLogLevel_onCommandContext(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	cmd := &cobra.Command{Use: "test"}
	if err := installLogLevel(cmd, "error"); err != nil {
		t.Fatal(err)
	}
	log := logging.GetLogger(cmd.Context())
	assertDisabled(t, log, slog.LevelInfo)
	assertEnabled(t, log, slog.LevelError)
}

func restoreLogger(t *testing.T) {
	t.Helper()
	prev := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(prev)
		if err := CloseSession(); err != nil {
			t.Errorf("CloseSession: %v", err)
		}
	})
}

func writeRepoYAML(t *testing.T, yaml string) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "revancedbot.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

func assertEnabled(t *testing.T, log *slog.Logger, level slog.Level) {
	t.Helper()
	if !log.Enabled(t.Context(), level) {
		t.Fatalf("Enabled(%s)=false, want true", level)
	}
}

func assertDisabled(t *testing.T, log *slog.Logger, level slog.Level) {
	t.Helper()
	if log.Enabled(t.Context(), level) {
		t.Fatalf("Enabled(%s)=true, want false", level)
	}
}
