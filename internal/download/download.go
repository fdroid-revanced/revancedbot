package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/lucasew/revancedbot/internal/netx"
	"github.com/lucasew/workspaced/pkg/logging"
)

// Request is a stock APK fetch request.
type Request struct {
	PackageID string
	Version   string // empty = latest
}

// Result is a successful download.
type Result struct {
	Path     string
	SourceID string
	URL      string
	SHA256   string
}

// Downloader fetches a stock APK for a job.
type Downloader interface {
	ID() string
	Fetch(ctx context.Context, req Request, destDir string) (*Result, error)
}

// Registry maps downloader ids to implementations.
type Registry map[string]Downloader

// DefaultOrder is the built-in fallback order when config omits downloaders.
var DefaultOrder = []string{"apkpure", "apkmirror"}

// DefaultRegistry returns built-in downloaders.
func DefaultRegistry() Registry {
	return Registry{
		"apkpure":   &APKPure{},
		"apkmirror": &APKMirror{},
	}
}

// FetchFirst tries downloaders in order until one succeeds and ValidateAPK passes.
// No per-source taskgroup Map — progress stays on the parent "apks" aggregate bar
// plus httpclient fetch bars for real network I/O.
func FetchFirst(ctx context.Context, reg Registry, order []string, req Request, destDir string) (*Result, error) {
	if len(order) == 0 {
		order = DefaultOrder
	}
	var errs []string
	for _, id := range order {
		res, err := tryDownloader(ctx, reg, id, req, destDir)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if logging.ContextHasLogger(ctx) {
			logging.GetLogger(ctx).Info("stock apk ok",
				"source", res.SourceID,
				"package", req.PackageID,
				"path", res.Path,
			)
		}
		return res, nil
	}
	if len(errs) == 0 {
		return nil, fmt.Errorf("all downloaders failed")
	}
	return nil, fmt.Errorf("all downloaders failed: %s", strings.Join(errs, "; "))
}

func tryDownloader(ctx context.Context, reg Registry, id string, req Request, destDir string) (*Result, error) {
	d, ok := reg[id]
	if !ok {
		return nil, fmt.Errorf("%s: unknown downloader", id)
	}
	res, err := d.Fetch(ctx, req, destDir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}
	if err := ValidateAPK(res.Path); err != nil {
		_ = os.Remove(res.Path)
		return nil, fmt.Errorf("%s: reject apk: %w", id, err)
	}
	if res.SHA256 == "" {
		sum, err := fileSHA256(res.Path)
		if err != nil {
			_ = os.Remove(res.Path)
			return nil, fmt.Errorf("%s: sha256: %w", id, err)
		}
		res.SHA256 = sum
	}
	return res, nil
}

// AcceptCached validates an existing stock cache file; on failure the path is removed.
func AcceptCached(path string) error {
	if err := ValidateAPK(path); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func httpClient(ctx context.Context) *http.Client {
	return netx.Client(ctx)
}

func httpClientJar(ctx context.Context) *http.Client {
	return netx.ClientWithJar(ctx)
}

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

func writeBody(path string, r io.Reader) (n int64, sha string, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha256.New()
	w := io.MultiWriter(f, h)
	n, err = io.Copy(w, r)
	if err != nil {
		_ = os.Remove(path)
		return n, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func stockFileName(packageID, version string) string {
	if version == "" {
		version = "latest"
	}
	return fmt.Sprintf("%s_%s.apk", sanitize(packageID), sanitize(version))
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
