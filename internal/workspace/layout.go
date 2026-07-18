package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

// Layout splits durable F-Droid REPO from disposable CACHE.
// REPO never holds tools, stock APKs, keystore, or work temps.
type Layout struct {
	Repo  string
	Cache string

	// Cache subdirs
	Tools        string
	StockAPKs    string
	Signing      string
	KeystorePath string
	Work         string

	// Repo subdirs (F-Droid simple binary)
	FDroidRepo string
	FDroidMeta string
}

// New builds a layout. cache may be empty → MkdirTemp.
// If cache is created via MkdirTemp, the caller owns cleanup if desired.
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
	l := &Layout{
		Repo:         repoAbs,
		Cache:        cacheAbs,
		Tools:        filepath.Join(cacheAbs, "tools"),
		StockAPKs:    filepath.Join(cacheAbs, "stock"),
		Signing:      filepath.Join(cacheAbs, "signing"),
		KeystorePath: filepath.Join(cacheAbs, "signing", "keystore.jks"),
		Work:         filepath.Join(cacheAbs, "work"),
		FDroidRepo:   filepath.Join(repoAbs, "repo"),
		FDroidMeta:   filepath.Join(repoAbs, "metadata"),
	}
	return l, nil
}

// Ensure creates required directories (cache always; repo/metadata/repo always).
func (l *Layout) Ensure() error {
	for _, d := range []string{l.Tools, l.StockAPKs, l.Signing, l.Work, l.Repo, l.FDroidRepo, l.FDroidMeta} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}

func (l *Layout) PatcherJAR() string   { return filepath.Join(l.Tools, "revanced-cli.jar") }
func (l *Layout) PatchesRVP() string   { return filepath.Join(l.Tools, "patches.rvp") }
func (l *Layout) FDroidConfig() string { return filepath.Join(l.Repo, "config.yml") }
func (l *Layout) BotConfig() string    { return filepath.Join(l.Repo, "revancedbot.yaml") }

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
