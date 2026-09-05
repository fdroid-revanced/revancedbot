package fdroid

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasew/revancedbot/internal/signing"
	"github.com/lucasew/revancedbot/internal/workspace"
)

func TestWriteConfig_keystoreBounds(t *testing.T) {
	cache := t.TempDir()
	repo := t.TempDir()
	cfg := filepath.Join(cache, "fdroid", "config.yml")
	blob := &signing.Blob{Alias: "a", StorePass: "s", KeyPass: "k"}
	good := workspace.KeystoreFile(cache)

	if err := WriteConfig(cfg, RepoMeta{}, cache, repo, good, blob); err != nil {
		t.Fatalf("good keystore: %v", err)
	}
	raw, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "keystore: "+good) {
		t.Fatalf("config missing keystore path:\n%s", raw)
	}

	tests := []struct {
		name string
		ks   string
	}{
		{name: "relative", ks: "keystore.jks"},
		{name: "under repo", ks: filepath.Join(repo, "keystore.jks")},
		{name: "outside both", ks: filepath.Join(t.TempDir(), "keystore.jks")},
		{name: "repo via dots", ks: filepath.Join(cache, "signing", "..", "..", filepath.Base(repo), "keystore.jks")},
		{name: "has-prefix sibling", ks: filepath.Join(cache+"-extra", "signing", "keystore.jks")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := WriteConfig(cfg, RepoMeta{}, cache, repo, tt.ks, blob); err == nil {
				t.Fatalf("keystore %s: expected error", tt.ks)
			}
		})
	}
}

func TestWriteConfig_keystoreSymlinkIntoRepo(t *testing.T) {
	cache := t.TempDir()
	repo := t.TempDir()
	cfg := filepath.Join(cache, "fdroid", "config.yml")
	blob := &signing.Blob{Alias: "a", StorePass: "s", KeyPass: "k"}

	signingDir := filepath.Join(cache, "signing")
	if err := os.MkdirAll(signingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(signingDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(repo, signingDir); err != nil {
		t.Skip(err)
	}
	ks := filepath.Join(signingDir, "keystore.jks")
	if !filepath.IsAbs(ks) {
		t.Fatal("expected abs path")
	}
	if err := WriteConfig(cfg, RepoMeta{}, cache, repo, ks, blob); err == nil {
		t.Fatal("keystore symlink into repo must be refused")
	}
}

func TestWriteConfig_repoInsideCacheStillBlocksRepoKeystore(t *testing.T) {
	cache := t.TempDir()
	repo := filepath.Join(cache, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(cache, "fdroid", "config.yml")
	blob := &signing.Blob{Alias: "a", StorePass: "s", KeyPass: "k"}

	if err := WriteConfig(cfg, RepoMeta{}, cache, repo, workspace.KeystoreFile(cache), blob); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfig(cfg, RepoMeta{}, cache, repo, filepath.Join(repo, "keystore.jks"), blob); err == nil {
		t.Fatal("keystore under repo must be refused even when repo is inside cache")
	}
}

func TestWriteConfig_requiresCacheAndRepo(t *testing.T) {
	dir := t.TempDir()
	blob := &signing.Blob{Alias: "a"}
	ks := workspace.KeystoreFile(dir)
	cfg := filepath.Join(dir, "config.yml")
	if err := WriteConfig(cfg, RepoMeta{}, "", dir, ks, blob); err == nil {
		t.Fatal("expected error for empty cache")
	}
	if err := WriteConfig(cfg, RepoMeta{}, dir, "", ks, blob); err == nil {
		t.Fatal("expected error for empty repo")
	}
	if err := WriteConfig(cfg, RepoMeta{}, "", "", ks, blob); !errors.Is(err, ErrBase) {
		t.Fatalf("err = %v, want wrap ErrBase", err)
	}
}
