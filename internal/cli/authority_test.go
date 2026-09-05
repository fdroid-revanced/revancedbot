package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasew/revancedbot/internal/config"
)

func execRoot(t *testing.T, args ...string) error {
	t.Helper()
	cfgFile = ""
	cacheFlag = ""
	t.Cleanup(func() {
		if err := CloseSession(); err != nil {
			t.Errorf("CloseSession: %v", err)
		}
		cfgFile = ""
		cacheFlag = ""
	})
	root := NewRoot()
	root.SetArgs(args)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	err := root.Execute()
	if closeErr := CloseSession(); err == nil {
		err = closeErr
	}
	return err
}

func TestRefuseMissingAuthorityDoc(t *testing.T) {
	repo := t.TempDir()
	tests := []struct {
		name string
		args []string
	}{
		{name: "run", args: []string{"run", repo}},
		{name: "smoke", args: []string{"smoke", repo}},
		{name: "fdroid-init", args: []string{"fdroid-init", repo}},
		{name: "fdroid-update", args: []string{"fdroid-update", repo}},
		{name: "download", args: []string{"download", "--package", "com.example", repo}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := execRoot(t, tt.args...)
			if err == nil {
				t.Fatalf("%s: expected error for missing authority doc", tt.name)
			}
			if !errors.Is(err, config.ErrMissingAuthorityDoc) {
				t.Fatalf("%s: want ErrMissingAuthorityDoc, got %v", tt.name, err)
			}
		})
	}
}

func TestLoadApp_missingYAMLOptional(t *testing.T) {
	repo := t.TempDir()
	if _, err := loadApp(nil, []string{repo}); err != nil {
		t.Fatal(err)
	}
}

func TestLoadAppRequired_missingYAML(t *testing.T) {
	repo := t.TempDir()
	_, err := loadAppRequired(nil, []string{repo})
	if err == nil {
		t.Fatal("expected error for missing authority doc")
	}
	if !errors.Is(err, config.ErrMissingAuthorityDoc) {
		t.Fatalf("want ErrMissingAuthorityDoc, got %v", err)
	}
}

func TestLoadAppRequired_configOverride(t *testing.T) {
	repo := t.TempDir()
	doc := filepath.Join(t.TempDir(), "custom.yaml")
	if err := os.WriteFile(doc, []byte("repo_name: custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgFile = doc
	t.Cleanup(func() { cfgFile = "" })
	if _, err := loadAppRequired(nil, []string{repo}); err != nil {
		t.Fatal(err)
	}
}
