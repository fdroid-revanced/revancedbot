package revanced

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v69/github"
	"github.com/lucasew/revancedbot/internal/netx"
	"golang.org/x/oauth2"
)

// Default patches sources after GitHub DMCA on ReVanced/revanced-patches.
// Official pointer: https://github.com/ReVanced/where-is-revanced-patches
// Source of truth (source tree + tags): https://gitlab.com/revanced/revanced-patches
// Prebuilt .rvp files are not currently attached to GitLab releases; we resolve
// the latest tag from GitLab then try GitHub (if reinstated) and known mirrors.
const (
	gitlabPatchesProject = "ReVanced%2Frevanced-patches"
	gitlabAPI            = "https://gitlab.com/api/v4"
)

// FetchLatest downloads the latest revanced-cli jar and patches rvp.
func FetchLatest(ctx context.Context, token, cliJarPath, patchesPath string) error {
	if err := FetchCLI(ctx, token, cliJarPath); err != nil {
		return err
	}
	return FetchPatches(ctx, token, patchesPath)
}

// FetchCLI downloads the latest revanced-cli jar from GitHub.
func FetchCLI(ctx context.Context, token, cliJarPath string) error {
	ctx = netx.WithLabel(ctx, "download ReVanced CLI")
	client := githubClient(ctx, token)
	if err := downloadLatestGitHubAsset(ctx, client, "ReVanced", "revanced-cli", func(name string) bool {
		return strings.HasSuffix(name, ".jar") && !strings.Contains(name, "sources")
	}, cliJarPath); err != nil {
		return fmt.Errorf("revanced-cli: %w", err)
	}
	return nil
}

// FetchPatches downloads the latest patches .rvp.
//
// Order:
//  1. REVANCEDBOT_PATCHES_FILE (local path)
//  2. REVANCEDBOT_PATCHES_URL (direct URL)
//  3. Resolve latest tag from GitLab (or REVANCEDBOT_PATCHES_REPO GitHub path)
//  4. Try GitHub release assets for that tag
//  5. Try SourceForge revanced.mirror for that tag (community mirror of .rvp)
func FetchPatches(ctx context.Context, token, patchesPath string) error {
	ctx = netx.WithLabel(ctx, "download ReVanced patches")
	if local := strings.TrimSpace(os.Getenv("REVANCEDBOT_PATCHES_FILE")); local != "" {
		return copyFile(local, patchesPath)
	}
	if u := strings.TrimSpace(os.Getenv("REVANCEDBOT_PATCHES_URL")); u != "" {
		return downloadURL(ctx, u, patchesPath)
	}

	tag, ver, err := resolveLatestPatchesVersion(ctx)
	if err != nil {
		return fmt.Errorf("resolve patches version: %w", err)
	}

	var errs []string

	// 1) GitHub (works again when DMCA lifts; also honors REVANCEDBOT_PATCHES_REPO)
	if err := fetchPatchesGitHub(ctx, token, tag, patchesPath); err == nil {
		return nil
	} else {
		errs = append(errs, "github: "+err.Error())
	}

	// 2) SourceForge community mirror (hosts patches-X.Y.Z.rvp)
	// https://sourceforge.net/projects/revanced.mirror/
	sfURL := fmt.Sprintf(
		"https://sourceforge.net/projects/revanced.mirror/files/%s/patches-%s.rvp/download",
		tag, ver,
	)
	if err := downloadURL(ctx, sfURL, patchesPath); err == nil {
		if st, e := os.Stat(patchesPath); e == nil && st.Size() > 1024 {
			return nil
		}
		errs = append(errs, "sourceforge: download too small")
	} else {
		errs = append(errs, "sourceforge: "+err.Error())
	}

	return fmt.Errorf("patches %s unavailable: %s (see https://github.com/ReVanced/where-is-revanced-patches → GitLab; or set REVANCEDBOT_PATCHES_FILE)", tag, strings.Join(errs, "; "))
}

func resolveLatestPatchesVersion(ctx context.Context) (tag, version string, err error) {
	// Prefer GitLab releases (stable first, then any).
	rels, err := gitlabReleases(ctx)
	if err != nil {
		return "", "", err
	}
	// Pass 1: non-dev
	for _, r := range rels {
		t := r.TagName
		if t == "" {
			continue
		}
		if strings.Contains(t, "-dev") {
			continue
		}
		return t, strings.TrimPrefix(t, "v"), nil
	}
	// Pass 2: anything
	for _, r := range rels {
		if r.TagName != "" {
			return r.TagName, strings.TrimPrefix(r.TagName, "v"), nil
		}
	}
	return "", "", fmt.Errorf("no releases on gitlab.com/ReVanced/revanced-patches")
}

type gitlabRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
}

func gitlabReleases(ctx context.Context) ([]gitlabRelease, error) {
	url := fmt.Sprintf("%s/projects/%s/releases?per_page=30", gitlabAPI, gitlabPatchesProject)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("gitlab releases: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var rels []gitlabRelease
	if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
		return nil, err
	}
	return rels, nil
}

func fetchPatchesGitHub(ctx context.Context, token, tag, dest string) error {
	owner, repo := "ReVanced", "revanced-patches"
	if alt := strings.TrimSpace(os.Getenv("REVANCEDBOT_PATCHES_REPO")); alt != "" {
		parts := strings.SplitN(alt, "/", 2)
		if len(parts) == 2 {
			owner, repo = parts[0], parts[1]
		}
	}
	client := githubClient(ctx, token)
	rel, _, err := client.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		// try latest if tag-specific fails
		rel, _, err = client.Repositories.GetLatestRelease(ctx, owner, repo)
		if err != nil {
			return err
		}
	}
	ver := strings.TrimPrefix(rel.GetTagName(), "v")
	match := func(name string) bool {
		if !strings.HasSuffix(name, ".rvp") {
			return false
		}
		// prefer patches-X.Y.Z.rvp
		return strings.Contains(name, ver) || strings.HasPrefix(name, "patches")
	}
	var asset *github.ReleaseAsset
	for _, a := range rel.Assets {
		if a.GetName() != "" && match(a.GetName()) {
			asset = a
			break
		}
	}
	if asset == nil {
		for _, a := range rel.Assets {
			if strings.HasSuffix(a.GetName(), ".rvp") {
				asset = a
				break
			}
		}
	}
	if asset == nil {
		return fmt.Errorf("no .rvp on %s/%s %s", owner, repo, rel.GetTagName())
	}
	return downloadGitHubAsset(ctx, client, owner, repo, asset, dest)
}

func githubClient(ctx context.Context, token string) *github.Client {
	if token == "" {
		return github.NewClient(nil)
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return github.NewClient(oauth2.NewClient(ctx, ts))
}

func downloadLatestGitHubAsset(ctx context.Context, client *github.Client, owner, repo string, match func(string) bool, dest string) error {
	rel, _, err := client.Repositories.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		return err
	}
	var asset *github.ReleaseAsset
	for _, a := range rel.Assets {
		if a.GetName() != "" && match(a.GetName()) {
			asset = a
			break
		}
	}
	if asset == nil {
		return fmt.Errorf("no matching asset on %s/%s %s", owner, repo, rel.GetTagName())
	}
	return downloadGitHubAsset(ctx, client, owner, repo, asset, dest)
}

func downloadGitHubAsset(ctx context.Context, client *github.Client, owner, repo string, asset *github.ReleaseAsset, dest string) error {
	// Prefer browser download URL through fetchurl (progress-aware).
	if u := asset.GetBrowserDownloadURL(); u != "" {
		if err := downloadURL(ctx, u, dest); err == nil {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	rc, _, err := client.Repositories.DownloadReleaseAsset(ctx, owner, repo, asset.GetID(), netx.Client(ctx))
	if err != nil {
		return err
	}
	defer rc.Close()
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := io.Copy(f, rc)
	if err != nil {
		return err
	}
	if n < 1024 {
		_ = os.Remove(dest)
		return fmt.Errorf("github asset too small (%d bytes)", n)
	}
	return nil
}

func downloadURL(ctx context.Context, url, dest string) error {
	// Known-URL asset path: fetchurl driver (+ httpclient progress).
	if err := netx.FetchURLs(ctx, []string{url}, dest, "", ""); err != nil {
		// Fallback: direct GET with progress client (e.g. drivers not registered in tests).
		return downloadURLDirect(ctx, url, dest)
	}
	return nil
}

func downloadURLDirect(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "revancedbot/1.0 (+https://github.com/lucasew/revancedbot)")
	resp, err := netx.Client(ctx).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	if n < 1024 {
		_ = os.Remove(dest)
		return fmt.Errorf("GET %s: body too small (%d bytes)", url, n)
	}
	return nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
