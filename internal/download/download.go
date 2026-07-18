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

// DefaultRegistry returns built-in downloaders.
func DefaultRegistry() Registry {
	return Registry{
		"apkpure": &APKPure{},
	}
}

// FetchFirst tries downloaders in order until one succeeds.
func FetchFirst(ctx context.Context, reg Registry, order []string, req Request, destDir string) (*Result, error) {
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
		return res, nil
	}
	return nil, fmt.Errorf("all downloaders failed: %s", stringsJoin(errs, "; "))
}

func stringsJoin(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for i := 1; i < len(ss); i++ {
		out += sep + ss[i]
	}
	return out
}

// APKPure downloads via the d.apkpure.com APK endpoint (HTTP, no browser).
// Prefers universal APK when the source serves one at this URL shape.
type APKPure struct {
	Client *http.Client
}

func (a *APKPure) ID() string { return "apkpure" }

func (a *APKPure) client() *http.Client {
	if a.Client != nil {
		return a.Client
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

func (a *APKPure) Fetch(ctx context.Context, req Request, destDir string) (*Result, error) {
	ver := req.Version
	if ver == "" {
		ver = "latest"
	}
	// Historical direct URL used by the prototype. May require browser fallback later.
	url := fmt.Sprintf("https://d.apkpure.com/b/APK/%s?version=%s", req.PackageID, ver)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; revancedbot/1.0)")

	resp, err := a.client().Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s for %s", resp.Status, url)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%s_%s.apk", sanitize(req.PackageID), sanitize(ver))
	path := filepath.Join(destDir, name)
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	h := sha256.New()
	w := io.MultiWriter(f, h)
	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return nil, err
	}
	if n < 1024 {
		_ = os.Remove(path)
		return nil, fmt.Errorf("download too small (%d bytes), likely not an APK", n)
	}
	// Reject HTML error pages that APK mirrors sometimes return.
	head := make([]byte, 4)
	if f2, err := os.Open(path); err == nil {
		_, _ = f2.Read(head)
		_ = f2.Close()
		if string(head) == "<!DO" || string(head) == "<htm" || string(head) == "<HTM" {
			_ = os.Remove(path)
			return nil, fmt.Errorf("download looks like HTML, not an APK")
		}
		if !(head[0] == 'P' && head[1] == 'K') {
			// APK is ZIP; allow if Android package magic via file later — still require ZIP
			_ = os.Remove(path)
			return nil, fmt.Errorf("download is not a ZIP/APK (magic %q)", head)
		}
	}

	return &Result{
		Path:     path,
		SourceID: a.ID(),
		URL:      url,
		SHA256:   hex.EncodeToString(h.Sum(nil)),
	}, nil
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
