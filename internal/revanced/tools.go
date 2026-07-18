package revanced

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v69/github"
	"golang.org/x/oauth2"
)

// FetchLatest downloads the latest revanced-cli jar and patches rvp.
func FetchLatest(ctx context.Context, token, cliJarPath, patchesPath string) error {
	if err := FetchCLI(ctx, token, cliJarPath); err != nil {
		return err
	}
	return FetchPatches(ctx, token, patchesPath)
}

// FetchCLI downloads the latest revanced-cli jar.
func FetchCLI(ctx context.Context, token, cliJarPath string) error {
	client := githubClient(ctx, token)
	if err := downloadLatestAsset(ctx, client, "ReVanced", "revanced-cli", func(name string) bool {
		return strings.HasSuffix(name, ".jar") && !strings.Contains(name, "sources")
	}, cliJarPath); err != nil {
		return fmt.Errorf("revanced-cli: %w", err)
	}
	return nil
}

// FetchPatches downloads the latest patches .rvp.
// Note: ReVanced/revanced-patches may be unavailable (e.g. GitHub DMCA 451);
// callers can supply a local file instead.
func FetchPatches(ctx context.Context, token, patchesPath string) error {
	client := githubClient(ctx, token)
	// Primary + optional alternate repos via env REVANCEDBOT_PATCHES_REPO=owner/name
	owner, repo := "ReVanced", "revanced-patches"
	if alt := strings.TrimSpace(os.Getenv("REVANCEDBOT_PATCHES_REPO")); alt != "" {
		parts := strings.SplitN(alt, "/", 2)
		if len(parts) == 2 {
			owner, repo = parts[0], parts[1]
		}
	}
	if err := downloadLatestAsset(ctx, client, owner, repo, func(name string) bool {
		return strings.HasSuffix(name, ".rvp")
	}, patchesPath); err != nil {
		return fmt.Errorf("revanced-patches (%s/%s): %w", owner, repo, err)
	}
	return nil
}

func githubClient(ctx context.Context, token string) *github.Client {
	if token == "" {
		return github.NewClient(nil)
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return github.NewClient(oauth2.NewClient(ctx, ts))
}

func downloadLatestAsset(ctx context.Context, client *github.Client, owner, repo string, match func(string) bool, dest string) error {
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

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	rc, _, err := client.Repositories.DownloadReleaseAsset(ctx, owner, repo, asset.GetID(), http.DefaultClient)
	if err != nil {
		if u := asset.GetBrowserDownloadURL(); u != "" {
			return downloadURL(ctx, u, dest)
		}
		return err
	}
	defer rc.Close()

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, rc); err != nil {
		return err
	}
	return nil
}

func downloadURL(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}
