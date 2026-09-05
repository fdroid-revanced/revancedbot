package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasew/revancedbot/internal/config"
	"github.com/lucasew/revancedbot/internal/revanced"
	"github.com/lucasew/workspaced/pkg/logging"
	"github.com/lucasew/workspaced/pkg/taskgroup"
)

func TestRunFull_allJobsSkippedPublishOK(t *testing.T) {
	restorePipelineHooks(t)
	a := testApp(t)
	ctx := testSession(t)

	var updated, published int
	stubPrelude()
	listJobs = func(*App) ([]revanced.Job, error) {
		return []revanced.Job{{PackageID: "com.example.app", Versions: []string{"1.0"}}}, nil
	}
	processPackage = func(*App, context.Context, revanced.Job) error {
		return ErrBase
	}
	fdroidUpdate = func(*App, context.Context, bool) error {
		updated++
		return nil
	}
	publishStage = func(*App) error {
		published++
		return nil
	}

	if err := a.RunFull(ctx); err != nil {
		t.Fatalf("RunFull: %v", err)
	}
	if updated != 1 {
		t.Fatalf("FDroidUpdate calls = %d; want 1", updated)
	}
	if published != 1 {
		t.Fatalf("PublishStage calls = %d; want 1", published)
	}
}

func TestRunSmoke_zeroSuccessNoPublish(t *testing.T) {
	restorePipelineHooks(t)
	a := testApp(t)
	ctx := testSession(t)

	var updated, published int
	stubPrelude()
	listJobs = func(*App) ([]revanced.Job, error) {
		return []revanced.Job{{PackageID: "com.example.app", Versions: []string{"1.0"}}}, nil
	}
	processPackage = func(*App, context.Context, revanced.Job) error {
		return ErrBase
	}
	fdroidUpdate = func(*App, context.Context, bool) error {
		updated++
		return nil
	}
	publishStage = func(*App) error {
		published++
		return nil
	}

	n, err := a.RunSmoke(ctx, 1)
	if err == nil || !errors.Is(err, ErrBase) {
		t.Fatalf("RunSmoke: zero successes must error (ErrBase), got %v", err)
	}
	if n != 0 {
		t.Fatalf("ok = %d; want 0", n)
	}
	if updated != 0 {
		t.Fatalf("FDroidUpdate called %d times; want 0", updated)
	}
	if published != 0 {
		t.Fatalf("PublishStage called %d times; want 0", published)
	}
}

func stubPrelude() {
	checkTools = func() error { return nil }
	loadSigning = func(*App) error { return nil }
	prepareStage = func(*App) error { return nil }
	fetchTools = func(*App, context.Context) error { return nil }
}

func restorePipelineHooks(t *testing.T) {
	t.Helper()
	c, l, p, f, j, pr, u, pub := checkTools, loadSigning, prepareStage, fetchTools, listJobs, processPackage, fdroidUpdate, publishStage
	t.Cleanup(func() {
		checkTools, loadSigning, prepareStage, fetchTools, listJobs, processPackage, fdroidUpdate, publishStage = c, l, p, f, j, pr, u, pub
	})
}

func testApp(t *testing.T) *App {
	t.Helper()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "revancedbot.yaml"), []byte("repo_name: t40\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFromRepo(repo, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func testSession(t *testing.T) context.Context {
	t.Helper()
	_, ctx := taskgroup.New(logging.NewWriterContext(t.Output()), taskgroup.DefaultLimits())
	return ctx
}
