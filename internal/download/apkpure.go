package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lucasew/revancedbot/internal/osx"
)

// Overridable for tests.
var (
	apkpureListURL = "https://api.pureapk.com/m/v3/cms/app_version?hl=en-US&package_name="
	apkpureCDN     = "https://download.pureapk.com"
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

// APKPure resolves a version from api.pureapk.com (same listing as apkeep),
// then downloads the APK URL. Falls back to d.apkpure.com URL shapes.
type APKPure struct {
	Client *http.Client
	CDPURL string
}

func (a *APKPure) ID() string { return "apkpure" }

func (a *APKPure) Fetch(ctx context.Context, req Request, destDir string) (*Result, error) {
	ver := req.Version
	label := ver
	if label == "" {
		label = "latest"
	}
	cl := orClient(a.Client, httpClient(ctx))

	var errs []string
	if url, err := a.listAPKURL(ctx, cl, req.PackageID, ver); err != nil {
		errs = append(errs, err.Error())
	} else if url != "" {
		res, err := a.fetchURL(ctx, cl, req.PackageID, label, url, destDir)
		if err == nil {
			return res, nil
		}
		errs = append(errs, err.Error())
	}

	// Legacy guessed URLs if the listing is missing or the CDN URL 404s.
	urls := []string{
		fmt.Sprintf("https://d.apkpure.com/b/APK/%s?version=%s", req.PackageID, label),
	}
	if ver != "" && isAllDigits(ver) {
		urls = append(urls, fmt.Sprintf("https://d.apkpure.com/b/APK/%s?versionCode=%s", req.PackageID, ver))
	}

	for _, url := range urls {
		res, err := a.fetchURL(ctx, cl, req.PackageID, label, url, destDir)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		return res, nil
	}
	if len(errs) == 0 {
		return nil, fmt.Errorf("no apkpure URL tried: %w", ErrBase)
	}
	return nil, fmt.Errorf("%s: %w", strings.Join(errs, "; "), ErrBase)
}

func (a *APKPure) listAPKURL(ctx context.Context, cl *http.Client, pkg, version string) (string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apkpureListURL+pkg, nil)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("User-Agent", browserUA)
	httpReq.Header.Set("Accept", "*/*")
	httpReq.Header.Set("x-cv", "3172501")
	httpReq.Header.Set("x-sv", "29")
	httpReq.Header.Set("x-abis", "arm64-v8a,armeabi-v7a,armeabi,x86,x86_64")
	httpReq.Header.Set("x-gp", "1")

	resp, err := cl.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	const maxListing = 8 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxListing))
	if err != nil {
		return "", err
	}
	body, err = (rawHTTP{URL: apkpureListURL + pkg, Status: resp.StatusCode, Body: body}).finish(ctx, a.CDPURL)
	if err != nil {
		return "", err
	}
	return apkpureURLForVersion(body, version)
}

func apkpureURLForVersion(body []byte, want string) (string, error) {
	text := string(body)
	want = strings.TrimSpace(want)
	urlPat := `APKJ.{0,4}(` + regexp.QuoteMeta(apkpureCDN) + `/b/APK/[^\x00-\x1f]+)`
	if want == "" || strings.EqualFold(want, "latest") {
		re, err := regexp.Compile(urlPat)
		if err != nil {
			return "", err
		}
		if m := re.FindStringSubmatch(text); len(m) == 2 {
			return trimHTTPURL(m[1]), nil
		}
		return "", fmt.Errorf("no APK URL in apkpure listing: %w", ErrBase)
	}
	re, err := regexp.Compile(`(?s)[^0-9]` + regexp.QuoteMeta(want) + `:(.+?)` + urlPat)
	if err != nil {
		return "", err
	}
	m := re.FindStringSubmatch(text)
	if len(m) < 3 {
		return "", fmt.Errorf("version %q not in apkpure listing: %w", want, ErrBase)
	}
	return trimHTTPURL(m[2]), nil
}

func trimHTTPURL(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] < 32 || s[i] == ' ' {
			return s[:i]
		}
	}
	return s
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func (a *APKPure) fetchURL(ctx context.Context, cl *http.Client, pkg, ver, url, destDir string) (*Result, error) {
	path := filepath.Join(destDir, stockFileName(pkg, ver))
	_, sha, err := saveAPK(ctx, cl, url, path)
	if err != nil {
		osx.Remove(path)
		return nil, err
	}
	return &Result{
		Path:     path,
		SourceID: a.ID(),
		URL:      url,
		SHA256:   sha,
	}, nil
}
