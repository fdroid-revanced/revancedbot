package cli

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasew/revancedbot/internal/signing"
	"github.com/lucasew/revancedbot/internal/toolscheck"
)

func TestFDroidInitPublishesEmptyLayout(t *testing.T) {
	if err := toolscheck.Check(toolscheck.KeysOnly()); err != nil {
		t.Skip(err)
	}
	repo := t.TempDir()
	cache := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "revancedbot.yaml"), []byte("repo_name: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	enc, err := signing.Generate("revancedbot")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVANCEDBOT_SIGNING", enc)

	cmd := NewRoot()
	cmd.SetArgs([]string{"fdroid-init", repo, "--cache", cache})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config.yml", "repo", "metadata"} {
		if _, err := os.Stat(filepath.Join(repo, name)); err != nil {
			t.Fatalf("live %s: %v", name, err)
		}
	}
	for _, name := range []string{"index-v1.json", "index-v2.json", "index.xml", "entry.json"} {
		if _, err := os.Stat(filepath.Join(repo, "repo", name)); err == nil {
			t.Fatalf("init must not require index %s", name)
		}
	}
}
