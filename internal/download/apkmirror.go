package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var apkmirrorBase = "https://www.apkmirror.com"

// APKMirror scrapes apkmirror.com (HTTP + HTML, no browser/CDP).
// Flow: search by package → release page → best variant → download/?key= → download.php.
type APKMirror struct {
	Client *http.Client
}

func (a *APKMirror) ID() string { return "apkmirror" }

func (a *APKMirror) client(ctx context.Context) *http.Client {
	if a.Client != nil {
		return a.Client
	}
	return httpClientJar(ctx)
}

func (a *APKMirror) Fetch(ctx context.Context, req Request, destDir string) (*Result, error) {
	if strings.TrimSpace(req.PackageID) == "" {
		return nil, fmt.Errorf("package id required")
	}
	cl := a.client(ctx)

	releasePath, err := a.findRelease(ctx, cl, req)
	if err != nil {
		return nil, err
	}
	variantPath, err := a.findVariant(ctx, cl, releasePath, req.PackageID)
	if err != nil {
		return nil, err
	}
	dlURL, err := a.resolveDownloadURL(ctx, cl, variantPath)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return nil, err
	}
	a.setHeaders(httpReq, apkmirrorBase+variantPath)

	resp, err := cl.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download HTTP %s for %s", resp.Status, dlURL)
	}

	path := filepath.Join(destDir, stockFileName(req.PackageID, req.Version))
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
		URL:      dlURL,
		SHA256:   sha,
	}, nil
}

func (a *APKMirror) setHeaders(req *http.Request, referer string) {
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}

func (a *APKMirror) getHTML(ctx context.Context, cl *http.Client, pageURL, referer string) (string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}
	a.setHeaders(httpReq, referer)
	resp, err := cl.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %s for %s", resp.Status, pageURL)
	}
	// Cap HTML so a runaway page cannot fill memory.
	const maxHTML = 8 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTML))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// releaseHrefRe matches /apk/{dev}/{app}/{slug}-release/ links.
var releaseHrefRe = regexp.MustCompile(`href="(/apk/[^"#?]+-release/)"`)

// variantHrefRe matches variant pages under a release.
var variantHrefRe = regexp.MustCompile(`href="(/apk/[^"#?]+-apk-download/)"`)

// downloadKeyHrefRe matches the intermediate download/?key= link on a variant page.
var downloadKeyHrefRe = regexp.MustCompile(`href="([^"]+/download/\?key=[a-f0-9]+)"`)

// downloadPHPRe matches the final download.php link.
var downloadPHPRe = regexp.MustCompile(`href="(/wp-content/themes/APKMirror/download\.php\?id=\d+&key=[a-f0-9]+)"`)

func (a *APKMirror) findRelease(ctx context.Context, cl *http.Client, req Request) (string, error) {
	// Search by package id; results are release rows for matching apps.
	q := url.Values{}
	q.Set("post_type", "app_release")
	q.Set("searchtype", "apk")
	q.Set("s", req.PackageID)
	searchURL := apkmirrorBase + "/?" + q.Encode()

	html, err := a.getHTML(ctx, cl, searchURL, apkmirrorBase+"/")
	if err != nil {
		return "", fmt.Errorf("search: %w", err)
	}

	links := uniqueInOrder(releaseHrefRe.FindAllStringSubmatch(html, -1))
	if len(links) == 0 {
		return "", fmt.Errorf("no release results for %q", req.PackageID)
	}

	if req.Version != "" {
		want := versionSlug(req.Version)
		for _, link := range links {
			if strings.Contains(link, want) {
				return link, nil
			}
		}
		return "", fmt.Errorf("no release matching version %q for %s", req.Version, req.PackageID)
	}
	// Latest = first result row (site sorts newest first).
	return links[0], nil
}

