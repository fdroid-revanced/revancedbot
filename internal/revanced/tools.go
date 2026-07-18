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
	client := githubClient(ctx, token)

	if err := downloadLatestAsset(ctx, client, "ReVanced", "revanced-cli", func(name string) bool {
		return strings.HasSuffix(name, ".jar") && !strings.Contains(name, "sources")
	}, cliJarPath); err != nil {
		return fmt.Errorf("revanced-cli: %w", err)
	}

	if err := downloadLatestAsset(ctx, client, "ReVanced", "revanced-patches", func(name string) bool {
		return strings.HasSuffix(name, ".rvp")
	}, patchesPath); err != nil {
		return fmt.Errorf("revanced-patches: %w", err)
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

	rc, _, err := client.Repositories.DownloadReleaseAsset(ctx, owner, repo, asset.GetID(), http.DefaultClient)
	if err != nil {
		return err
	}
	defer rc.Close()

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
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
