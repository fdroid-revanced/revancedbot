package download

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Aptoide uses the public ws75/ws2 JSON APIs (same flow as PyAPKDownloader):
//
//	getVersions(package) → app id → getMeta(app_id) → file.path download.
//
// Prefer this for rate-limit resilience vs HTML scrapers.
//
// Aurora Store / Play Store download is intentionally not implemented here:
// it needs Google auth tokens, device spoofing, and protobuf (see Aurora token
// dispenser). Aptoide covers many Play-mirrored packages without that stack.
type Aptoide struct {
	Client *http.Client
}

func (a *Aptoide) ID() string { return "aptoide" }

func (a *Aptoide) client(ctx context.Context) *http.Client {
	if a.Client != nil {
		return a.Client
	}
	return httpClient(ctx)
}

func (a *Aptoide) Fetch(ctx context.Context, req Request, destDir string) (*Result, error) {
	pkg := strings.TrimSpace(req.PackageID)
	if pkg == "" {
		return nil, fmt.Errorf("package id required")
	}
	cl := a.client(ctx)

	appID, err := a.findAppID(ctx, cl, pkg, req.Version)
	if err != nil {
		return nil, err
	}
	dlURL, verName, err := a.metaDownload(ctx, cl, appID)
	if err != nil {
		return nil, err
	}
	if looksLikeBundleURL(dlURL) {
		return nil, fmt.Errorf("aptoide returned bundle/XAPK URL, skipping")
	}

	label := req.Version
	if label == "" {
		label = verName
	}
	if label == "" {
		label = "latest"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", browserUA)
	httpReq.Header.Set("Accept", "*/*")

	resp, err := cl.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download HTTP %s for %s", resp.Status, dlURL)
	}
	if looksLikeBundleResponse(resp) {
		return nil, fmt.Errorf("response looks like an APK bundle, not a single APK")
	}

	path := filepath.Join(destDir, stockFileName(pkg, label))
	n, sha, err := writeBody(path, resp.Body)
	if err != nil {
		return nil, err
	}
	if n < MinAPKBytes {
		_ = os.Remove(path)
		return nil, fmt.Errorf("download too small (%d bytes)", n)
	}
	if err := ValidateAPK(path); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return &Result{
		Path:     path,
		SourceID: a.ID(),
		URL:      dlURL,
		SHA256:   sha,
	}, nil
}

func looksLikeBundleURL(u string) bool {
	low := strings.ToLower(u)
	for _, mark := range []string{".apkm", ".xapk", ".apks", "apkm", "xapk", "apks", "bundle"} {
		if strings.Contains(low, mark) {
			return true
		}
	}
	return false
}

type aptoideVersionsResp struct {
	Info struct {
		Status string `json:"status"`
	} `json:"info"`
	List []struct {
		ID   int64 `json:"id"`
		File struct {
			VerName string `json:"vername"`
			VerCode int64  `json:"vercode"`
		} `json:"file"`
	} `json:"list"`
}

func (a *Aptoide) findAppID(ctx context.Context, cl *http.Client, pkg, version string) (int64, error) {
	// PyAPKDownloader: https://ws75.aptoide.com/api/7/app/getVersions/package_name=…/limit=…
	url := fmt.Sprintf("https://ws75.aptoide.com/api/7/app/getVersions/package_name=%s/limit=%d",
		pkg, 40)
	body, err := a.getJSON(ctx, cl, url)
	if err != nil {
		return 0, fmt.Errorf("getVersions: %w", err)
	}
	var vr aptoideVersionsResp
	if err := json.Unmarshal(body, &vr); err != nil {
		return 0, fmt.Errorf("getVersions json: %w", err)
	}
	if !strings.EqualFold(vr.Info.Status, "OK") || len(vr.List) == 0 {
		return 0, fmt.Errorf("getVersions: no versions for %s (status=%s)", pkg, vr.Info.Status)
	}
	want := strings.TrimSpace(version)
	if want == "" || strings.EqualFold(want, "latest") {
		return vr.List[0].ID, nil
	}
	for _, it := range vr.List {
		if it.File.VerName == want {
			return it.ID, nil
		}
	}
	// Soft match: version is a prefix (e.g. "3.3" vs "3.3.6").
	for _, it := range vr.List {
		if strings.HasPrefix(it.File.VerName, want+".") || strings.HasPrefix(it.File.VerName, want) {
			return it.ID, nil
		}
	}
	return 0, fmt.Errorf("getVersions: version %q not in first %d for %s", want, len(vr.List), pkg)
}

type aptoideMetaResp struct {
	Info struct {
		Status string `json:"status"`
	} `json:"info"`
	Data struct {
		File struct {
			VerName string `json:"vername"`
			Path    string `json:"path"`
			PathAlt string `json:"path_alt"`
		} `json:"file"`
	} `json:"data"`
}

func (a *Aptoide) metaDownload(ctx context.Context, cl *http.Client, appID int64) (dlURL, verName string, err error) {
	// PyAPKDownloader uses ws2 for getMeta; ws75 also works for some packages.
	for _, base := range []string{
		"https://ws2.aptoide.com/api/7/app/getMeta/",
		"https://ws75.aptoide.com/api/7/app/getMeta/",
	} {
		url := fmt.Sprintf("%sapp_id=%d", base, appID)
		body, e := a.getJSON(ctx, cl, url)
		if e != nil {
			err = e
			continue
		}
		var mr aptoideMetaResp
		if e := json.Unmarshal(body, &mr); e != nil {
			err = e
			continue
		}
		if !strings.EqualFold(mr.Info.Status, "OK") {
			err = fmt.Errorf("getMeta status %s", mr.Info.Status)
			continue
		}
		dlURL = mr.Data.File.Path
		if dlURL == "" {
			dlURL = mr.Data.File.PathAlt
		}
		if dlURL == "" {
			err = fmt.Errorf("getMeta: empty download path for app_id=%d", appID)
			continue
		}
		return dlURL, mr.Data.File.VerName, nil
	}
	if err == nil {
		err = fmt.Errorf("getMeta failed for app_id=%d", appID)
	}
	return "", "", err
}

func (a *Aptoide) getJSON(ctx context.Context, cl *http.Client, url string) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", browserUA)
	httpReq.Header.Set("Accept", "application/json")
	resp, err := cl.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s for %s", resp.Status, url)
	}
	const maxJSON = 4 << 20
	return io.ReadAll(io.LimitReader(resp.Body, maxJSON))
}
