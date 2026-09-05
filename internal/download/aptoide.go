package download

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/lucasew/revancedbot/internal/osx"
	"github.com/lucasew/workspaced/pkg/logging"
)

// Overridable for tests.
var (
	aptoideVersionsURL = "https://ws75.aptoide.com/api/7/app/getVersions/"
	aptoideMetaURLs    = []string{
		"https://ws2.aptoide.com/api/7/app/getMeta/",
		"https://ws75.aptoide.com/api/7/app/getMeta/",
	}
)

const (
	aptoidePageSize = 40
	aptoideMaxPages = 5
)

// Aptoide uses the public ws75/ws2 JSON APIs (same flow as PyAPKDownloader):
//
//	getVersions(package) → pick TRUSTED row (md5/size/ABI) → getMeta(app_id) → file.path.
//
// Prefer this for rate-limit resilience vs HTML scrapers.
//
// Aurora Store / Play Store download is intentionally not implemented here:
// it needs Google auth tokens, device spoofing, and protobuf (see Aurora token
// dispenser). Aptoide covers many Play-mirrored packages without that stack.
type Aptoide struct {
	Client *http.Client
	CDPURL string
}

func (a *Aptoide) ID() string { return "aptoide" }

func (a *Aptoide) Fetch(ctx context.Context, req Request, destDir string) (*Result, error) {
	pkg := strings.TrimSpace(req.PackageID)
	if pkg == "" {
		return nil, fmt.Errorf("package id required: %w", ErrBase)
	}
	req.PackageID = pkg
	cl := orClient(a.Client, httpClient(ctx))

	cands, err := a.findVersions(ctx, cl, pkg, req.Version)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, cand := range cands {
		res, err := a.fetchCandidate(ctx, cl, req, destDir, cand)
		if err == nil {
			return res, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no aptoide APK candidate: %w", ErrBase)
	}
	return nil, lastErr
}

func (a *Aptoide) fetchCandidate(ctx context.Context, cl *http.Client, req Request, destDir string, cand aptoideVersion) (*Result, error) {
	dlURL, verName, err := a.metaDownload(ctx, cl, cand.ID)
	if err != nil {
		return nil, err
	}
	if looksLikeBundleURL(dlURL) {
		return nil, fmt.Errorf("aptoide returned bundle/XAPK URL, skipping: %w", ErrBase)
	}

	label := req.Version
	if label == "" {
		label = verName
	}
	if label == "" {
		label = cand.VerName
	}
	if label == "" {
		label = "latest"
	}

	if logging.ContextHasLogger(ctx) {
		logging.GetLogger(ctx).Info("aptoide pick",
			"package", req.PackageID,
			"version", cand.VerName,
			"vercode", cand.VerCode,
			"malware", cand.Malware,
			"md5", cand.MD5,
			"size", cand.Size,
		)
	}

	path := filepath.Join(destDir, stockFileName(req.PackageID, label))
	n, sha, err := saveAPK(ctx, cl, dlURL, path)
	if err != nil {
		return nil, err
	}
	if err := verifyAdvertised(path, n, cand); err != nil {
		osx.Remove(path)
		return nil, fmt.Errorf("aptoide: %w", err)
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
	for _, mark := range []string{".apkm", ".xapk", ".apks", "xapk", "bundle", "_apkmir"} {
		if strings.Contains(low, mark) {
			return true
		}
	}
	// "apkm" but not the APKMirror hostname.
	if strings.Contains(low, "apkm") && !strings.Contains(low, "apkmirror") {
		return true
	}
	// APKMirror split packages: ..._4arch_7dpi_24lang_....
	if strings.Contains(low, "arch_") && (strings.Contains(low, "dpi_") || strings.Contains(low, "lang_")) {
		return true
	}
	return false
}

type aptoideVersionsResp struct {
	Info struct {
		Status string `json:"status"`
	} `json:"info"`
	List []aptoideVersionItem `json:"list"`
}

type aptoideVersionItem struct {
	ID   int64           `json:"id"`
	Size int64           `json:"size"`
	OBB  json.RawMessage `json:"obb"`
	File struct {
		VerName  string `json:"vername"`
		VerCode  int64  `json:"vercode"`
		MD5      string `json:"md5sum"`
		Filesize int64  `json:"filesize"`
		Malware  struct {
			Rank string `json:"rank"`
		} `json:"malware"`
		Hardware struct {
			CPUs      []string        `json:"cpus"`
			Densities json.RawMessage `json:"densities"`
		} `json:"hardware"`
	} `json:"file"`
}

type aptoideVersion struct {
	ID        int64
	VerName   string
	VerCode   int64
	MD5       string
	Size      int64
	CPUs      []string
	Densities []byte
	HasOBB    bool
	Malware   string
}

func (it aptoideVersionItem) asVersion() aptoideVersion {
	size := it.File.Filesize
	if size == 0 {
		size = it.Size
	}
	return aptoideVersion{
		ID:        it.ID,
		VerName:   it.File.VerName,
		VerCode:   it.File.VerCode,
		MD5:       it.File.MD5,
		Size:      size,
		CPUs:      it.File.Hardware.CPUs,
		Densities: append([]byte(nil), it.File.Hardware.Densities...),
		HasOBB:    aptoideHasOBB(it.OBB),
		Malware:   it.File.Malware.Rank,
	}
}

func aptoideHasOBB(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s != "" && s != "null" && s != "{}"
}

func (a *Aptoide) findVersions(ctx context.Context, cl *http.Client, pkg, version string) ([]aptoideVersion, error) {
	var lastErr error
	var found []aptoideVersion
	latest := version == "" || strings.EqualFold(version, "latest")
	for page := range aptoideMaxPages {
		offset := page * aptoidePageSize
		list, err := a.listVersions(ctx, cl, pkg, aptoidePageSize, offset)
		if err != nil {
			return nil, err
		}
		if len(list) == 0 {
			break
		}
		ranked := rankAptoideVersions(list, version)
		if len(ranked) > 0 {
			found = append(found, ranked...)
			if latest {
				return rankAptoideVersions(found, version), nil
			}
			continue
		}
		if len(found) > 0 {
			break
		}
		lastErr = fmt.Errorf("getVersions: version %q not on page offset=%d: %w", version, offset, ErrBase)
		if latest {
			break
		}
	}
	if ranked := rankAptoideVersions(found, version); len(ranked) > 0 {
		return ranked, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("getVersions: no versions for %s: %w", pkg, ErrBase)
	}
	return nil, lastErr
}

func (a *Aptoide) listVersions(ctx context.Context, cl *http.Client, pkg string, limit, offset int) ([]aptoideVersion, error) {
	url := fmt.Sprintf("%spackage_name=%s/limit=%d/offset=%d", aptoideVersionsURL, pkg, limit, offset)
	body, err := a.getJSON(ctx, cl, url)
	if err != nil {
		return nil, fmt.Errorf("getVersions: %w", err)
	}
	var vr aptoideVersionsResp
	if err := json.Unmarshal(body, &vr); err != nil {
		return nil, fmt.Errorf("getVersions json: %w", err)
	}
	if !strings.EqualFold(vr.Info.Status, "OK") {
		return nil, fmt.Errorf("getVersions: no versions for %s (status=%s): %w", pkg, vr.Info.Status, ErrBase)
	}
	out := make([]aptoideVersion, 0, len(vr.List))
	for _, it := range vr.List {
		out = append(out, it.asVersion())
	}
	return out, nil
}

func pickAptoideVersion(list []aptoideVersion, want string) (aptoideVersion, bool) {
	ranked := rankAptoideVersions(list, want)
	if len(ranked) == 0 {
		return aptoideVersion{}, false
	}
	return ranked[0], true
}

func rankAptoideVersions(list []aptoideVersion, want string) []aptoideVersion {
	want = strings.TrimSpace(want)
	if strings.EqualFold(want, "latest") {
		want = ""
	}
	var ok []aptoideVersion
	for _, it := range list {
		if !aptoideAccept(it) {
			continue
		}
		if want != "" && !aptoideVersionFits(want, it) {
			continue
		}
		ok = append(ok, it)
	}
	if len(ok) == 0 {
		return nil
	}
	if want != "" {
		sort.SliceStable(ok, func(i, j int) bool {
			return scoreAptoide(ok[i]) > scoreAptoide(ok[j])
		})
		return ok
	}
	var keys []string
	buckets := make(map[string][]aptoideVersion)
	for _, it := range ok {
		k := aptoideVersionKey(it)
		if _, seen := buckets[k]; !seen {
			keys = append(keys, k)
		}
		buckets[k] = append(buckets[k], it)
	}
	var out []aptoideVersion
	for _, k := range keys {
		group := buckets[k]
		sort.SliceStable(group, func(i, j int) bool {
			return scoreAptoide(group[i]) > scoreAptoide(group[j])
		})
		out = append(out, group...)
	}
	return out
}

func aptoideVersionKey(it aptoideVersion) string {
	if it.VerName != "" {
		return it.VerName
	}
	return strconv.FormatInt(it.VerCode, 10)
}

func scoreAptoide(it aptoideVersion) int {
	s := 0
	switch {
	case aptoideUniversal(it):
		s += 30
	case len(it.CPUs) >= 2:
		s += 20
	}
	if aptoideBroadDPI(it) {
		s += 10
	}
	return s
}

func aptoideBroadDPI(it aptoideVersion) bool {
	d := bytes.TrimSpace(it.Densities)
	return len(d) == 0 || string(d) == "null" || string(d) == "[]"
}

func aptoideAccept(it aptoideVersion) bool {
	if it.ID == 0 || it.HasOBB {
		return false
	}
	rank := strings.ToUpper(strings.TrimSpace(it.Malware))
	if rank != "" && rank != "TRUSTED" {
		return false
	}
	return true
}

func aptoideUniversal(it aptoideVersion) bool {
	var arm64, armv7 bool
	for _, c := range it.CPUs {
		switch strings.ToLower(c) {
		case "arm64-v8a", "arm64-v8a-hwasan":
			arm64 = true
		case "armeabi-v7a", "armeabi":
			armv7 = true
		}
	}
	return arm64 && armv7
}

func aptoideVersionFits(want string, it aptoideVersion) bool {
	if it.VerName == want {
		return true
	}
	if isAllDigits(want) && strconv.FormatInt(it.VerCode, 10) == want {
		return true
	}
	return strings.HasPrefix(it.VerName, want+".") || strings.HasPrefix(it.VerName, want)
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
	for _, base := range aptoideMetaURLs {
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
			err = fmt.Errorf("getMeta status %s: %w", mr.Info.Status, ErrBase)
			continue
		}
		dlURL = mr.Data.File.Path
		if dlURL == "" {
			dlURL = mr.Data.File.PathAlt
		}
		if dlURL == "" {
			err = fmt.Errorf("getMeta: empty download path for app_id=%d: %w", appID, ErrBase)
			continue
		}
		return dlURL, mr.Data.File.VerName, nil
	}
	if err == nil {
		err = fmt.Errorf("getMeta failed for app_id=%d: %w", appID, ErrBase)
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
	const maxJSON = 4 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJSON))
	if err != nil {
		return nil, err
	}
	return (rawHTTP{URL: url, Status: resp.StatusCode, Body: body}).finish(ctx, a.CDPURL)
}
