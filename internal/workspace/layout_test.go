package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNew_refusesCacheInRepo(t *testing.T) {
	repo := t.TempDir()
	tests := []struct {
		name  string
		cache string
	}{
		{name: "equal", cache: repo},
		{name: "subdir", cache: filepath.Join(repo, "cache")},
		{name: "nested", cache: filepath.Join(repo, "a", "b")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(repo, tt.cache)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, ErrBase) {
				t.Fatalf("err = %v, want wrap ErrBase", err)
			}
		})
	}
}

func TestNew_refusesRelativeCacheInRepo(t *testing.T) {
	repo := t.TempDir()
	t.Chdir(repo)
	for _, cache := range []string{".", "cache", "./cache"} {
		if _, err := New(".", cache); err == nil {
			t.Fatalf("cache %q: expected error", cache)
		}
	}
}

func TestNew_allowsSiblingAndEmptyCache(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	cache := filepath.Join(base, "cache")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}

	l, err := New(repo, cache)
	if err != nil {
		t.Fatal(err)
	}
	if l.KeystorePath != KeystoreFile(l.Cache) {
		t.Fatalf("KeystorePath = %s, want %s", l.KeystorePath, KeystoreFile(l.Cache))
	}
	if Under(l.Repo, l.Cache) {
		t.Fatalf("cache %s is under repo %s", l.Cache, l.Repo)
	}

	tmp, err := New(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp.Cache) })
	if Under(tmp.Repo, tmp.Cache) {
		t.Fatalf("temp cache %s is under repo %s", tmp.Cache, tmp.Repo)
	}
}

func TestNew_hasPrefixTrap(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	cache := filepath.Join(base, "repo-cache")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := New(repo, cache); err != nil {
		t.Fatalf("sibling repo-cache must be allowed: %v", err)
	}
}

func TestNew_refusesCacheSymlinkIntoRepo(t *testing.T) {
	repo := t.TempDir()
	inner := filepath.Join(repo, "c")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(t.TempDir(), "cache")
	if err := os.Symlink(inner, cache); err != nil {
		t.Skip(err)
	}
	if _, err := New(repo, cache); err == nil {
		t.Fatal("symlink into repo must be refused")
	}
}
