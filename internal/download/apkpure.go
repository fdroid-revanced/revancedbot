package download

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
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
	if urls, err := a.listAPKURLs(ctx, cl, req.PackageID, ver); err != nil {
		errs = append(errs, err.Error())
	} else {
		for _, url := range urls {
			if looksLikeBundleURL(url) {
				errs = append(errs, "skip bundle URL")
				continue
			}
			res, err := a.fetchURL(ctx, cl, req.PackageID, label, url, destDir)
			if err == nil {
				return res, nil
			}
			errs = append(errs, err.Error())
		}
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

func (a *APKPure) listAPKURLs(ctx context.Context, cl *http.Client, pkg, version string) ([]string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apkpureListURL+pkg, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", browserUA)
	httpReq.Header.Set("Accept", "*/*")
	httpReq.Header.Set("x-cv", "3172501")
	httpReq.Header.Set("x-sv", "29")
	httpReq.Header.Set("x-abis", "arm64-v8a,armeabi-v7a,armeabi,x86,x86_64")
	httpReq.Header.Set("x-gp", "1")

	resp, err := cl.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	const maxListing = 8 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxListing))
	if err != nil {
		return nil, err
	}
	body, err = (rawHTTP{URL: apkpureListURL + pkg, Status: resp.StatusCode, Body: body}).finish(ctx, a.CDPURL)
	if err != nil {
		return nil, err
	}
	return apkpureURLsForVersion(body, version)
}

func apkpureURLForVersion(body []byte, want string) (string, error) {
	urls, err := apkpureURLsForVersion(body, want)
	if err != nil {
		return "", err
	}
	return urls[0], nil
}

type apkpureHit struct {
	Version string
	URL     string
}

func apkpureURLsForVersion(body []byte, want string) ([]string, error) {
	hits := apkpureHits(body)
	want = strings.TrimSpace(want)
	if strings.EqualFold(want, "latest") {
		want = ""
	}
	if want != "" {
		var filtered []apkpureHit
		for _, h := range hits {
			if h.Version == want {
				filtered = append(filtered, h)
			}
		}
		hits = filtered
	}
	urls := rankAPKPureHits(hits)
	if len(urls) == 0 {
		if want == "" {
			return nil, fmt.Errorf("no APK URL in apkpure listing: %w", ErrBase)
		}
		return nil, fmt.Errorf("version %q not in apkpure listing: %w", want, ErrBase)
	}
	return urls, nil
}

var apkpureGluedVerRe = regexp.MustCompile(`x\d+\.\d+(?:\.\d+)*:`)

func apkpureHits(body []byte) []apkpureHit {
	text := string(body)
	verRe := regexp.MustCompile(`[^0-9](\d+\.\d+(?:\.\d+)*)\:`)
	prefs := []string{apkpureCDN + "/b/APK/", apkpureCDN + "/b/XAPK/"}
	var hits []apkpureHit
	for pos := 0; pos < len(text); {
		j, pref := indexPrefixed(text[pos:], prefs)
		if j < 0 {
			break
		}
		abs := pos + j
		url := trimAPKPureURL(text[abs:])
		start := abs - 240
		if start < 0 {
			start = 0
		}
		ver := ""
		if vm := verRe.FindAllStringSubmatch(text[start:abs], -1); len(vm) > 0 {
			ver = vm[len(vm)-1][1]
		}
		hits = append(hits, apkpureHit{Version: ver, URL: url})
		pos = abs + len(pref)
	}
	return hits
}

func indexPrefixed(s string, prefs []string) (int, string) {
	best, pref := -1, ""
	for _, p := range prefs {
		i := strings.Index(s, p)
		if i >= 0 && (best < 0 || i < best) {
			best, pref = i, p
		}
	}
	return best, pref
}

func trimAPKPureURL(s string) string {
	s = trimHTTPURL(s)
	if s == "" {
		return s
	}
	for _, mark := range []string{"XAPKJ", "APKJ", "https://", "http://"} {
		if i := strings.Index(s[1:], mark); i >= 0 {
			s = s[:i+1]
		}
	}
	if loc := apkpureGluedVerRe.FindStringIndex(s); loc != nil {
		s = s[:loc[0]]
	}
	return s
}

func rankAPKPureHits(hits []apkpureHit) []string {
	type cand struct {
		url   string
		score int
	}
	var keys []string
	buckets := make(map[string][]cand)
	seen := make(map[string]bool)
	for _, h := range hits {
		if h.URL == "" || looksLikeBundleURL(h.URL) || seen[h.URL] {
			continue
		}
		seen[h.URL] = true
		k := h.Version
		if _, ok := buckets[k]; !ok {
			keys = append(keys, k)
		}
		buckets[k] = append(buckets[k], cand{url: h.URL, score: scoreAPKPureURL(h.URL)})
	}
	var out []string
	for _, k := range keys {
		group := buckets[k]
		sort.SliceStable(group, func(i, j int) bool {
			return group[i].score > group[j].score
		})
		for _, c := range group {
			out = append(out, c.url)
		}
	}
	return out
}

func scoreAPKPureURL(u string) int {
	label := apkpureLabel(u)
	s := 0
	if strings.Contains(u, "/b/APK/") && !strings.Contains(strings.ToLower(u), "/b/xapk/") {
		s += 50
	}
	if strings.Contains(label, "universal") {
		s += 30
	} else if strings.Contains(label, "arm64") && (strings.Contains(label, "armeabi") || strings.Contains(label, "armv7")) {
		s += 20
	}
	if strings.Contains(label, "nodpi") || strings.Contains(label, "anydpi") {
		s += 15
	}
	return s
}

func apkpureLabel(u string) string {
	low := strings.ToLower(u)
	i := strings.Index(u, "_fn=")
	if i < 0 {
		return low
	}
	rest := u[i+4:]
	if j := strings.IndexAny(rest, "&"); j >= 0 {
		rest = rest[:j]
	}
	raw := strings.ReplaceAll(strings.ReplaceAll(rest, "-", "+"), "_", "/")
	switch len(raw) % 4 {
	case 2:
		raw += "=="
	case 3:
		raw += "="
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return low
	}
	return low + " " + strings.ToLower(string(b))
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
