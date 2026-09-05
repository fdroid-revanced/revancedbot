package revanced

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-github/v69/github"
	_ "github.com/lucasew/revancedbot/internal/drivers"
	"github.com/lucasew/workspaced/pkg/logging"
)

func TestFetchPatches_FromMirror(t *testing.T) {
	if os.Getenv("REVANCEDBOT_SKIP_NETWORK") == "1" {
		t.Skip("network disabled")
	}
	t.Setenv("REVANCEDBOT_PATCHES_FILE", "")
	t.Setenv("REVANCEDBOT_PATCHES_URL", "")
	ctx, cancel := context.WithTimeout(logging.NewWriterContext(t.Output()), 3*time.Minute)
	defer cancel()
	dest := filepath.Join(t.TempDir(), "patches.rvp")
	if err := FetchPatches(ctx, os.Getenv("GITHUB_TOKEN"), dest); err != nil {
		t.Fatalf("FetchPatches: %v", err)
	}
	st, err := os.Stat(dest)
	if err != nil || st.Size() < 100_000 {
		t.Fatalf("patches file missing/small: %v size=%v", err, st)
	}
	t.Logf("fetched %d bytes", st.Size())
}

func TestFetchPatches_FromFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.rvp")
	want := bytes.Repeat([]byte("from-file"), 256)
	if err := os.WriteFile(src, want, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVANCEDBOT_PATCHES_FILE", src)
	t.Setenv("REVANCEDBOT_PATCHES_URL", "")
	dest := filepath.Join(t.TempDir(), "patches.rvp")
	if err := FetchPatches(t.Context(), "", dest); err != nil {
		t.Fatalf("FetchPatches: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q", got[:min(24, len(got))])
	}
}

func TestFetchPatches_FromURL(t *testing.T) {
	want := bytes.Repeat([]byte("from-url"), 256)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(want); err != nil {
			t.Errorf("write override body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("REVANCEDBOT_PATCHES_FILE", "")
	t.Setenv("REVANCEDBOT_PATCHES_URL", srv.URL+"/patches.rvp")
	dest := filepath.Join(t.TempDir(), "patches.rvp")
	if err := FetchPatches(t.Context(), "", dest); err != nil {
		t.Fatalf("FetchPatches: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q", got[:min(24, len(got))])
	}
}

func TestFetchPatchesGitHub_OnlyResolvedTag(t *testing.T) {
	tagged := bytes.Repeat([]byte("tagged-rvp"), 200)
	older := bytes.Repeat([]byte("older-rvp!"), 200)
	var latestHits atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/ReVanced/revanced-patches/releases/tags/v5.41.0", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v5.41.0","assets":[{"id":1,"name":"patches-5.41.0.rvp","browser_download_url":"http://%s/tagged.rvp"}]}`, r.Host)
	})
	mux.HandleFunc("/repos/ReVanced/revanced-patches/releases/tags/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	})
	mux.HandleFunc("/repos/ReVanced/revanced-patches/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		latestHits.Add(1)
		fmt.Fprintf(w, `{"tag_name":"v4.0.0","assets":[{"id":2,"name":"patches-4.0.0.rvp","browser_download_url":"http://%s/older.rvp"}]}`, r.Host)
	})
	mux.HandleFunc("/tagged.rvp", func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(tagged); err != nil {
			t.Errorf("write tagged rvp: %v", err)
		}
	})
	mux.HandleFunc("/older.rvp", func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(older); err != nil {
			t.Errorf("write older rvp: %v", err)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := github.NewClient(srv.Client())
	u, err := url.Parse(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	client.BaseURL = u
	orig := newGitHubClient
	newGitHubClient = func(context.Context, string) *github.Client { return client }
	t.Cleanup(func() { newGitHubClient = orig })

	dest := filepath.Join(t.TempDir(), "ok.rvp")
	if err := fetchPatchesGitHub(t.Context(), "", "v5.41.0", dest); err != nil {
		t.Fatalf("tagged release: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, tagged) {
		t.Fatalf("tagged body: got %q", got[:min(24, len(got))])
	}

	missing := filepath.Join(t.TempDir(), "missing.rvp")
	err = fetchPatchesGitHub(t.Context(), "", "v9.9.9", missing)
	if err == nil {
		t.Fatal("missing GitLab tag must not succeed via GetLatestRelease")
	}
	if n := latestHits.Load(); n != 0 {
		t.Fatalf("GetLatestRelease called %d times", n)
	}
	if _, statErr := os.Stat(missing); statErr == nil {
		t.Fatal("must not write an older latest .rvp")
	}
}
