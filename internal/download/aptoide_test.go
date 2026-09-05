package download

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestLooksLikeBundleURL(t *testing.T) {
	if !looksLikeBundleURL("https://cdn.example/app.apkm") {
		t.Fatal("apkm")
	}
	if !looksLikeBundleURL("https://cdn.example/xapk/foo") {
		t.Fatal("xapk path")
	}
	if looksLikeBundleURL("https://pool.apk.aptoide.com/store/app.apk") {
		t.Fatal("plain apk must pass")
	}
	if !looksLikeBundleURL("https://cdn/com.google.android.youtube_20.40.45-1_4arch_7dpi_24lang_abc_apkmir") {
		t.Fatal("apkmir split name")
	}
}

func TestIsAllDigits(t *testing.T) {
	if !isAllDigits("220489") {
		t.Fatal("digits")
	}
	if isAllDigits("1.2.3") || isAllDigits("") {
		t.Fatal("non-digits")
	}
}

func TestAptoide_implementsDownloader(t *testing.T) {
	var _ Downloader = &Aptoide{}
}

func TestPickAptoideVersion(t *testing.T) {
	trustedUni := aptoideVersion{
		ID: 1, VerName: "3.3.6", VerCode: 220489, Malware: "TRUSTED",
		CPUs: []string{"armeabi-v7a", "arm64-v8a"},
	}
	trustedArm64 := aptoideVersion{
		ID: 2, VerName: "3.3.6", VerCode: 220490, Malware: "TRUSTED",
		CPUs: []string{"arm64-v8a"},
	}
	untrusted := aptoideVersion{
		ID: 3, VerName: "3.3.7", VerCode: 220500, Malware: "WARNING",
		CPUs: []string{"armeabi-v7a", "arm64-v8a"},
	}
	older := aptoideVersion{
		ID: 4, VerName: "3.2.0", VerCode: 210000, Malware: "TRUSTED",
		CPUs: []string{"armeabi-v7a", "arm64-v8a"},
	}
	narrowDPI := aptoideVersion{
		ID: 5, VerName: "3.3.6", VerCode: 220491, Malware: "TRUSTED",
		CPUs: []string{"armeabi-v7a", "arm64-v8a"}, Densities: []byte(`[[480,480]]`),
	}
	obb := aptoideVersion{
		ID: 6, VerName: "3.3.6", VerCode: 220492, Malware: "TRUSTED",
		CPUs: []string{"armeabi-v7a", "arm64-v8a"}, HasOBB: true,
	}
	newerArm64 := aptoideVersion{
		ID: 7, VerName: "3.3.7", VerCode: 220501, Malware: "TRUSTED",
		CPUs: []string{"arm64-v8a"},
	}
	multiX86 := aptoideVersion{
		ID: 8, VerName: "3.3.6", VerCode: 220493, Malware: "TRUSTED",
		CPUs: []string{"x86", "x86_64"},
	}

	t.Parallel()
	tests := []struct {
		name string
		list []aptoideVersion
		want string
		id   int64
		ok   bool
	}{
		{name: "exact", list: []aptoideVersion{trustedUni, older}, want: "3.3.6", id: 1, ok: true},
		{name: "vercode", list: []aptoideVersion{trustedUni}, want: "220489", id: 1, ok: true},
		{name: "prefix", list: []aptoideVersion{older, trustedUni}, want: "3.3", id: 1, ok: true},
		{name: "skip untrusted latest", list: []aptoideVersion{untrusted, trustedUni}, want: "", id: 1, ok: true},
		{name: "prefer universal", list: []aptoideVersion{trustedArm64, trustedUni}, want: "3.3.6", id: 1, ok: true},
		{name: "prefer multi-abi", list: []aptoideVersion{trustedArm64, multiX86}, want: "3.3.6", id: 8, ok: true},
		{name: "prefer broad dpi", list: []aptoideVersion{narrowDPI, trustedUni}, want: "3.3.6", id: 1, ok: true},
		{name: "skip obb", list: []aptoideVersion{obb, trustedUni}, want: "3.3.6", id: 1, ok: true},
		{name: "latest stays on newest", list: []aptoideVersion{newerArm64, trustedUni}, want: "", id: 7, ok: true},
		{name: "reject untrusted exact", list: []aptoideVersion{untrusted}, want: "3.3.7", ok: false},
		{name: "empty", list: nil, want: "1.0", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := pickAptoideVersion(tt.list, tt.want)
			if ok != tt.ok {
				t.Fatalf("ok=%v want %v (got %+v)", ok, tt.ok, got)
			}
			if tt.ok && got.ID != tt.id {
				t.Fatalf("id=%d want %d", got.ID, tt.id)
			}
		})
	}
}

