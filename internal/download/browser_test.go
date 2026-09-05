package download

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
)

func TestConnectBrowser_emptyCDP(t *testing.T) {
	t.Parallel()
	_, err := connectBrowser(t.Context(), "")
	if !errors.Is(err, ErrNeedBrowser) {
		t.Fatalf("empty: %v", err)
	}
	_, err = connectBrowser(t.Context(), "  ")
	if !errors.Is(err, ErrNeedBrowser) {
		t.Fatalf("whitespace: %v", err)
	}
}

func TestConnectBrowser_deadCDP(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_, err := connectBrowser(ctx, "ws://127.0.0.1:1")
	if err == nil {
		t.Fatal("want connect error")
	}
	if errors.Is(err, ErrNeedBrowser) {
		t.Fatalf("want connect error, got need-browser: %v", err)
	}
}

func TestLooksLikeChallenge(t *testing.T) {
	t.Parallel()
	if !looksLikeChallenge("Just a moment...") {
		t.Fatal("cloudflare interstitial")
	}
	if looksLikeChallenge(`<html><a href="/apk/x/y/z-release/">ok</a></html>`) {
		t.Fatal("plain release page")
	}
}

func TestHTTPStatusNeedsBrowser(t *testing.T) {
	t.Parallel()
	if !httpStatusNeedsBrowser(http.StatusForbidden) {
		t.Fatal("403")
	}
	if httpStatusNeedsBrowser(http.StatusNotFound) {
		t.Fatal("404 is not a browser problem")
	}
}

func TestRawHTTP_finishNeedBrowser(t *testing.T) {
	t.Parallel()
	r := rawHTTP{URL: "https://example.invalid/x", Status: http.StatusForbidden, Body: []byte("no")}
	_, err := r.finish(t.Context(), "")
	if !errors.Is(err, ErrNeedBrowser) {
		t.Fatalf("got %v", err)
	}
}

func TestRawHTTP_finishChallengeNeedBrowser(t *testing.T) {
	t.Parallel()
	r := rawHTTP{
		URL:    "https://example.invalid/x",
		Status: http.StatusOK,
		Body:   []byte("Just a moment... cf-browser-verification"),
		HTMLOK: true,
	}
	_, err := r.finish(t.Context(), "")
	if !errors.Is(err, ErrNeedBrowser) {
		t.Fatalf("got %v", err)
	}
}

func TestRawHTTP_notFoundIsNotBrowser(t *testing.T) {
	t.Parallel()
	r := rawHTTP{URL: "https://example.invalid/x", Status: http.StatusNotFound, Body: []byte("no")}
	_, err := r.finish(t.Context(), "")
	if err == nil || errors.Is(err, ErrNeedBrowser) {
		t.Fatalf("got %v", err)
	}
}

