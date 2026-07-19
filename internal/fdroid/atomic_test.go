package fdroid

import (
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
}

func TestPublish(t *testing.T) {
	stage := t.TempDir()
	live := t.TempDir()
	// stage content
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
	if err := os.WriteFile(filepath.Join(stage, "metadata", "pkg.yml"), []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// pre-existing live content should be replaced
	if err := os.MkdirAll(filepath.Join(live, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "repo", "old.apk"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Publish(stage, live); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(live, "config.yml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(live, "repo", "a.apk")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(live, "repo", "old.apk")); err == nil {
		t.Fatal("old apk should be gone after full dir replace")
	}
	// authority file untouched if present
	auth := filepath.Join(live, "revancedbot.yaml")
	if err := os.WriteFile(auth, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Publish(stage, live); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(auth)
	if string(b) != "ok\n" {
		t.Fatalf("revancedbot.yaml should not be touched: %q", b)
	}
}
