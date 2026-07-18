package fdroid

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/lucasew/revancedbot/internal/apksign"
	"github.com/lucasew/revancedbot/internal/signing"
)

// RepoMeta is branding for config.yml (from revancedbot.yaml).
type RepoMeta struct {
	Name        string
	URL         string
	Description string
}

// Env names the bot sets for the fdroid subprocess (passwords not written to disk).
const (
	EnvKeystorePass = "REVANCEDBOT_KEYSTORE_PASS"
	EnvKeyPass      = "REVANCEDBOT_KEY_PASS"
)

// WriteConfig writes gitignored REPO/config.yml.
// keystorePath must be absolute under CACHE. Passwords use {env: …} syntax.
func WriteConfig(path string, meta RepoMeta, keystoreAbs string, blob *signing.Blob) error {
	if meta.Name == "" {
		meta.Name = "ReVanced F-Droid Repo"
	}
	if meta.URL == "" {
		meta.URL = "https://example.invalid/fdroid/repo"
	}
	if !filepath.IsAbs(keystoreAbs) {
		return fmt.Errorf("keystore path must be absolute: %s", keystoreAbs)
	}

	sdkLine := ""
	if home := os.Getenv("ANDROID_HOME"); home != "" {
		sdkLine = "sdk_path: " + home + "\n"
	} else if home := os.Getenv("ANDROID_SDK_ROOT"); home != "" {
		sdkLine = "sdk_path: " + home + "\n"
	}

	const tmpl = `{{.SDK}}repo_url: {{.URL}}
repo_name: {{.Name}}
repo_description: >-
  {{.Description}}

repo_keyalias: {{.Alias}}
keystore: {{.Keystore}}
keystorepass: {env: {{.EnvStore}}}
keypass: {env: {{.EnvKey}}}
keydname: CN=revancedbot, OU=F-Droid, O=revancedbot, C=US
`
	type data struct {
		SDK         string
		URL         string
		Name        string
		Description string
		Alias       string
		Keystore    string
		EnvStore    string
		EnvKey      string
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	t := template.Must(template.New("cfg").Parse(tmpl))
	return t.Execute(f, data{
		SDK:         sdkLine,
		URL:         meta.URL,
		Name:        meta.Name,
		Description: indentDesc(meta.Description),
		Alias:       blob.Alias,
		Keystore:    keystoreAbs,
		EnvStore:    EnvKeystorePass,
		EnvKey:      EnvKeyPass,
	})
}

func indentDesc(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "ReVanced-patched applications."
	}
	return strings.ReplaceAll(s, "\n", "\n  ")
}

// EnsureLayout creates repo/ and metadata/ under the F-Droid root.
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

// StageAPK copies a patched APK into REPO/repo/.
func StageAPK(fdroidRoot, apkPath string) error {
	dest := filepath.Join(fdroidRoot, "repo", filepath.Base(apkPath))
	in, err := os.ReadFile(apkPath)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, in, 0o644)
}

// WritePatchesMetadata writes/updates metadata YAML with patches footer.
func WritePatchesMetadata(fdroidRoot, packageID string, patches []string) error {
	path := filepath.Join(fdroidRoot, "metadata", packageID+".yml")
	var b strings.Builder
	fmt.Fprintf(&b, "Categories:\n  - Multimedia\n")
	fmt.Fprintf(&b, "License: Unknown\n")
	fmt.Fprintf(&b, "AuthorName: ReVanced (patched)\n")
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

// Update runs `fdroid update` in the F-Droid REPO root with signing env from blob.
func Update(fdroidRoot string, blob *signing.Blob, createMetadata bool) error {
	args := []string{"update", "--pretty", "--delete-unknown"}
	if createMetadata {
		args = append(args, "-c")
	}
	// Shims so apksigner works when SDK scripts have a broken #!/bin/bash.
	shimDir := filepath.Join(os.TempDir(), "revancedbot-shims")
	if d, err := apksign.EnsureShims(shimDir); err == nil {
		_ = os.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	cmd := exec.Command("fdroid", args...)
	cmd.Dir = fdroidRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	env := append(os.Environ(),
		EnvKeystorePass+"="+blob.StorePass,
		EnvKeyPass+"="+blob.KeyPass,
	)
	if d, err := apksign.EnsureShims(shimDir); err == nil {
		env = append(env, "PATH="+d+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("fdroid update: %w", err)
	}
	return nil
}
