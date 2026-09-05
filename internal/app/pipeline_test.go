package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasew/revancedbot/internal/config"
	"github.com/lucasew/revancedbot/internal/download"
	"github.com/lucasew/revancedbot/internal/revanced"
	"github.com/lucasew/revancedbot/internal/workspace"
	"github.com/lucasew/workspaced/pkg/logging"
)

func TestRequirePatchedPackageID(t *testing.T) {
	if err := requirePatchedPackageID("com.example.app.revanced", "com.example.app"); err != nil {
		t.Fatal(err)
	}
	err := requirePatchedPackageID("com.example.app", "com.example.app")
	if err == nil {
		t.Fatal("expected mismatch")
	}
	if errors.Is(err, errStopWalk) {
		t.Fatal("identity mismatch must fail this version, not stop the Job")
	}
	if err := requirePatchedPackageID("", "com.example.app"); err == nil {
		t.Fatal("expected empty package mismatch")
	}
	if err := requirePatchedPackageID("com.example.app.revanced", "com.other.app"); err == nil {
		t.Fatal("expected other stock mismatch")
	}
}

func TestTryVersions_firstOK(t *testing.T) {
	var tried []string
	err := tryVersions([]string{"1", "2"}, func(ver string) error {
		tried = append(tried, ver)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tried) != 1 || tried[0] != "1" {
		t.Fatalf("tried %v", tried)
	}
}

func TestTryVersions_secondOK(t *testing.T) {
	var tried []string
	err := tryVersions([]string{"1", "2"}, func(ver string) error {
		tried = append(tried, ver)
		if ver == "1" {
			return fmt.Errorf("version miss: %w", ErrBase)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tried) != 2 {
		t.Fatalf("tried %v", tried)
	}
}

func TestTryVersions_stopWalk(t *testing.T) {
	var tried []string
	err := tryVersions([]string{"1", "2"}, func(ver string) error {
		tried = append(tried, ver)
		return fmt.Errorf("meta: %w: %w", ErrBase, errStopWalk)
	})
	if !errors.Is(err, errStopWalk) {
		t.Fatalf("want errStopWalk, got %v", err)
	}
	if len(tried) != 1 || tried[0] != "1" {
		t.Fatalf("must not walk next version after stage: tried %v", tried)
	}
}

func TestTryVersions_allFail(t *testing.T) {
	err := tryVersions([]string{"1", "2"}, func(string) error {
		return fmt.Errorf("version miss: %w", ErrBase)
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTryVersions_empty(t *testing.T) {
	var n int
	err := tryVersions(nil, func(ver string) error {
		n++
		if ver != "" {
			t.Fatalf("ver %q", ver)
		}
		return nil
	})
	if err != nil || n != 1 {
		t.Fatalf("err %v n %d", err, n)
	}
}

func TestStagePatched_ok(t *testing.T) {
	stage := t.TempDir()
	if err := os.MkdirAll(filepath.Join(stage, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(stage, "metadata"), 0o755); err != nil {
		t.Fatal(err)
	}
	apk := filepath.Join(t.TempDir(), "com.example.app_1.0_revanced.apk")
	if err := os.WriteFile(apk, []byte("apk"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{WS: &workspace.Layout{Stage: stage}}
	if err := a.stagePatched(apk, "com.example.app.revanced", []string{"p1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(stage, "repo", "com.example.app_1.0_revanced.apk")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(stage, "metadata", "com.example.app.revanced.yml")); err != nil {
		t.Fatal(err)
	}
}

func TestStagePatched_metadataFailRemovesAPK(t *testing.T) {
	stage := t.TempDir()
	if err := os.MkdirAll(filepath.Join(stage, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "metadata"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	apk := filepath.Join(t.TempDir(), "com.example.app_1.0_revanced.apk")
	if err := os.WriteFile(apk, []byte("apk"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{WS: &workspace.Layout{Stage: stage}}
	err := a.stagePatched(apk, "com.example.app.revanced", nil)
	if err == nil || !errors.Is(err, errStopWalk) {
		t.Fatalf("want errStopWalk, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(stage, "repo", "com.example.app_1.0_revanced.apk")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("staged apk should be removed: %v", err)
	}
}

func TestFetchTools_PatchesFileOverridesCacheHit(t *testing.T) {
	a := testApp(t)
	oldCLI := writeFat(t, a.WS.PatcherJAR(), "cli-old")
	writeFat(t, a.WS.PatchesRVP(), "rvp-old")
	src := filepath.Join(t.TempDir(), "override.rvp")
	want := writeFat(t, src, "rvp-new")
	t.Setenv("REVANCEDBOT_PATCHES_FILE", src)
	t.Setenv("REVANCEDBOT_PATCHES_URL", "")

	if err := a.FetchTools(logging.NewWriterContext(t.Output())); err != nil {
		t.Fatalf("FetchTools: %v", err)
	}
	assertFile(t, a.WS.PatchesRVP(), want)
	assertFile(t, a.WS.PatcherJAR(), oldCLI)
}

func TestFetchTools_PatchesURLOverridesCacheHit(t *testing.T) {
	a := testApp(t)
	oldCLI := writeFat(t, a.WS.PatcherJAR(), "cli-old")
	writeFat(t, a.WS.PatchesRVP(), "rvp-old")
	want := bytes.Repeat([]byte("rvp-url"), 256)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(want); err != nil {
			t.Errorf("write override body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("REVANCEDBOT_PATCHES_FILE", "")
	t.Setenv("REVANCEDBOT_PATCHES_URL", srv.URL+"/patches.rvp")

	if err := a.FetchTools(logging.NewWriterContext(t.Output())); err != nil {
		t.Fatalf("FetchTools: %v", err)
	}
	assertFile(t, a.WS.PatchesRVP(), want)
	assertFile(t, a.WS.PatcherJAR(), oldCLI)
}

func TestFetchTools_CacheHitWithoutOverride(t *testing.T) {
	a := testApp(t)
	cli := writeFat(t, a.WS.PatcherJAR(), "cli-hit")
	rvp := writeFat(t, a.WS.PatchesRVP(), "rvp-hit")
	t.Setenv("REVANCEDBOT_PATCHES_FILE", "")
	t.Setenv("REVANCEDBOT_PATCHES_URL", "")

	if err := a.FetchTools(logging.NewWriterContext(t.Output())); err != nil {
		t.Fatalf("FetchTools: %v", err)
	}
	assertFile(t, a.WS.PatcherJAR(), cli)
	assertFile(t, a.WS.PatchesRVP(), rvp)
}

func writeFat(t *testing.T, path, stamp string) []byte {
	t.Helper()
	b := bytes.Repeat([]byte(stamp), 256)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return b
}

func assertFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s: got %q want %q", path, clip(got), clip(want))
	}
}

func clip(b []byte) string {
	if len(b) > 24 {
		return string(b[:24]) + "…"
	}
	return string(b)
}

func TestStockRegistry_passesBrowserCDPURL(t *testing.T) {
	t.Parallel()
	const cdp = "ws://127.0.0.1:3000"
	a := &App{Cfg: &config.Config{BrowserCDPURL: cdp}}
	reg := a.stockRegistry()
	if got := reg["aptoide"].(*download.Aptoide).CDPURL; got != cdp {
		t.Fatalf("aptoide CDPURL=%q want %q", got, cdp)
	}
	if got := reg["apkpure"].(*download.APKPure).CDPURL; got != cdp {
		t.Fatalf("apkpure CDPURL=%q want %q", got, cdp)
	}
	if got := reg["apkmirror"].(*download.APKMirror).CDPURL; got != cdp {
		t.Fatalf("apkmirror CDPURL=%q want %q", got, cdp)
	}
}

func TestStockRegistry_emptyCDP(t *testing.T) {
	t.Parallel()
	a := &App{Cfg: &config.Config{}}
	reg := a.stockRegistry()
	if got := reg["aptoide"].(*download.Aptoide).CDPURL; got != "" {
		t.Fatalf("empty config CDPURL=%q", got)
	}
}

func TestRunSmoke_walksSameJobsAsRun(t *testing.T) {
	jobs := []revanced.Job{
		{PackageID: "com.google.android.youtube"},
		{PackageID: "com.google.android.apps.photos"},
		{PackageID: "com.example.app"},
	}
	tried := map[string]int{}
	ok, err := runSmoke(testSession(t), smokeRun{
		jobs:  append([]revanced.Job(nil), jobs...),
		maxOK: len(jobs),
		try: func(_ context.Context, job revanced.Job) error {
			tried[job.PackageID]++
			return nil
		},
		update:  func(context.Context) error { return nil },
		publish: func() error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok != len(jobs) {
		t.Fatalf("ok=%d want %d", ok, len(jobs))
	}
	for _, job := range jobs {
		if tried[job.PackageID] != 1 {
			t.Fatalf("job %s tries=%d want 1 (tried=%v)", job.PackageID, tried[job.PackageID], tried)
		}
	}
}

func TestRunSmoke_stopsAtMax(t *testing.T) {
	var n int
	ok, err := runSmoke(testSession(t), smokeRun{
		jobs: []revanced.Job{
			{PackageID: "com.google.android.youtube"},
			{PackageID: "com.example.app"},
			{PackageID: "com.other.app"},
		},
		maxOK: 1,
		try: func(_ context.Context, _ revanced.Job) error {
			n++
			return nil
		},
		update:  func(context.Context) error { return nil },
		publish: func() error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok != 1 || n != 1 {
		t.Fatalf("ok=%d tries=%d want 1, 1", ok, n)
	}
}

func TestRunSmoke_zeroSuccessDoesNotPublish(t *testing.T) {
	var updated, published bool
	tried := map[string]int{}
	ok, err := runSmoke(testSession(t), smokeRun{
		jobs: []revanced.Job{
			{PackageID: "com.google.android.youtube"},
			{PackageID: "com.google.android.apps.photos"},
		},
		maxOK: 1,
		try: func(_ context.Context, job revanced.Job) error {
			tried[job.PackageID]++
			return fmt.Errorf("skip: %w", ErrBase)
		},
		update: func(context.Context) error {
			updated = true
			return nil
		},
		publish: func() error {
			published = true
			return nil
		},
	})
	if ok != 0 {
		t.Fatalf("ok=%d want 0", ok)
	}
	if err == nil || !errors.Is(err, ErrBase) {
		t.Fatalf("err=%v want wrap of ErrBase", err)
	}
	if updated || published {
		t.Fatalf("updated=%v published=%v want neither", updated, published)
	}
	if tried["com.google.android.youtube"] != 1 || tried["com.google.android.apps.photos"] != 1 {
		t.Fatalf("tried=%v want youtube and photos once each", tried)
	}
}
