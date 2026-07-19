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
	"time"
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

// FetchFirst tries downloaders in order until one succeeds and the file
// passes ValidateAPK. Failed partial files from a downloader are left for
// that downloader to clean up; FetchFirst removes the path if validation fails
// after a reported success.
func FetchFirst(ctx context.Context, reg Registry, order []string, req Request, destDir string) (*Result, error) {
	if len(order) == 0 {
		order = DefaultOrder
	}
	var errs []string
	for _, id := range order {
		d, ok := reg[id]
		if !ok {
			errs = append(errs, fmt.Sprintf("%s: unknown downloader", id))
			continue
		}
		res, err := d.Fetch(ctx, req, destDir)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		if err := ValidateAPK(res.Path); err != nil {
			_ = os.Remove(res.Path)
			errs = append(errs, fmt.Sprintf("%s: reject apk: %v", id, err))
			continue
		}
		if res.SHA256 == "" {
			sum, err := fileSHA256(res.Path)
			if err != nil {
				_ = os.Remove(res.Path)
				errs = append(errs, fmt.Sprintf("%s: sha256: %v", id, err))
				continue
			}
			res.SHA256 = sum
		}
		return res, nil
	}
	return nil, fmt.Errorf("all downloaders failed: %s", strings.Join(errs, "; "))
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

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Minute}
}

// browserUA is a desktop Chrome UA; many APK hosts reject library defaults.
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
