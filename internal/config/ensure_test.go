package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureRepoDir_creates(t *testing.T) {
	base := t.TempDir()
	p := filepath.Join(base, "new-repo")
	got, err := EnsureRepoDir(p)
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(got)
	if err != nil || !st.IsDir() {
		t.Fatalf("expected dir %s: %v", got, err)
	}
	// idempotent
	got2, err := EnsureRepoDir(p)
	if err != nil || got2 != got {
		t.Fatalf("idempotent: %v %q %q", err, got, got2)
	}
}

func TestLoadFromRepo_emptyDownloaders(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "revancedbot.yaml"), []byte("repo_name: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFromRepo(repo, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.DownloaderOrder) != 0 {
		t.Fatalf("want empty downloaders so DefaultOrder applies, got %v", cfg.DownloaderOrder)
	}
}

func TestLoadFromRepo_missingYAMLUsesDefaults(t *testing.T) {
	repo := t.TempDir()
	cfg, err := LoadFromRepo(repo, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RepoName == "" {
		t.Fatal("expected default repo_name")
	}
}

func TestLoadFromRepoRequired_missingYAML(t *testing.T) {
	repo := t.TempDir()
	_, err := LoadFromRepoRequired(repo, t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error for missing authority doc")
	}
	if !errors.Is(err, ErrMissingAuthorityDoc) {
		t.Fatalf("want ErrMissingAuthorityDoc, got %v", err)
	}
}

func TestLoadFromRepoRequired_present(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "revancedbot.yaml"), []byte("repo_name: present\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFromRepoRequired(repo, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RepoName != "present" {
		t.Fatalf("RepoName=%q want present", cfg.RepoName)
	}
}

func TestLoadFromRepoRequired_configOverride(t *testing.T) {
	repo := t.TempDir()
	other := filepath.Join(t.TempDir(), "custom.yaml")
	if err := os.WriteFile(other, []byte("repo_name: custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFromRepoRequired(repo, t.TempDir(), other)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RepoName != "custom" {
		t.Fatalf("RepoName=%q want custom", cfg.RepoName)
	}
}

func TestEnsureRepoDir_rejectsFile(t *testing.T) {
	base := t.TempDir()
	p := filepath.Join(base, "file")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureRepoDir(p); err == nil {
		t.Fatal("expected error for file path")
	}
}
