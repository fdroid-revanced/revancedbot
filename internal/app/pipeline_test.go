package app

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasew/revancedbot/internal/config"
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

func testApp(t *testing.T) *App {
	t.Helper()
	a, err := New(&config.Config{Repo: t.TempDir(), Cache: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return a
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
