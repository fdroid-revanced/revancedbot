//go:build e2e

package e2e_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucasew/revancedbot/internal/app"
	"github.com/lucasew/revancedbot/internal/config"
	_ "github.com/lucasew/revancedbot/internal/drivers"
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
	if err := a.PrepareStage(); err != nil {
		t.Fatal(err)
	}
	// stage config in CACHE; not live REPO until PublishStage
	if _, err := os.Stat(a.WS.StageConfig()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.WS.LiveConfig()); err == nil {
		t.Fatal("config.yml must not be written directly to live REPO before publish")
	}
	if err := a.PublishStage(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.WS.LiveConfig()); err != nil {
		t.Fatal("config.yml missing from REPO after publish")
	}
	if !filepath.HasPrefix(a.WS.Stage, a.WS.Cache) {
		t.Fatalf("stage not under cache: %s", a.WS.Stage)
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
	// Index is published into live REPO/repo after atomic PublishStage.
	matches, _ := filepath.Glob(filepath.Join(a.WS.LiveRepoDir(), "index-v1.jar"))
	if len(matches) == 0 {
		t.Fatal("expected index-v1.jar in REPO/repo after publish")
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

	// Blob is the only line on stdout; slog hints go to stderr.
	cmdGen := exec.Command(bin, "keys", "generate")
	var stdout, stderr []byte
	var err error
	stdout, err = cmdGen.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = ee.Stderr
		}
		t.Fatalf("keys generate: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	blob := pickSigningBlob(string(stdout))
	if blob == "" {
		t.Fatalf("no signing blob on stdout: %q", stdout)
	}
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

func pickSigningBlob(stdout string) string {
	// Prefer a long base64-ish line (the secret); ignore empty/log noise.
	var best string
	for _, line := range splitLines(stdout) {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		if len(line) > len(best) && looksLikeBlob(line) {
			best = line
		}
	}
	return best
}

func looksLikeBlob(s string) bool {
	if len(s) < 80 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=' {
			continue
		}
		return false
	}
	return true
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
