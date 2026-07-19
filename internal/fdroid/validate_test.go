package fdroid

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeAndValidateJSON(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "good.json"), []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "bad.json"), []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := SanitizeJSONTree(repo)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("removed %d want 1", n)
	}
	if err := ValidateJSONTree(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "bad.json")); !os.IsNotExist(err) {
		t.Fatal("bad.json should be gone")
	}
}

func TestValidateStageAfterUpdate(t *testing.T) {
	stage := t.TempDir()
	if err := EnsureLayout(stage); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "config.yml"), []byte("repo_name: t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateStageAfterUpdate(stage); err == nil {
		t.Fatal("expected error without index")
	}
	if err := os.WriteFile(filepath.Join(stage, "repo", "index-v1.json"), []byte(`{"apps":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateStageAfterUpdate(stage); err != nil {
		t.Fatal(err)
	}
}

func TestSeedStageSkipsCorruptLive(t *testing.T) {
	live := t.TempDir()
	stage := t.TempDir()
	if err := os.MkdirAll(filepath.Join(live, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(live, "metadata"), 0o755); err != nil {
		t.Fatal(err)
	}
	// corrupt json — sanitize removes it, then seed should succeed with empty-ish repo
	if err := os.WriteFile(filepath.Join(live, "repo", "broken.json"), []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "repo", "app.apk"), []byte("apk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SeedStage(stage, live); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(stage, "repo", "app.apk")); err != nil {
		t.Fatal("apk should be seeded", err)
	}
	if _, err := os.Stat(filepath.Join(stage, "repo", "broken.json")); !os.IsNotExist(err) {
		t.Fatal("broken json must not be seeded")
	}
}
