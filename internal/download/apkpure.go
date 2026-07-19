package download

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

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
	return defaultHTTPClient()
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
	httpReq.Header.Set("User-Agent", browserUA)

	resp, err := a.client().Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s for %s", resp.Status, url)
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
