package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasew/revancedbot/internal/workspace"
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
