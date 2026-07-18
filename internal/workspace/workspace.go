package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

// Layout holds well-known paths under the bot workspace.
type Layout struct {
	Root         string
	Tools        string
	StockAPKs    string
	PatchedAPKs  string
	Signing      string
	KeystorePath string
	FDroid       string
	FDroidRepo   string
	FDroidMeta   string
}

func New(root string) *Layout {
	fd := filepath.Join(root, "fdroid")
	return &Layout{
		Root:         root,
		Tools:        filepath.Join(root, "tools"),
		StockAPKs:    filepath.Join(root, "stock_apks"),
		PatchedAPKs:  filepath.Join(root, "patched_apks"),
		Signing:      filepath.Join(root, "signing"),
		KeystorePath: filepath.Join(root, "signing", "keystore.p12"),
		FDroid:       fd,
		FDroidRepo:   filepath.Join(fd, "repo"),
		FDroidMeta:   filepath.Join(fd, "metadata"),
	}
}

func (l *Layout) Ensure() error {
	for _, d := range []string{l.Root, l.Tools, l.StockAPKs, l.PatchedAPKs, l.Signing, l.FDroid, l.FDroidRepo, l.FDroidMeta} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}

func (l *Layout) PatcherJAR() string  { return filepath.Join(l.Tools, "revanced-cli.jar") }
func (l *Layout) PatchesRVP() string  { return filepath.Join(l.Tools, "patches.rvp") }
func (l *Layout) FDroidConfig() string { return filepath.Join(l.FDroid, "config.yml") }
