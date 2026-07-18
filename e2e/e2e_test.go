//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucasew/revancedbot/internal/app"
	"github.com/lucasew/revancedbot/internal/config"
	"github.com/lucasew/revancedbot/internal/download"
	"github.com/lucasew/revancedbot/internal/fdroid"
	"github.com/lucasew/revancedbot/internal/revanced"
	"github.com/lucasew/revancedbot/internal/signing"
	"workspaced/pkg/logging"
)

// TestE2E_Pipeline walks the real bot path as far as the environment allows:
//
//  1. keytool signing blob (required)
//  2. fetch ReVanced CLI (required)
//  3. patches file (GitHub or REVANCEDBOT_PATCHES_FILE)
//  4. list-jobs (requires patches)
//  5. fdroid-init (required)
//  6. download+patch one package (soft unless REVANCEDBOT_E2E_STRICT)
//  7. fdroid update if fdroid is installed (soft unless strict)
//
//	mise run test:e2e
//
// Optional env:
//
//	GITHUB_TOKEN / REVANCEDBOT_GITHUB_TOKEN
//	REVANCEDBOT_PATCHES_FILE   local .rvp when GitHub is DMCA-blocked
//	REVANCEDBOT_PATCHES_REPO   owner/name alternate release repo
//	REVANCEDBOT_E2E_PACKAGE    force package id for download/patch
//	REVANCEDBOT_E2E_STRICT=1   fail soft steps instead of skipping
func TestE2E_Pipeline(t *testing.T) {
	requireOnPath(t, "java", "keytool")

	ctx := logging.NewWriterContext(t.Output())
	ws := t.TempDir()

	enc, err := signing.Generate("revancedbot")
	if err != nil {
		t.Fatalf("keys generate: %v", err)
	}
	blob, err := signing.DecodeBlob(enc)
	if err != nil {
		t.Fatalf("decode blob: %v", err)
	}
	if err := blob.Materialize(filepath.Join(ws, "signing", "keystore.p12")); err != nil {
		t.Fatalf("materialize keystore: %v", err)
	}
	t.Log("keys: ok")

	cfg := &config.Config{
		Workspace:       ws,
		RepoName:        "revancedbot e2e",
		RepoURL:         "https://example.invalid/fdroid/repo",
		RepoDescription: "e2e test repository",
		DownloaderOrder: []string{"apkpure"},
		PoolIO:          2,
		PoolInternet:    2,
		SigningBlob:     enc,
		GitHubToken:     firstEnv("GITHUB_TOKEN", "REVANCEDBOT_GITHUB_TOKEN"),
	}
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.LoadSigning(); err != nil {
		t.Fatalf("load signing: %v", err)
	}

	// --- CLI jar ---
	t.Run("fetch_cli", func(t *testing.T) {
		c, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := revanced.FetchCLI(c, cfg.GitHubToken, a.WS.PatcherJAR()); err != nil {
			t.Fatalf("fetch cli: %v", err)
		}
		st, err := os.Stat(a.WS.PatcherJAR())
		if err != nil || st.Size() < 1024 {
			t.Fatalf("cli jar missing/small: %v", err)
		}
		t.Logf("cli jar: %d bytes", st.Size())
	})

	// --- patches ---
	patchesOK := false
	t.Run("fetch_patches", func(t *testing.T) {
		if local := os.Getenv("REVANCEDBOT_PATCHES_FILE"); local != "" {
			if err := copyFile(local, a.WS.PatchesRVP()); err != nil {
				t.Fatalf("copy patches file: %v", err)
			}
			patchesOK = true
			t.Logf("using local patches: %s", local)
			return
		}
		c, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := revanced.FetchPatches(c, cfg.GitHubToken, a.WS.PatchesRVP()); err != nil {
			// Common on some networks: GitHub DMCA 451 for ReVanced/revanced-patches
			if strictE2E() {
				t.Fatalf("fetch patches: %v\n(set REVANCEDBOT_PATCHES_FILE to a local .rvp)", err)
			}
			t.Skipf("patches unavailable (non-strict): %v\nset REVANCEDBOT_PATCHES_FILE for full e2e", err)
		}
		st, err := os.Stat(a.WS.PatchesRVP())
		if err != nil || st.Size() < 100 {
			t.Fatalf("patches missing/small: %v", err)
		}
		patchesOK = true
		t.Logf("patches: %d bytes", st.Size())
	})

	var jobs []revanced.Job
	t.Run("list_jobs", func(t *testing.T) {
		if !patchesOK {
			if _, err := os.Stat(a.WS.PatchesRVP()); err != nil {
				t.Skip("no patches file")
			}
			patchesOK = true
		}
		var err error
		jobs, err = a.ListJobs()
		if err != nil {
			t.Fatalf("list-jobs: %v", err)
		}
		if len(jobs) == 0 {
			t.Fatal("expected at least one package from list-versions")
		}
		t.Logf("packages: %d first=%s versions=%v", len(jobs), jobs[0].PackageID, jobs[0].Versions)
	})

	t.Run("fdroid_init", func(t *testing.T) {
		if err := a.EnsureFDroidConfig(); err != nil {
			t.Fatalf("fdroid-init: %v", err)
		}
		for _, p := range []string{a.WS.FDroidConfig(), a.WS.FDroidRepo, a.WS.FDroidMeta} {
			if _, err := os.Stat(p); err != nil {
				t.Fatalf("expected %s: %v", p, err)
			}
		}
		t.Log("fdroid scaffold ok:", a.WS.FDroid)
	})

	t.Run("download_and_patch_one", func(t *testing.T) {
		if len(jobs) == 0 {
			t.Skip("no jobs (patches missing?)")
		}
		job := pickJob(jobs)
		t.Logf("trying %s %v", job.PackageID, job.Versions)

		c, cancel := context.WithTimeout(ctx, 15*time.Minute)
		defer cancel()

		versions := job.Versions
		if len(versions) == 0 {
			versions = []string{""}
		}
		versions = versions[:1]

		reg := download.DefaultRegistry()
		var lastErr error
		for _, ver := range versions {
			res, err := download.FetchFirst(c, reg, cfg.DownloaderOrder, download.Request{
				PackageID: job.PackageID,
				Version:   ver,
			}, a.WS.StockAPKs)
			if err != nil {
				lastErr = err
				t.Logf("download: %v", err)
				continue
			}
			out := filepath.Join(a.WS.PatchedAPKs, "e2e-patched.apk")
			patches, err := revanced.Patch(revanced.PatchOptions{
				CLIJar:                  a.WS.PatcherJAR(),
				PatchesRVP:              a.WS.PatchesRVP(),
				InputAPK:                res.Path,
				OutputAPK:               out,
				KeystorePath:            a.WS.KeystorePath,
				Blob:                    a.Blob,
				EnableChangePackageName: true,
			})
			if err != nil {
				lastErr = err
				t.Logf("patch: %v", err)
				continue
			}
			st, err := os.Stat(out)
			if err != nil || st.Size() < 1024 {
				t.Fatalf("patched apk: %v", err)
			}
			if err := fdroid.StageAPK(a.WS.FDroid, out); err != nil {
				t.Fatal(err)
			}
			if err := fdroid.WritePatchesMetadata(a.WS.FDroid, job.PackageID+".revanced", patches); err != nil {
				t.Fatal(err)
			}
			t.Logf("patched ok (%d bytes)", st.Size())
			lastErr = nil
			break
		}
		if lastErr != nil {
			if strictE2E() {
				t.Fatalf("download/patch: %v", lastErr)
			}
			t.Skipf("download/patch unavailable: %v", lastErr)
		}
	})

	t.Run("fdroid_update", func(t *testing.T) {
		if _, err := exec.LookPath("fdroid"); err != nil {
			if strictE2E() {
				t.Fatal("fdroid not on PATH")
			}
			t.Skip("fdroid not installed")
		}
		if err := a.FDroidUpdate(true); err != nil {
			if strictE2E() {
				t.Fatalf("fdroid update: %v", err)
			}
			t.Skipf("fdroid update: %v", err)
		}
		matches, _ := filepath.Glob(filepath.Join(a.WS.FDroidRepo, "index*"))
		t.Logf("index artifacts: %v", matches)
	})
}

