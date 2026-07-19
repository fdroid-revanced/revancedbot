package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

// Layout splits durable F-Droid REPO from disposable CACHE.
// REPO is only updated via atomic publish from CACHE/fdroid (stage).
// Tools, stock APKs, keystore, patch work, and the F-Droid build tree live in CACHE.
type Layout struct {
	Repo  string
	Cache string

	// Cache subdirs
	Tools        string
	StockAPKs    string
	Signing      string
	KeystorePath string
	Work         string
	Stage        string // CACHE/fdroid — full F-Droid tree being built
	Shims        string // CACHE/shims — apksigner wrappers for fdroid

	// Stage subdirs (writes during run; not live REPO)
	StageRepo string
	StageMeta string
}

// New builds a layout. cache may be empty → MkdirTemp.
func New(repo, cache string) (*Layout, error) {
	repoAbs, err := filepath.Abs(repo)
	if err != nil {
		return nil, err
	}
	var cacheAbs string
	if cache == "" {
		cacheAbs, err = os.MkdirTemp("", "revancedbot-cache-*")
		if err != nil {
			return nil, fmt.Errorf("mkdtemp cache: %w", err)
		}
	} else {
		cacheAbs, err = filepath.Abs(cache)
		if err != nil {
			return nil, err
		}
	}
	stage := filepath.Join(cacheAbs, "fdroid")
	l := &Layout{
		Repo:         repoAbs,
		Cache:        cacheAbs,
		Tools:        filepath.Join(cacheAbs, "tools"),
		StockAPKs:    filepath.Join(cacheAbs, "stock"),
		Signing:      filepath.Join(cacheAbs, "signing"),
		KeystorePath: filepath.Join(cacheAbs, "signing", "keystore.jks"),
		Work:         filepath.Join(cacheAbs, "work"),
		Stage:        stage,
		Shims:        filepath.Join(cacheAbs, "shims"),
		StageRepo:    filepath.Join(stage, "repo"),
		StageMeta:    filepath.Join(stage, "metadata"),
	}
	return l, nil
}

// Ensure creates CACHE directories (including empty stage). Does not create live REPO/repo.
func (l *Layout) Ensure() error {
	for _, d := range []string{l.Tools, l.StockAPKs, l.Signing, l.Work, l.Stage, l.StageRepo, l.StageMeta, l.Shims} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}

func (l *Layout) PatcherJAR() string { return filepath.Join(l.Tools, "revanced-cli.jar") }
func (l *Layout) PatchesRVP() string { return filepath.Join(l.Tools, "patches.rvp") }

// StageConfig is config.yml under CACHE stage (fdroid update cwd).
func (l *Layout) StageConfig() string { return filepath.Join(l.Stage, "config.yml") }

// LiveConfig is the published config.yml in REPO (after atomic publish).
func (l *Layout) LiveConfig() string { return filepath.Join(l.Repo, "config.yml") }

// FDroidConfig is the stage config path (writes go here during a run).
func (l *Layout) FDroidConfig() string { return l.StageConfig() }

func (l *Layout) BotConfig() string { return filepath.Join(l.Repo, "revancedbot.yaml") }

// LiveRepoDir is REPO/repo after publish.
func (l *Layout) LiveRepoDir() string { return filepath.Join(l.Repo, "repo") }

// LiveMetaDir is REPO/metadata after publish.
func (l *Layout) LiveMetaDir() string { return filepath.Join(l.Repo, "metadata") }

// CacheHit reports whether path exists and is large enough to count as a hit.
func CacheHit(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Size() > 1024
}

// StockAPKPath is the naive cache path for a stock APK.
func (l *Layout) StockAPKPath(packageID, version string) string {
	if version == "" {
		version = "latest"
	}
	return filepath.Join(l.StockAPKs, sanitize(packageID)+"_"+sanitize(version)+".apk")
}

func sanitize(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-' {
			b = append(b, c)
		} else {
			b = append(b, '_')
		}
	}
	return string(b)
}
