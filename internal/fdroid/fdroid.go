package fdroid

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/lucasew/revancedbot/internal/signing"
)

// RepoMeta is branding for config.yml.
type RepoMeta struct {
	Name        string
	URL         string
	Description string
}

// WriteConfig writes fdroidserver config.yml using the operator keystore.
func WriteConfig(path string, meta RepoMeta, ksPath string, blob *signing.Blob) error {
	if meta.Name == "" {
		meta.Name = "ReVanced F-Droid Repo"
	}
	if meta.URL == "" {
		meta.URL = "https://example.invalid/fdroid/repo"
	}
	const tmpl = `repo_url: {{.URL}}
repo_name: {{.Name}}
repo_description: >-
  {{.Description}}

repo_keyalias: {{.Alias}}
keystore: {{.Keystore}}
keystorepass: {{.StorePass}}
keypass: {{.KeyPass}}
keydname: CN=revancedbot, OU=F-Droid, O=revancedbot, C=US
`

	type data struct {
		URL         string
		Name        string
		Description string
		Alias       string
		Keystore    string
		StorePass   string
		KeyPass     string
	}
	// Prefer relative keystore path if possible
	ks := ksPath
	if rel, err := filepath.Rel(filepath.Dir(path), ksPath); err == nil {
		ks = rel
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	t := template.Must(template.New("cfg").Parse(tmpl))
	return t.Execute(f, data{
		URL:         meta.URL,
		Name:        meta.Name,
		Description: indentDesc(meta.Description),
		Alias:       blob.Alias,
		Keystore:    ks,
		StorePass:   blob.StorePass,
		KeyPass:     blob.KeyPass,
	})
}

func indentDesc(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "ReVanced-patched applications."
	}
	return strings.ReplaceAll(s, "\n", "\n  ")
}

// EnsureLayout creates repo/ and metadata/ directories.
func EnsureLayout(fdroidRoot string) error {
	for _, d := range []string{
		filepath.Join(fdroidRoot, "repo"),
		filepath.Join(fdroidRoot, "metadata"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// StageAPK copies a patched APK into repo/.
func StageAPK(fdroidRoot, apkPath string) error {
	dest := filepath.Join(fdroidRoot, "repo", filepath.Base(apkPath))
	in, err := os.ReadFile(apkPath)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, in, 0o644)
}

// WritePatchesMetadata appends/creates a minimal metadata YAML with patches footer.
// packageID should be the *published* id (with .revanced).
func WritePatchesMetadata(fdroidRoot, packageID string, patches []string) error {
	path := filepath.Join(fdroidRoot, "metadata", packageID+".yml")
	var b strings.Builder
	if existing, err := os.ReadFile(path); err == nil {
		b.Write(existing)
		if !strings.HasSuffix(string(existing), "\n") {
			b.WriteByte('\n')
		}
	} else {
		fmt.Fprintf(&b, "Categories:\n  - Multimedia\n")
		fmt.Fprintf(&b, "License: Unknown\n")
		fmt.Fprintf(&b, "AuthorName: ReVanced (patched)\n")
	}
	// Always rewrite Description end with patches list for this build.
	// Simple approach: set Description fully.
	fmt.Fprintf(&b, "Description: |\n")
	fmt.Fprintf(&b, "  ReVanced-patched build of %s.\n\n", strings.TrimSuffix(packageID, ".revanced"))
	fmt.Fprintf(&b, "  Patches applied:\n")
	if len(patches) == 0 {
		fmt.Fprintf(&b, "  - (see ReVanced defaults for this package)\n")
	} else {
		for _, p := range patches {
			fmt.Fprintf(&b, "  - %s\n", sanitizeYAML(p))
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func sanitizeYAML(s string) string {
	return strings.ReplaceAll(s, "\n", " ")
}

// Update runs `fdroid update` in the fdroid root (simple binary repo).
// createMetadata adds -c when true (first-time package metadata from APKs).
func Update(fdroidRoot string, createMetadata bool) error {
	args := []string{"update", "--pretty", "--delete-unknown"}
	if createMetadata {
		args = append(args, "-c")
	}
	cmd := exec.Command("fdroid", args...)
	cmd.Dir = fdroidRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("fdroid update: %w", err)
	}
	return nil
}