func TestRankAptoideVersions_order(t *testing.T) {
	t.Parallel()
	uni := aptoideVersion{
		ID: 1, VerName: "3.3.6", VerCode: 1, Malware: "TRUSTED",
		CPUs: []string{"armeabi-v7a", "arm64-v8a"},
	}
	arm := aptoideVersion{
		ID: 2, VerName: "3.3.6", VerCode: 2, Malware: "TRUSTED",
		CPUs: []string{"arm64-v8a"},
	}
	ranked := rankAptoideVersions([]aptoideVersion{arm, uni}, "3.3.6")
	if len(ranked) != 2 || ranked[0].ID != 1 || ranked[1].ID != 2 {
		t.Fatalf("order %+v", ranked)
	}
}

func TestAptoide_Fetch_httptest(t *testing.T) {
	apkBody := mustStoredZipBytes(t, map[string][]byte{
		"AndroidManifest.xml": make([]byte, int(MinAPKBytes)+128),
		"classes.dex":         []byte("dex"),
	})
	sum := md5.Sum(apkBody)
	md5hex := hex.EncodeToString(sum[:])

	mux := http.NewServeMux()
	mux.HandleFunc("/versions/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
			"info":{"status":"OK"},
			"list":[{
				"id":99,
				"size":%d,
				"file":{
					"vername":"3.3.6",
					"vercode":220489,
					"md5sum":%q,
					"filesize":%d,
					"malware":{"rank":"TRUSTED"},
					"hardware":{"cpus":["armeabi-v7a","arm64-v8a"]}
				}
			}]
		}`, len(apkBody), md5hex, len(apkBody))
	})
	mux.HandleFunc("/meta/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"info":{"status":"OK"},"data":{"file":{"vername":"3.3.6","path":"%s/apk"}}}`, "http://"+r.Host)
	})
	mux.HandleFunc("/apk", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.android.package-archive")
		if _, err := w.Write(apkBody); err != nil {
			panic(err)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	prevV, prevM := aptoideVersionsURL, aptoideMetaURLs
	aptoideVersionsURL = srv.URL + "/versions/"
	aptoideMetaURLs = []string{srv.URL + "/meta/"}
	t.Cleanup(func() {
		aptoideVersionsURL = prevV
		aptoideMetaURLs = prevM
	})

	d := &Aptoide{Client: srv.Client()}
	res, err := d.Fetch(t.Context(), Request{PackageID: "com.bandcamp.android", Version: "3.3.6"}, t.TempDir())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.SourceID != "aptoide" {
		t.Fatalf("source %q", res.SourceID)
	}
	if err := ValidateAPK(res.Path); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestAptoide_Fetch_rejectsMD5Mismatch(t *testing.T) {
	apkBody := mustStoredZipBytes(t, map[string][]byte{
		"AndroidManifest.xml": make([]byte, int(MinAPKBytes)+128),
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/versions/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
			"info":{"status":"OK"},
			"list":[{
				"id":1,
				"file":{
					"vername":"1.0.0",
					"vercode":1,
					"md5sum":"00000000000000000000000000000000",
					"filesize":%d,
					"malware":{"rank":"TRUSTED"},
					"hardware":{"cpus":["arm64-v8a","armeabi-v7a"]}
				}
			}]
		}`, len(apkBody))
	})
	mux.HandleFunc("/meta/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"info":{"status":"OK"},"data":{"file":{"vername":"1.0.0","path":"%s/apk"}}}`, "http://"+r.Host)
	})
	mux.HandleFunc("/apk", func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(apkBody); err != nil {
			panic(err)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	prevV, prevM := aptoideVersionsURL, aptoideMetaURLs
	aptoideVersionsURL = srv.URL + "/versions/"
	aptoideMetaURLs = []string{srv.URL + "/meta/"}
	t.Cleanup(func() {
		aptoideVersionsURL = prevV
		aptoideMetaURLs = prevM
	})

	_, err := (&Aptoide{Client: srv.Client()}).Fetch(t.Context(), Request{PackageID: "com.example.app", Version: "1.0.0"}, t.TempDir())
	if err == nil || !errors.Is(err, ErrBase) {
		t.Fatalf("want digest error, got %v", err)
	}
}

func TestAptoide_Fetch_skipsBundleTriesNext(t *testing.T) {
	apkBody := mustStoredZipBytes(t, map[string][]byte{
		"AndroidManifest.xml": make([]byte, int(MinAPKBytes)+128),
		"classes.dex":         []byte("dex"),
	})
	sum := md5.Sum(apkBody)
	md5hex := hex.EncodeToString(sum[:])

	mux := http.NewServeMux()
	mux.HandleFunc("/versions/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
			"info":{"status":"OK"},
			"list":[
				{"id":1,"file":{"vername":"1.0.0","vercode":1,"md5sum":%q,"filesize":%d,"malware":{"rank":"TRUSTED"},"hardware":{"cpus":["armeabi-v7a","arm64-v8a"]}}},
				{"id":2,"file":{"vername":"1.0.0","vercode":2,"md5sum":%q,"filesize":%d,"malware":{"rank":"TRUSTED"},"hardware":{"cpus":["arm64-v8a"]}}}
			]
		}`, md5hex, len(apkBody), md5hex, len(apkBody))
	})
	mux.HandleFunc("/meta/", func(w http.ResponseWriter, r *http.Request) {
		host := "http://" + r.Host
		if strings.Contains(r.URL.Path, "app_id=1") {
			fmt.Fprintf(w, `{"info":{"status":"OK"},"data":{"file":{"vername":"1.0.0","path":"%s/app.xapk"}}}`, host)
			return
		}
		fmt.Fprintf(w, `{"info":{"status":"OK"},"data":{"file":{"vername":"1.0.0","path":"%s/apk"}}}`, host)
	})
	mux.HandleFunc("/apk", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.android.package-archive")
		if _, err := w.Write(apkBody); err != nil {
			panic(err)
		}
	})
	mux.HandleFunc("/app.xapk", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("bundle URL should not be downloaded")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	prevV, prevM := aptoideVersionsURL, aptoideMetaURLs
	aptoideVersionsURL = srv.URL + "/versions/"
	aptoideMetaURLs = []string{srv.URL + "/meta/"}
	t.Cleanup(func() {
		aptoideVersionsURL = prevV
		aptoideMetaURLs = prevM
	})

	res, err := (&Aptoide{Client: srv.Client()}).Fetch(t.Context(), Request{PackageID: "com.example.app", Version: "1.0.0"}, t.TempDir())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.HasSuffix(res.URL, "/apk") {
		t.Fatalf("url %q", res.URL)
	}
	if err := ValidateAPK(res.Path); err != nil {
		t.Fatal(err)
	}
}

// Optional live smoke (opt-in): REVANCEDBOT_NETWORK=1 go test ./internal/download -run AptoideLive
func TestAptoideLive_bandcamp(t *testing.T) {
	if os.Getenv("REVANCEDBOT_NETWORK") != "1" {
		t.Skip("set REVANCEDBOT_NETWORK=1 for live Aptoide smoke")
	}
	dir := t.TempDir()
	res, err := (&Aptoide{}).Fetch(t.Context(), Request{PackageID: "com.bandcamp.android"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAPK(res.Path); err != nil {
		t.Fatal(err)
	}
	t.Logf("ok source=%s path=%s", res.SourceID, res.Path)
}