func TestRawHTTP_okSkipsBrowser(t *testing.T) {
	t.Parallel()
	want := []byte(`{"info":{"status":"OK"}}`)
	r := rawHTTP{URL: "https://example.invalid/x", Status: http.StatusOK, Body: want}
	got, err := r.finish(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestRawHTTP_finishDeadCDP(t *testing.T) {
	prev := connectCDP
	connectCDP = func(ctx context.Context, cdp string) (*rod.Browser, error) {
		return nil, fmt.Errorf("cdp connect: connection refused: %w", ErrBase)
	}
	t.Cleanup(func() { connectCDP = prev })

	r := rawHTTP{URL: "https://example.invalid/x", Status: http.StatusForbidden, Body: []byte("no")}
	_, err := r.finish(t.Context(), "ws://127.0.0.1:1")
	if err == nil || errors.Is(err, ErrNeedBrowser) {
		t.Fatalf("want connect error, got %v", err)
	}
}

func TestDefaultRegistry_setsCDP(t *testing.T) {
	t.Parallel()
	const cdp = "ws://127.0.0.1:3000"
	reg := DefaultRegistry(cdp)
	if got := reg["aptoide"].(*Aptoide).CDPURL; got != cdp {
		t.Fatalf("aptoide %q", got)
	}
	if got := reg["apkpure"].(*APKPure).CDPURL; got != cdp {
		t.Fatalf("apkpure %q", got)
	}
	if got := reg["apkmirror"].(*APKMirror).CDPURL; got != cdp {
		t.Fatalf("apkmirror %q", got)
	}
}

func TestFetchFirst_needBrowserContinues(t *testing.T) {
	good := mustStoredZipBytes(t, map[string][]byte{
		"AndroidManifest.xml": make([]byte, int(MinAPKBytes)+64),
	})
	reg := Registry{
		"need": &stubDL{id: "need", err: ErrNeedBrowser},
		"ok":   &stubDL{id: "ok", body: good},
	}
	res, err := FetchFirst(t.Context(), reg, []string{"need", "ok"}, Request{
		PackageID: "com.example.app",
		Version:   "1",
	}, t.TempDir())
	if err != nil {
		t.Fatalf("FetchFirst: %v", err)
	}
	if res.SourceID != "ok" {
		t.Fatalf("want ok, got %s", res.SourceID)
	}
}

func TestFetchFirst_deadCDPContinues(t *testing.T) {
	good := mustStoredZipBytes(t, map[string][]byte{
		"AndroidManifest.xml": make([]byte, int(MinAPKBytes)+64),
	})
	reg := Registry{
		"dead": &stubDL{id: "dead", err: fmt.Errorf("cdp connect: connection refused: %w", ErrBase)},
		"ok":   &stubDL{id: "ok", body: good},
	}
	res, err := FetchFirst(t.Context(), reg, []string{"dead", "ok"}, Request{
		PackageID: "com.example.app",
		Version:   "1",
	}, t.TempDir())
	if err != nil {
		t.Fatalf("FetchFirst: %v", err)
	}
	if res.SourceID != "ok" {
		t.Fatalf("want ok, got %s", res.SourceID)
	}
}

func TestINV06_noLauncherImport(t *testing.T) {
	t.Parallel()
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		b, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		needle := strings.Join([]string{"go-rod/rod/lib", "launcher"}, "/")
		if bytes.Contains(b, []byte(needle)) {
			t.Fatalf("%s imports rod launcher", e.Name())
		}
	}
}

func TestAptoide_Fetch_forbiddenWithoutCDP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	prev := aptoideVersionsURL
	aptoideVersionsURL = srv.URL + "/versions/"
	t.Cleanup(func() { aptoideVersionsURL = prev })

	_, err := (&Aptoide{Client: srv.Client()}).Fetch(t.Context(), Request{PackageID: "com.example.app"}, t.TempDir())
	if !errors.Is(err, ErrNeedBrowser) {
		t.Fatalf("got %v", err)
	}
}

func TestAptoide_Fetch_forbiddenDeadCDP(t *testing.T) {
	prevFn := connectCDP
	connectCDP = func(ctx context.Context, cdp string) (*rod.Browser, error) {
		return nil, fmt.Errorf("cdp connect: connection refused: %w", ErrBase)
	}
	t.Cleanup(func() { connectCDP = prevFn })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	prev := aptoideVersionsURL
	aptoideVersionsURL = srv.URL + "/versions/"
	t.Cleanup(func() { aptoideVersionsURL = prev })

	_, err := (&Aptoide{Client: srv.Client(), CDPURL: "ws://127.0.0.1:1"}).Fetch(t.Context(), Request{PackageID: "com.example.app"}, t.TempDir())
	if err == nil || errors.Is(err, ErrNeedBrowser) {
		t.Fatalf("want connect error, got %v", err)
	}
}

func TestAPKPure_listAPKURLs_forbiddenWithoutCDP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	prevList := apkpureListURL
	apkpureListURL = srv.URL + "/list?pkg="
	t.Cleanup(func() { apkpureListURL = prevList })

	cl := srv.Client()
	_, err := (&APKPure{Client: cl}).listAPKURLs(t.Context(), cl, "com.example.app", "")
	if !errors.Is(err, ErrNeedBrowser) {
		t.Fatalf("got %v", err)
	}
}

func TestAPKMirror_Fetch_forbiddenWithoutCDP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	prev := apkmirrorBase
	apkmirrorBase = srv.URL
	t.Cleanup(func() { apkmirrorBase = prev })

	_, err := (&APKMirror{Client: srv.Client()}).Fetch(t.Context(), Request{PackageID: "com.example.app"}, t.TempDir())
	if !errors.Is(err, ErrNeedBrowser) {
		t.Fatalf("got %v", err)
	}
}
