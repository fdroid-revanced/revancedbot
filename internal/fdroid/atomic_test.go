package fdroid

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := WriteFileAtomic(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "hello" {
		t.Fatalf("got %q err %v", b, err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o644 {
		t.Fatalf("perm=%o want 644", st.Mode().Perm())
	}
}

func writeValidStage(t *testing.T, stage string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(stage, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(stage, "metadata"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "config.yml"), []byte("repo_name: t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "repo", "a.apk"), []byte("apk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "repo", "index-v1.json"), []byte(`{"apps":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "metadata", "pkg.yml"), []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPublish(t *testing.T) {
	stage := t.TempDir()
	live := t.TempDir()
	writeValidStage(t, stage)
	if err := os.MkdirAll(filepath.Join(live, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "repo", "old.apk"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Publish(PublishArgs{Stage: stage, Live: live}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(live, "config.yml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(live, "repo", "a.apk")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(live, "metadata", "pkg.yml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(live, "repo", "old.apk")); err == nil {
		t.Fatal("old apk should be gone after full dir replace")
	}
	auth := filepath.Join(live, "revancedbot.yaml")
	if err := os.WriteFile(auth, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Publish(PublishArgs{Stage: stage, Live: live}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(auth)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "ok\n" {
		t.Fatalf("revancedbot.yaml should not be touched: %q", b)
	}
}

func TestPublishLayoutOnly(t *testing.T) {
	stage := t.TempDir()
	live := t.TempDir()
	if err := EnsureLayout(stage); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "config.yml"), []byte("repo_name: t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Publish(PublishArgs{Stage: stage, Live: live}); err == nil {
		t.Fatal("expected index gate without layoutOnly")
	}
	if err := Publish(PublishArgs{Stage: stage, Live: live, LayoutOnly: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(live, "config.yml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(live, "repo")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(live, "metadata")); err != nil {
		t.Fatal(err)
	}
}

func TestPublishRequiresIndex(t *testing.T) {
	stage := t.TempDir()
	live := t.TempDir()
	if err := EnsureLayout(stage); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "config.yml"), []byte("repo_name: t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Publish(PublishArgs{Stage: stage, Live: live}); err == nil {
		t.Fatal("publish without index should fail")
	}
	if _, err := os.Stat(filepath.Join(live, "config.yml")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("live config.yml must stay unpublished when stage has no index")
	}
}

func TestRemovePublishLeftoversRestoresRepo(t *testing.T) {
	live := t.TempDir()
	old := filepath.Join(live, ".repo.old")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "index-v1.json"), []byte(`{"apps":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(live, "metadata"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RemovePublishLeftovers(live); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(live, "repo", "index-v1.json")); err != nil {
		t.Fatalf("repo should be restored from .repo.old: %v", err)
	}
	if _, err := os.Stat(old); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(".repo.old should be removed after restore")
	}
}

func TestRemovePublishLeftoversRestoresMetadata(t *testing.T) {
	live := t.TempDir()
	old := filepath.Join(live, ".metadata.old")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "pkg.yml"), []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(live, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RemovePublishLeftovers(live); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(live, "metadata", "pkg.yml")); err != nil {
		t.Fatalf("metadata should be restored from .metadata.old: %v", err)
	}
	if _, err := os.Stat(old); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(".metadata.old should be removed after restore")
	}
}

func TestRemovePublishLeftoversRollsBackPartialUnit(t *testing.T) {
	live := t.TempDir()
	if err := os.WriteFile(filepath.Join(live, "config.yml"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, ".config.yml.old"), []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(live, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "repo", "new.apk"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(live, ".repo.old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, ".repo.old", "old.apk"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(live, "metadata"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "metadata", "old.yml"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(live, ".metadata.new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, ".metadata.new", "new.yml"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RemovePublishLeftovers(live); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(live, "config.yml"))
	if err != nil || string(b) != "old\n" {
		t.Fatalf("config.yml = %q err %v; want previous publish", b, err)
	}
	if _, err := os.Stat(filepath.Join(live, "repo", "old.apk")); err != nil {
		t.Fatalf("repo should roll back to .repo.old: %v", err)
	}
	if _, err := os.Stat(filepath.Join(live, "repo", "new.apk")); err == nil {
		t.Fatal("partial new repo must not stay after unit rollback")
	}
	if _, err := os.Stat(filepath.Join(live, "metadata", "old.yml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(live, ".metadata.new")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(".metadata.new should be deleted")
	}
	if _, err := os.Stat(filepath.Join(live, ".repo.old")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(".repo.old should be deleted after restore")
	}
}
