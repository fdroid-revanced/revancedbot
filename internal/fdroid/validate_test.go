package fdroid

import (
	"errors"
	"io/fs"
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
	if _, err := os.Stat(filepath.Join(repo, "bad.json")); !errors.Is(err, fs.ErrNotExist) {
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

func TestValidateStageLayout(t *testing.T) {
	stage := t.TempDir()
	if err := ValidateStageLayout(stage); err == nil {
		t.Fatal("expected error without repo/metadata")
	}
	if err := EnsureLayout(stage); err != nil {
		t.Fatal(err)
	}
	if err := ValidateStageLayout(stage); err == nil {
		t.Fatal("expected error without config.yml")
	}
	if err := os.WriteFile(filepath.Join(stage, "config.yml"), []byte("repo_name: t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateStageLayout(stage); err != nil {
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
	broken := []byte(`{`)
	if err := os.WriteFile(filepath.Join(live, "repo", "broken.json"), broken, 0o644); err != nil {
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
	if _, err := os.Stat(filepath.Join(stage, "repo", "broken.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("broken json must not be seeded")
	}
	got, err := os.ReadFile(filepath.Join(live, "repo", "broken.json"))
	if err != nil {
		t.Fatal("live broken.json must remain", err)
	}
	if string(got) != string(broken) {
		t.Fatalf("live broken.json changed: %q", got)
	}
}

func TestSeedStageLeavesLiveUntouched(t *testing.T) {
	live := t.TempDir()
	stage := t.TempDir()
	if err := os.MkdirAll(filepath.Join(live, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(live, "metadata"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(live, ".repo.old"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"config.yml":           []byte("repo_name: live\n"),
		"repo/index-v1.json":   []byte(`{"apps":[]}`),
		"repo/broken.json":     []byte(`{`),
		"repo/app.apk":         []byte("apk"),
		"metadata/pkg.yml":     []byte("x: 1\n"),
		".repo.old/stale.json": []byte(`{`),
	}
	for rel, data := range files {
		if err := os.WriteFile(filepath.Join(live, rel), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := SeedStage(stage, live); err != nil {
		t.Fatal(err)
	}
	for rel, want := range files {
		got, err := os.ReadFile(filepath.Join(live, rel))
		if err != nil {
			t.Fatalf("live %s must remain: %v", rel, err)
		}
		if string(got) != string(want) {
			t.Fatalf("live %s changed: %q", rel, got)
		}
	}
	if _, err := os.Stat(filepath.Join(stage, "repo", "app.apk")); err != nil {
		t.Fatal("apk should be seeded", err)
	}
	if _, err := os.Stat(filepath.Join(stage, "repo", "index-v1.json")); err != nil {
		t.Fatal("valid json should be seeded", err)
	}
	if _, err := os.Stat(filepath.Join(stage, "repo", "broken.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("broken json must not be seeded")
	}
	if _, err := os.Stat(filepath.Join(stage, "config.yml")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("live config.yml must not be copied to stage")
	}
}

func TestSeedStageSkipsInvalidLayout(t *testing.T) {
	live := t.TempDir()
	stage := t.TempDir()
	if err := os.WriteFile(filepath.Join(live, "repo"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SeedStage(stage, live); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(live, "repo"))
	if err != nil {
		t.Fatal("live repo file must remain", err)
	}
	if string(got) != "not a dir" {
		t.Fatalf("live repo file changed: %q", got)
	}
	entries, err := os.ReadDir(filepath.Join(stage, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid live must not be seeded, got %d entries", len(entries))
	}
}
