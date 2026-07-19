package config

import (
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