// TestE2E_CLI builds the binary and runs keys → fetch-tools → fdroid-init (and list-jobs if patches work).
func TestE2E_CLI(t *testing.T) {
	requireOnPath(t, "java", "keytool", "go")

	root := repoRoot(t)
	bin := filepath.Join(t.TempDir(), "revancedbot")
	build := exec.Command("go", "build", "-o", bin, "./cmd/revancedbot")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	ws := t.TempDir()
	gen := exec.Command(bin, "keys", "generate")
	var stdout, stderr bytes.Buffer
	gen.Stdout = &stdout
	gen.Stderr = &stderr
	if err := gen.Run(); err != nil {
		t.Fatalf("keys generate: %v\n%s", err, stderr.String())
	}
	blob := lastNonEmptyLine(stdout.String())

	env := append(os.Environ(),
		"REVANCEDBOT_SIGNING="+blob,
		"GITHUB_TOKEN="+firstEnv("GITHUB_TOKEN", "REVANCEDBOT_GITHUB_TOKEN"),
	)

	runOK := func(name string, args ...string) []byte {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
		t.Logf("%s ok (%d bytes output)", name, len(out))
		return out
	}

	runOK("keys validate", "keys", "validate", "--workspace", ws)
	// fetch-tools may fail on patches DMCA — CLI path should still get the jar.
	cmd := exec.Command(bin, "fetch-tools", "--workspace", ws)
	cmd.Env = env
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Accept partial: if jar exists, continue.
		if _, statErr := os.Stat(filepath.Join(ws, "tools", "revanced-cli.jar")); statErr != nil {
			t.Fatalf("fetch-tools: %v\n%s", err, out)
		}
		t.Logf("fetch-tools partial failure (cli jar present): %v\n%s", err, truncate(string(out), 500))
	} else {
		t.Log("fetch-tools ok")
	}

	// If local patches provided, copy in for list-jobs
	if local := os.Getenv("REVANCEDBOT_PATCHES_FILE"); local != "" {
		_ = copyFile(local, filepath.Join(ws, "tools", "patches.rvp"))
	}

	if _, err := os.Stat(filepath.Join(ws, "tools", "patches.rvp")); err == nil {
		runOK("list-jobs", "list-jobs", "--workspace", ws)
	} else {
		t.Log("skip list-jobs (no patches.rvp)")
	}

	runOK("fdroid-init", "fdroid-init", "--workspace", ws)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func pickJob(jobs []revanced.Job) revanced.Job {
	if p := os.Getenv("REVANCEDBOT_E2E_PACKAGE"); p != "" {
		for _, j := range jobs {
			if j.PackageID == p {
				return j
			}
		}
	}
	avoid := []string{"youtube", "googlevoice", "photos", "magazines"}
	for _, j := range jobs {
		low := strings.ToLower(j.PackageID)
		bad := false
		for _, a := range avoid {
			if strings.Contains(low, a) {
				bad = true
				break
			}
		}
		if !bad {
			return j
		}
	}
	return jobs[0]
}

func requireOnPath(t *testing.T, bins ...string) {
	t.Helper()
	for _, b := range bins {
		if _, err := exec.LookPath(b); err != nil {
			t.Skipf("%s not on PATH", b)
		}
	}
}

func strictE2E() bool {
	v := strings.ToLower(os.Getenv("REVANCEDBOT_E2E_STRICT"))
	return v == "1" || v == "true" || v == "yes"
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
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

func lastNonEmptyLine(s string) string {
	var last string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			last = line
		}
	}
	return last
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
