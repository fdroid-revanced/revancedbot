package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnder(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(root, "a", "b")
	sibling := filepath.Join(base, "root-extra")
	parent := base

	if !Under(root, root) {
		t.Fatal("root should contain itself")
	}
	if !Under(root, inside) {
		t.Fatalf("Under(%s, %s) = false", root, inside)
	}
	if Under(root, sibling) {
		t.Fatalf("HasPrefix trap: %s must not be under %s", sibling, root)
	}
	if Under(root, parent) {
		t.Fatalf("parent %s must not be under %s", parent, root)
	}
}

func TestKeystoreFile(t *testing.T) {
	cache := t.TempDir()
	got := KeystoreFile(cache)
	want := filepath.Join(cache, "signing", "keystore.jks")
	if got != want {
		t.Fatalf("KeystoreFile(%s) = %s, want %s", cache, got, want)
	}
}

func TestResolve_symlinkPrefix(t *testing.T) {
	realDir := t.TempDir()
	linkParent := t.TempDir()
	link := filepath.Join(linkParent, "c")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skip(err)
	}
	got, err := Resolve(filepath.Join(link, "missing", "file"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(realDir)
	if err != nil {
		t.Fatal(err)
	}
	want = filepath.Join(want, "missing", "file")
	if got != want {
		t.Fatalf("Resolve = %s, want %s", got, want)
	}
}
