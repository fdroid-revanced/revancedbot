package download

import (
	"archive/zip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPKMirror_Fetch_httptest(t *testing.T) {
	apkBody := mustStoredZipBytes(t, map[string][]byte{
		"AndroidManifest.xml": make([]byte, int(MinAPKBytes)+128),
		"classes.dex":         []byte("dex"),
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.RawQuery, "com.example.app"):
			_, _ = w.Write([]byte(`<a href="/apk/ex/app/app-1-2-3-release/">App 1.2.3</a>`))
		case r.URL.Path == "/apk/ex/app/app-1-2-3-release/":
			_, _ = w.Write([]byte(`
				com.example.app
				<div>universal Android 10+ nodpi</div>
				<a href="/apk/ex/app/app-1-2-3-release/app-1-2-3-android-apk-download/">dl</a>
				<div>arm64-v8a 480dpi</div>
				<a href="/apk/ex/app/app-1-2-3-release/app-1-2-3-2-android-apk-download/">dl2</a>
			`))
		case r.URL.Path == "/apk/ex/app/app-1-2-3-release/app-1-2-3-android-apk-download/":
			_, _ = w.Write([]byte(`
				<a class="downloadButton" href="/apk/ex/app/app-1-2-3-release/app-1-2-3-android-apk-download/download/?key=deadbeef">Download APK</a>
			`))
		case r.URL.Path == "/apk/ex/app/app-1-2-3-release/app-1-2-3-android-apk-download/download/":
			_, _ = w.Write([]byte(`
				<a id="download-link" rel="nofollow" href="/wp-content/themes/APKMirror/download.php?id=99&key=cafebabe">here</a>
			`))
		case r.URL.Path == "/wp-content/themes/APKMirror/download.php":
			w.Header().Set("Content-Type", "application/vnd.android.package-archive")
			_, _ = w.Write(apkBody)
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	prev := apkmirrorBase
	apkmirrorBase = srv.URL
	t.Cleanup(func() { apkmirrorBase = prev })

	d := &APKMirror{Client: srv.Client()}
	dest := t.TempDir()
	res, err := d.Fetch(context.Background(), Request{PackageID: "com.example.app", Version: "1.2.3"}, dest)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.SourceID != "apkmirror" {
		t.Fatalf("source %q", res.SourceID)
	}
	if err := ValidateAPK(res.Path); err != nil {
		t.Fatalf("validate: %v", err)
	}
	st, err := os.Stat(res.Path)
	if err != nil || st.Size() < MinAPKBytes {
		t.Fatalf("bad result path: %v size=%v", err, st)
	}
}

func TestFetchFirst_validatesAndFallsThrough(t *testing.T) {
	// First source writes HTML garbage; second writes a real APK.
	html := []byte("<!DOCTYPE html><html><body>mirror error</body></html>")
	for len(html) < int(MinAPKBytes)+8 {
		html = append(html, ' ')
	}
	good := mustStoredZipBytes(t, map[string][]byte{
		"AndroidManifest.xml": make([]byte, int(MinAPKBytes)+64),
	})
	reg := Registry{
		"html": &stubDL{id: "html", body: html},
		"ok":   &stubDL{id: "ok", body: good},
	}
	dest := t.TempDir()
	res, err := FetchFirst(context.Background(), reg, []string{"html", "ok"}, Request{
		PackageID: "com.example.app",
		Version:   "1",
	}, dest)
	if err != nil {
		t.Fatalf("FetchFirst: %v", err)
	}
	if res.SourceID != "ok" {
		t.Fatalf("want ok source, got %s", res.SourceID)
	}
	if err := ValidateAPK(res.Path); err != nil {
		t.Fatal(err)
	}
}

type stubDL struct {
	id   string
	err  error
	body []byte
}

func (s *stubDL) ID() string { return s.id }

func (s *stubDL) Fetch(ctx context.Context, req Request, destDir string) (*Result, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.body == nil {
		return nil, fmt.Errorf("stub empty")
	}
	path := filepath.Join(destDir, stockFileName(req.PackageID, req.Version)+"."+s.id)
	if err := os.WriteFile(path, s.body, 0o644); err != nil {
		return nil, err
	}
	return &Result{Path: path, SourceID: s.id, URL: "stub://" + s.id}, nil
}

func mustStoredZipBytes(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "t.apk")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, content := range files {
		h := &zip.FileHeader{Name: name, Method: zip.Store}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
