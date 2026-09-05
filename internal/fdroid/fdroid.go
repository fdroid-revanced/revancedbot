package fdroid

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/lucasew/revancedbot/internal/apksign"
	"github.com/lucasew/revancedbot/internal/signing"
	"github.com/lucasew/revancedbot/internal/workspace"
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

// WriteConfig writes stage config.yml (under CACHE).
// keystoreAbs must resolve under cache and must not resolve under repo.
func WriteConfig(path string, meta RepoMeta, cache, repo, keystoreAbs string, blob *signing.Blob) error {
	if meta.Name == "" {
		meta.Name = "ReVanced F-Droid Repo"
	}
	if meta.URL == "" {
		meta.URL = "https://example.invalid/fdroid/repo"
	}
	if cache == "" || repo == "" {
		return fmt.Errorf("cache and repo are required: %w", ErrBase)
	}
	if !filepath.IsAbs(keystoreAbs) {
		return fmt.Errorf("keystore path must be absolute: %s: %w", keystoreAbs, ErrBase)
	}
	ks, err := workspace.Resolve(keystoreAbs)
	if err != nil {
		return fmt.Errorf("keystore path %s: %w", keystoreAbs, err)
	}
	if !workspace.Under(cache, ks) {
		return fmt.Errorf("keystore path must resolve under cache: %s: %w", keystoreAbs, ErrBase)
	}
	if workspace.Under(repo, ks) {
		return fmt.Errorf("keystore path must not resolve under repo: %s: %w", keystoreAbs, ErrBase)
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
	var b strings.Builder
	t := template.Must(template.New("cfg").Parse(tmpl))
	if err := t.Execute(&b, data{
		SDK:         sdkLine,
		URL:         meta.URL,
		Name:        meta.Name,
		Description: indentDesc(meta.Description),
		Alias:       blob.Alias,
		Keystore:    keystoreAbs,
		EnvStore:    EnvKeystorePass,
		EnvKey:      EnvKeyPass,
	}); err != nil {
		return err
	}
	return WriteFileAtomic(path, []byte(b.String()), 0o600)
}

func indentDesc(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "ReVanced-patched applications."
	}
	return strings.ReplaceAll(s, "\n", "\n  ")
}

// EnsureLayout creates repo/ and metadata/ under a root (stage or live).
func EnsureLayout(root string) error {
	for _, d := range []string{
		filepath.Join(root, "repo"),
		filepath.Join(root, "metadata"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// SeedStage copies existing live REPO repo/ and metadata/ into stage (for history).
// If live is missing or fails structure validation, starts an empty stage (regen).
// Live {repo,metadata,config.yml} are not written; corrupt JSON is stripped on the stage copy only.
func SeedStage(stageRoot, liveRepo string) error {
	if err := os.RemoveAll(stageRoot); err != nil {
		return err
	}
	if err := EnsureLayout(stageRoot); err != nil {
		return err
	}
	if err := ValidateLiveForSeed(liveRepo); err != nil {
		// Outside happy path: do not copy garbage; empty stage (fdroid update will regen indexes).
		return nil
	}
	for _, name := range []string{"repo", "metadata"} {
		src := filepath.Join(liveRepo, name)
		dst := filepath.Join(stageRoot, name)
		st, err := os.Stat(src)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return err
		}
		if !st.IsDir() {
			continue
		}
		if err := copyDir(src, dst); err != nil {
			return fmt.Errorf("seed %s: %w", name, err)
		}
	}
	// Strip corrupt JSON on the stage copy only (INV-01: live is unchanged until Publish).
	if _, err := SanitizeJSONTree(filepath.Join(stageRoot, "repo")); err != nil {
		return err
	}
	if err := ValidateJSONTree(filepath.Join(stageRoot, "repo")); err != nil {
		return fmt.Errorf("seed produced invalid structure (fix live repo or wipe repo/): %w", err)
	}
	return nil
}

// StageAPK copies a patched APK into stage/repo/ (atomic write).
func StageAPK(stageRoot, apkPath string) error {
	dest := filepath.Join(stageRoot, "repo", filepath.Base(apkPath))
	in, err := os.ReadFile(apkPath)
	if err == nil {
		return WriteFileAtomic(dest, in, 0o644)
	}
	return err
}

// WritePatchesMetadata writes metadata YAML into stage/metadata/.
func WritePatchesMetadata(stageRoot, packageID string, patches []string) error {
	path := filepath.Join(stageRoot, "metadata", packageID+".yml")
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
	return WriteFileAtomic(path, []byte(b.String()), 0o644)
}

func sanitizeYAML(s string) string {
	return strings.ReplaceAll(s, "\n", " ")
}

// Update runs `fdroid update` in the stage root (CACHE), not live REPO.
func Update(stageRoot string, blob *signing.Blob, createMetadata bool, shimDir string) error {
	args := []string{"update", "--pretty", "--delete-unknown"}
	if createMetadata {
		args = append(args, "-c")
	}
	if shimDir == "" {
		shimDir = filepath.Join(os.TempDir(), "revancedbot-shims")
	}
	pathEnv := os.Getenv("PATH")
	if d, err := apksign.EnsureShims(shimDir); err == nil {
		pathEnv = d + string(os.PathListSeparator) + pathEnv
	}
	cmd := exec.Command("fdroid", args...)
	cmd.Dir = stageRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		EnvKeystorePass+"="+blob.StorePass,
		EnvKeyPass+"="+blob.KeyPass,
		"PATH="+pathEnv,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("fdroid update: %w", err)
	}
	if err := ValidateStageAfterUpdate(stageRoot); err != nil {
		return err
	}
	return nil
}
