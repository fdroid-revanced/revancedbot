package cli

import (
	"bytes"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasew/revancedbot/internal/version"
	"github.com/lucasew/workspaced/pkg/logging"
	"github.com/spf13/cobra"
)

func TestVersion_printsVersion(t *testing.T) {
	t.Setenv("CI", "1")
	t.Cleanup(func() { closeSession(t) })

	root := NewRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(out.String())
	if got != version.Version {
		t.Fatalf("version = %q; want %q", got, version.Version)
	}
}

func TestVersion_extraArgsCobraDefault(t *testing.T) {
	t.Setenv("CI", "1")
	t.Cleanup(func() { closeSession(t) })

	root := NewRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version", "leftover"})
	// Args unset → Cobra ArbitraryArgs; extra args are not a custom error.
	if err := root.Execute(); err != nil {
		t.Fatalf("cobra default accepts extra args: %v", err)
	}
	got := strings.TrimSpace(out.String())
	if got != version.Version {
		t.Fatalf("version = %q; want %q", got, version.Version)
	}
}

func TestConfigFlag_missingFileRejectedWithoutRepo(t *testing.T) {
	t.Setenv("CI", "1")
	t.Cleanup(func() {
		cfgFile = ""
		closeSession(t)
	})

	missing := filepath.Join(t.TempDir(), "nope.yaml")
	root := NewRoot()
	root.SetArgs([]string{"keys", "generate", "--config", missing})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected missing --config to fail on keys generate")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v; want fs.ErrNotExist", err)
	}
	var pe *os.PathError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v; want *os.PathError", err)
	}
	if pe.Path != missing {
		t.Fatalf("path = %q; want %q", pe.Path, missing)
	}
}

func TestConfigFlag_existingFileAcceptedOnVersion(t *testing.T) {
	t.Setenv("CI", "1")
	t.Cleanup(func() {
		cfgFile = ""
		closeSession(t)
	})

	p := filepath.Join(t.TempDir(), "revancedbot.yaml")
	if err := os.WriteFile(p, []byte("repo_name: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := NewRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--config", p, "version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != version.Version {
		t.Fatalf("version = %q; want %q", got, version.Version)
	}
}

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
	if _, err := loadApp([]string{repo}, loadOpts{}); err != nil {
		t.Fatal(err)
	}
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

func closeSession(t *testing.T) {
	t.Helper()
	if err := CloseSession(); err != nil {
		t.Errorf("CloseSession: %v", err)
	}
}
