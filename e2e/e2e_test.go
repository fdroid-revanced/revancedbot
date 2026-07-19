//go:build e2e

package e2e_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/lucasew/revancedbot/internal/app"
	_ "github.com/lucasew/revancedbot/internal/drivers"
	"github.com/lucasew/revancedbot/internal/config"
	"github.com/lucasew/revancedbot/internal/signing"
	"github.com/lucasew/revancedbot/internal/toolscheck"
	"workspaced/pkg/logging"
)

// TestE2E_RepoCacheLayout exercises REPO + CACHE + tools + list-jobs + fdroid-init.
func TestE2E_RepoCacheLayout(t *testing.T) {
	if err := toolscheck.Check(toolscheck.KeysOnly()); err != nil {
		t.Skip(err)
	}
	ctx := logging.NewWriterContext(t.Output())
	repo := t.TempDir()
	cache := t.TempDir()

	// write authority yaml
	yaml := []byte("repo_name: e2e\nrepo_url: https://example.invalid/fdroid/repo\nrepo_description: e2e\n")
	if err := os.WriteFile(filepath.Join(repo, "revancedbot.yaml"), yaml, 0o644); err != nil {
		t.Fatal(err)
	}

	enc, err := signing.Generate("revancedbot")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVANCEDBOT_SIGNING", enc)

	cfg, err := config.LoadFromRepo(repo, cache, "")
	if err != nil {
		t.Fatal(err)
	}
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.LoadSigning(); err != nil {
		t.Fatal(err)
	}
	// keystore must be under cache
	if filepath.Dir(filepath.Dir(a.WS.KeystorePath)) != cache && !filepath.HasPrefix(a.WS.KeystorePath, cache) {
		// KeystorePath is cache/signing/keystore.jks
		if !filepath.HasPrefix(a.WS.KeystorePath, a.WS.Cache) {
			t.Fatalf("keystore not under cache: %s", a.WS.KeystorePath)
		}
	}
	if err := a.WriteFDroidConfig(); err != nil {
		t.Fatal(err)
	}
	// config.yml exists; keystore does not live in repo
	if _, err := os.Stat(a.WS.FDroidConfig()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "keystore.jks")); err == nil {
		t.Fatal("keystore must not be in REPO")
	}

	c, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := a.FetchTools(c); err != nil {
		t.Fatalf("fetch-tools: %v", err)
	}
	jobs, err := a.ListJobs()
	if err != nil {
		t.Fatalf("list-jobs: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("no jobs")
	}
	t.Logf("jobs=%d cache=%s repo=%s", len(jobs), a.WS.Cache, a.WS.Repo)

	// Optional full kitchen sink if tools present
	if err := toolscheck.Check(toolscheck.DefaultRun()); err != nil {
		t.Logf("skip smoke patch/fdroid: %v", err)
		return
	}
	// Try one small package if REVANCEDBOT_E2E_PACKAGE set, else skip download loop in unit e2e
	if os.Getenv("REVANCEDBOT_E2E_FULL") != "1" {
		t.Log("set REVANCEDBOT_E2E_FULL=1 to run download+patch+fdroid in e2e")
		return
	}
	n, err := a.RunSmoke(c, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("smoke packages ok: %d", n)
	matches, _ := filepath.Glob(filepath.Join(a.WS.FDroidRepo, "index-v1.jar"))
	if len(matches) == 0 {
		t.Fatal("expected index-v1.jar after fdroid update")
	}
}

func TestE2E_CLI(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip(err)
	}
	if err := toolscheck.Check(toolscheck.KeysOnly()); err != nil {
		t.Skip(err)
	}
	root := repoRoot(t)
	bin := filepath.Join(t.TempDir(), "revancedbot")
	build := exec.Command("go", "build", "-o", bin, "./cmd/revancedbot")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	repo := t.TempDir()
	cache := t.TempDir()
	_ = os.WriteFile(filepath.Join(repo, "revancedbot.yaml"), []byte("repo_name: e2e\n"), 0o644)

	out, err := exec.Command(bin, "keys", "generate").CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	// last line blob
	lines := splitNonEmpty(string(out))
	blob := lines[len(lines)-1]
	env := append(os.Environ(), "REVANCEDBOT_SIGNING="+blob)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		cmd.Dir = root
		o, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, o)
		}
	}
	run("keys", "validate", repo, "--cache", cache)
	run("fetch-tools", repo, "--cache", cache)
	run("list-jobs", repo, "--cache", cache)
	run("fdroid-init", repo, "--cache", cache)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(wd) == "e2e" {
		return filepath.Dir(wd)
	}
	return wd
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, line := range splitLines(s) {
		if line != "" && line[0] != '#' {
			out = append(out, line)
		}
	}
	return out
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
