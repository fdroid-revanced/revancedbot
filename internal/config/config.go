package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config is loaded from REPO/revancedbot.yaml (authority) plus env/flags.
type Config struct {
	Repo  string // absolute F-Droid root
	Cache string // absolute cache (may be empty before layout resolves mkdtemp)

	RepoName        string
	RepoURL         string
	RepoDescription string
	DownloaderOrder []string
	BrowserCDPURL   string
	PoolIO          int
	PoolCPU         int
	PoolInternet    int
	LogLevel        string

	SigningBlob string
	GitHubToken string
}

// LoadFromRepo loads REPO/revancedbot.yaml (or cfgFile override).
// cacheFlag empty means caller will mkdtemp.
func LoadFromRepo(repo, cacheFlag, cfgFile string) (*Config, error) {
	repoAbs, err := filepath.Abs(repo)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(repoAbs)
	if err != nil {
		return nil, fmt.Errorf("repo %s: %w", repoAbs, err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("repo is not a directory: %s", repoAbs)
	}

	v := viper.New()
	v.SetConfigType("yaml")
	v.SetEnvPrefix("REVANCEDBOT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	v.SetDefault("repo_name", "ReVanced F-Droid Repo")
	v.SetDefault("repo_url", "https://example.invalid/fdroid/repo")
	v.SetDefault("repo_description", "ReVanced-patched apps (simple binary repository).")
	v.SetDefault("downloaders", []string{"apkpure"})
	v.SetDefault("pool_io", 4)
	v.SetDefault("pool_cpu", 0)
	v.SetDefault("pool_internet", 4)
	v.SetDefault("log_level", "info")

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config %s: %w", cfgFile, err)
		}
	} else {
		path := filepath.Join(repoAbs, "revancedbot.yaml")
		if _, err := os.Stat(path); err == nil {
			v.SetConfigFile(path)
			if err := v.ReadInConfig(); err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
		}
		// optional: missing yaml uses defaults only
	}

	type fileShape struct {
		RepoName        string   `mapstructure:"repo_name"`
		RepoURL         string   `mapstructure:"repo_url"`
		RepoDescription string   `mapstructure:"repo_description"`
		Downloaders     []string `mapstructure:"downloaders"`
		PoolIO          int      `mapstructure:"pool_io"`
		PoolCPU         int      `mapstructure:"pool_cpu"`
		PoolInternet    int      `mapstructure:"pool_internet"`
		LogLevel        string   `mapstructure:"log_level"`
		Browser         struct {
			CDPURL string `mapstructure:"cdp_url"`
		} `mapstructure:"browser"`
		CDPURL string `mapstructure:"cdp_url"`
	}
	var raw fileShape
	if err := v.Unmarshal(&raw); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg := &Config{
		Repo:            repoAbs,
		Cache:           cacheFlag,
		RepoName:        raw.RepoName,
		RepoURL:         raw.RepoURL,
		RepoDescription: raw.RepoDescription,
		DownloaderOrder: raw.Downloaders,
		PoolIO:          raw.PoolIO,
		PoolCPU:         raw.PoolCPU,
		PoolInternet:    raw.PoolInternet,
		LogLevel:        raw.LogLevel,
		BrowserCDPURL:   raw.CDPURL,
	}
	if cfg.BrowserCDPURL == "" {
		cfg.BrowserCDPURL = raw.Browser.CDPURL
	}
	if u := os.Getenv("REVANCEDBOT_CDP_URL"); u != "" {
		cfg.BrowserCDPURL = u
	}
	cfg.SigningBlob = firstNonEmpty(os.Getenv("REVANCEDBOT_SIGNING"), os.Getenv("REVANCEDBOT_SIGNING_BLOB"))
	cfg.GitHubToken = firstNonEmpty(os.Getenv("GITHUB_TOKEN"), os.Getenv("REVANCEDBOT_GITHUB_TOKEN"))
	return cfg, nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
