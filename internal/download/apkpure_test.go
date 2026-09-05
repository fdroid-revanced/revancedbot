package download

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPKPure_implementsDownloader(t *testing.T) {
	var _ Downloader = &APKPure{}
}

func TestApkpureURLForVersion(t *testing.T) {
	body := []byte("x3.3.6:(4d35d46444e950e98e1b908489374d7dea0f9a85" +
		"APKJ\x00https://download.pureapk.com/b/APK/AAA?k=1" +
		"x3.3.5:(4d35d46444e950e98e1b908489374d7dea0f9a85" +
		"APKJ\x00https://download.pureapk.com/b/APK/BBB?k=2")

	t.Parallel()
	got, err := apkpureURLForVersion(body, "3.3.6")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "https://download.pureapk.com/b/APK/AAA") {
		t.Fatalf("got %q", got)
	}
	got, err = apkpureURLForVersion(body, "3.3.5")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "https://download.pureapk.com/b/APK/BBB") {
		t.Fatalf("got %q", got)
	}
	got, err = apkpureURLForVersion(body, "")
	if err != nil || !strings.Contains(got, "/APK/AAA") {
		t.Fatalf("latest: %q %v", got, err)
	}
	if _, err := apkpureURLForVersion(body, "9.9.9"); err == nil {
		t.Fatal("expected missing version")
	}
	if _, err := apkpureURLForVersion([]byte("no urls"), ""); err == nil {
		t.Fatal("expected no URL")
	}
}

func TestApkpureURLForVersion_skipsXAPK(t *testing.T) {
	t.Parallel()
	body := []byte("x3.3.6:(aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
		"XAPKJ\x00https://download.pureapk.com/b/XAPK/BUNDLE?k=1" +
		"x3.3.6:(aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
		"APKJ\x00https://download.pureapk.com/b/APK/AAA?k=2")
	got, err := apkpureURLForVersion(body, "3.3.6")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "/b/APK/AAA") {
		t.Fatalf("got %q", got)
	}
	got, err = apkpureURLForVersion(body, "")
	if err != nil || !strings.Contains(got, "/b/APK/AAA") {
		t.Fatalf("latest: %q %v", got, err)
	}
}

func TestApkpureURLForVersion_xapkOnlyIsMissing(t *testing.T) {
	t.Parallel()
	body := []byte("x3.3.6:(aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
		"XAPKJ\x00https://download.pureapk.com/b/XAPK/BUNDLE?k=1" +
		"x3.3.5:(aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
		"APKJ\x00https://download.pureapk.com/b/APK/BBB?k=2")
	if _, err := apkpureURLForVersion(body, "3.3.6"); err == nil {
		t.Fatal("expected XAPK-only version to miss")
	}
	got, err := apkpureURLForVersion(body, "3.3.5")
	if err != nil || !strings.Contains(got, "/b/APK/BBB") {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestScoreAPKPureURL(t *testing.T) {
	t.Parallel()
	uni := scoreAPKPureURL("https://download.pureapk.com/b/APK/AAA?x=universal-nodpi")
	abi := scoreAPKPureURL("https://download.pureapk.com/b/APK/BBB?x=arm64-v8a-480dpi")
	if !(uni > abi) {
		t.Fatalf("scores universal=%d abi=%d; want universal > abi", uni, abi)
	}
}

func TestLooksLikeBundleResponse(t *testing.T) {
	t.Parallel()
	if !looksLikeBundleResponse(&http.Response{
		Header: http.Header{"Content-Disposition": []string{`attachment; filename="app.xapk"`}},
	}) {
		t.Fatal("xapk filename")
	}
	if looksLikeBundleResponse(&http.Response{
		Header: http.Header{"Content-Type": []string{"application/vnd.android.package-archive"}},
	}) {
		t.Fatal("plain apk must pass")
	}
}

func TestAPKPure_Fetch_httptest(t *testing.T) {
	apkBody := mustStoredZipBytes(t, map[string][]byte{
		"AndroidManifest.xml": make([]byte, int(MinAPKBytes)+64),
	})
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/list", func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("x1.2.3:(aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaAPKJ\x00" + srv.URL + "/b/APK/file?k=1")); err != nil {
			panic(err)
		}
	})
	mux.HandleFunc("/b/APK/file", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.android.package-archive")
		if _, err := w.Write(apkBody); err != nil {
			panic(err)
		}
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	prevList, prevCDN := apkpureListURL, apkpureCDN
	apkpureListURL = srv.URL + "/list?pkg="
	apkpureCDN = srv.URL
	t.Cleanup(func() {
		apkpureListURL = prevList
		apkpureCDN = prevCDN
	})

	d := &APKPure{Client: srv.Client()}
	res, err := d.Fetch(t.Context(), Request{PackageID: "com.example.app", Version: "1.2.3"}, t.TempDir())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.SourceID != "apkpure" {
		t.Fatalf("source %q", res.SourceID)
	}
	if err := ValidateAPK(res.Path); err != nil {
		t.Fatal(err)
	}
}

func TestAPKPure_Fetch_skipsBundleTriesNext(t *testing.T) {
	apkBody := mustStoredZipBytes(t, map[string][]byte{
		"AndroidManifest.xml": make([]byte, int(MinAPKBytes)+64),
	})
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/list", func(w http.ResponseWriter, r *http.Request) {
		body := "x1.2.3:(aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaAPKJ\x00" + srv.URL + "/b/APK/fake?k=1\x00" +
			"x1.2.3:(aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaAPKJ\x00" + srv.URL + "/b/APK/file?k=2"
		if _, err := w.Write([]byte(body)); err != nil {
			panic(err)
		}
	})
	mux.HandleFunc("/b/APK/fake", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="app.xapk"`)
		if _, err := w.Write(make([]byte, int(MinAPKBytes)+8)); err != nil {
			panic(err)
		}
	})
	mux.HandleFunc("/b/APK/file", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.android.package-archive")
		if _, err := w.Write(apkBody); err != nil {
			panic(err)
		}
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	prevList, prevCDN := apkpureListURL, apkpureCDN
	apkpureListURL = srv.URL + "/list?pkg="
	apkpureCDN = srv.URL
	t.Cleanup(func() {
		apkpureListURL = prevList
		apkpureCDN = prevCDN
	})

	res, err := (&APKPure{Client: srv.Client()}).Fetch(t.Context(), Request{PackageID: "com.example.app", Version: "1.2.3"}, t.TempDir())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(res.URL, "/b/APK/file") {
		t.Fatalf("url %q", res.URL)
	}
	if err := ValidateAPK(res.Path); err != nil {
		t.Fatal(err)
	}
}
