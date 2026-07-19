package download

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// looksLikeBundleResponse rejects CDN replies that are clearly split/bundle packages.
func looksLikeBundleResponse(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	cd := strings.ToLower(resp.Header.Get("Content-Disposition"))
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	for _, mark := range []string{".apkm", ".xapk", ".apks", "bundle"} {
		if strings.Contains(cd, mark) {
			return true
		}
	}
	// Unusual but seen on some mirrors.
	if strings.Contains(ct, "apkm") || strings.Contains(ct, "xapk") {
		return true
	}
	return false
}

// APKPure downloads via the d.apkpure.com APK endpoint (HTTP, no browser).
// Prefers universal APK when the source serves one at this URL shape.
type APKPure struct {
	Client *http.Client
}

func (a *APKPure) ID() string { return "apkpure" }

func (a *APKPure) client(ctx context.Context) *http.Client {
	if a.Client != nil {
		return a.Client
	}
	return httpClient(ctx)
}

func (a *APKPure) Fetch(ctx context.Context, req Request, destDir string) (*Result, error) {
	ver := req.Version
	if ver == "" {
		ver = "latest"
	}
	url := fmt.Sprintf("https://d.apkpure.com/b/APK/%s?version=%s", req.PackageID, ver)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", browserUA)

	resp, err := a.client(ctx).Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s for %s", resp.Status, url)
	}
	// /b/APK/ should be a single APK; reject bundle packaging if the CDN mislabels.
	if looksLikeBundleResponse(resp) {
		return nil, fmt.Errorf("response looks like an APK bundle/XAPK, not a single APK")
	}

	path := filepath.Join(destDir, stockFileName(req.PackageID, ver))
	n, sha, err := writeBody(path, resp.Body)
	if err != nil {
		return nil, err
	}
	if n < MinAPKBytes {
		_ = os.Remove(path)
		return nil, fmt.Errorf("download too small (%d bytes), likely not an APK", n)
	}
	if err := ValidateAPK(path); err != nil {
		_ = os.Remove(path)
		return nil, err
	}

	return &Result{
		Path:     path,
		SourceID: a.ID(),
		URL:      url,
		SHA256:   sha,
	}, nil
}