func (a *APKMirror) findVariant(ctx context.Context, cl *http.Client, releasePath, packageID string) (string, error) {
	pageURL := apkmirrorBase + releasePath
	html, err := a.getHTML(ctx, cl, pageURL, apkmirrorBase+"/")
	if err != nil {
		return "", fmt.Errorf("release page: %w", err)
	}
	if packageID != "" && !strings.Contains(html, packageID) {
		// Soft check: package id usually appears on the page; warn via error if totally wrong app.
		// Some pages bury the id; still try if variants exist.
	}

	type cand struct {
		path  string
		score int
		ctx   string
	}
	var cands []cand
	seen := map[string]bool{}
	for _, m := range variantHrefRe.FindAllStringSubmatchIndex(html, -1) {
		// m: full start/end, group1 start/end
		if len(m) < 4 {
			continue
		}
		path := html[m[2]:m[3]]
		if seen[path] {
			continue
		}
		// Only variants that belong to this release.
		if !strings.HasPrefix(path, strings.TrimSuffix(releasePath, "/")+"/") {
			continue
		}
		seen[path] = true
		// Score from nearby text (ABI / DPI table cells).
		start := m[0] - 400
		if start < 0 {
			start = 0
		}
		end := m[1] + 200
		if end > len(html) {
			end = len(html)
		}
		window := strings.ToLower(stripTags(html[start:end]))
		cands = append(cands, cand{path: path, score: scoreVariant(window, path), ctx: window})
	}
	if len(cands) == 0 {
		return "", fmt.Errorf("no APK variants on %s", releasePath)
	}
	sort.SliceStable(cands, func(i, j int) bool {
		return cands[i].score > cands[j].score
	})
	// Prefer positive scores (APK + universal-ish); still take best even if all low.
	return cands[0].path, nil
}

func scoreVariant(window, path string) int {
	s := 0
	lowPath := strings.ToLower(path)
	// Prefer plain APK pages over bundle/APKM naming in the slug.
	if strings.Contains(lowPath, "bundle") || strings.Contains(window, " apkm ") || strings.Contains(window, "bundle") {
		s -= 50
	}
	if strings.Contains(window, "universal") {
		s += 30
	}
	if strings.Contains(window, "nodpi") {
		s += 20
	}
	if strings.Contains(window, "arm64-v8a") && !strings.Contains(window, "universal") {
		s += 5 // better than nothing, worse than universal
	}
	if strings.Contains(window, "armeabi") && !strings.Contains(window, "universal") {
		s += 3
	}
	if strings.Contains(window, "x86") && !strings.Contains(window, "universal") {
		s += 1
	}
	// Prefer fewer hyphenated variant suffixes (…-2-android-apk-download is often a second ABI).
	if strings.Contains(lowPath, "-2-android-apk-download") || strings.Contains(lowPath, "-3-android-apk-download") {
		s -= 5
	}
	return s
}

func (a *APKMirror) resolveDownloadURL(ctx context.Context, cl *http.Client, variantPath string) (string, error) {
	variantURL := apkmirrorBase + variantPath
	html, err := a.getHTML(ctx, cl, variantURL, apkmirrorBase+"/")
	if err != nil {
		return "", fmt.Errorf("variant page: %w", err)
	}

	keyHref := firstGroup(downloadKeyHrefRe, html)
	if keyHref == "" {
		// Some pages may embed download.php directly.
		if php := firstGroup(downloadPHPRe, html); php != "" {
			return apkmirrorBase + php, nil
		}
		return "", fmt.Errorf("no download key on variant %s", variantPath)
	}
	keyURL := absAPKM(keyHref)

	html2, err := a.getHTML(ctx, cl, keyURL, variantURL)
	if err != nil {
		return "", fmt.Errorf("download page: %w", err)
	}
	php := firstGroup(downloadPHPRe, html2)
	if php == "" {
		// id="download-link" may use &amp;
		php = firstGroup(regexp.MustCompile(`id="download-link"[^>]*href="([^"]+)"`), html2)
		if php == "" {
			php = firstGroup(regexp.MustCompile(`href="([^"]*download\.php\?[^"]+)"`), html2)
		}
	}
	if php == "" {
		return "", fmt.Errorf("no download.php on %s", keyURL)
	}
	php = strings.ReplaceAll(php, "&amp;", "&")
	return absAPKM(php), nil
}

func absAPKM(href string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	if !strings.HasPrefix(href, "/") {
		href = "/" + href
	}
	return apkmirrorBase + href
}

func firstGroup(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func uniqueInOrder(matches [][]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		p := m[1]
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// versionSlug turns "3.3.6" into a path fragment like "3-3-6" used in APKMirror URLs.
func versionSlug(ver string) string {
	ver = strings.TrimSpace(ver)
	ver = strings.ReplaceAll(ver, " ", "")
	return strings.ReplaceAll(ver, ".", "-")
}

var tagRe = regexp.MustCompile(`(?s)<[^>]*>`)

func stripTags(s string) string {
	return tagRe.ReplaceAllString(s, " ")
}
