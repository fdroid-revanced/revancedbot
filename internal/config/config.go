package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config is the runtime configuration loaded from revancedbot.yaml, env, and flags.
type Config struct {
	Workspace string `mapstructure:"workspace"`

	RepoName        string `mapstructure:"repo_name"`
	RepoURL         string `mapstructure:"repo_url"`
	RepoDescription string `mapstructure:"repo_description"`

	DownloaderOrder []string `mapstructure:"downloaders"`

	BrowserCDPURL string `mapstructure:"cdp_url"`

	PoolIO       int `mapstructure:"pool_io"`
	PoolCPU      int `mapstructure:"pool_cpu"`
	PoolInternet int `mapstructure:"pool_internet"`

	LogLevel string `mapstructure:"log_level"`

	// SigningBlob is the pasteable secret (env REVANCEDBOT_SIGNING).
	SigningBlob string `mapstructure:"-"`

	GitHubToken string `mapstructure:"-"`
}

// Load reads config from path (optional), env, and applies flag overrides.
func Load(cfgFile string, workspaceFlag string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetEnvPrefix("REVANCEDBOT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	v.SetDefault("workspace", ".revancedbot")
	v.SetDefault("repo_name", "ReVanced F-Droid Repo")
	v.SetDefault("repo_url", "https://example.invalid/fdroid/repo")
	v.SetDefault("repo_description", "ReVanced-patched apps (simple binary repository).")
	v.SetDefault("downloaders", []string{"apkpure"})
	v.SetDefault("pool_io", 4)
	v.SetDefault("pool_cpu", 0) // 0 = NumCPU at runtime
	v.SetDefault("pool_internet", 4)
	v.SetDefault("log_level", "info")

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config %s: %w", cfgFile, err)
		}
	} else {
		// Prefer consumer-repo root file when present.
		v.SetConfigName("revancedbot")
		v.AddConfigPath(".")
		_ = v.ReadInConfig() // optional
	}

	// Nested browser.cdp_url support via flat key from env REVANCEDBOT_CDP_URL
	// and yaml:
	// browser:
	//   cdp_url: ...
	type fileShape struct {
		Workspace       string   `mapstructure:"workspace"`
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
		Workspace:       raw.Workspace,
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
	if workspaceFlag != "" {
		cfg.Workspace = workspaceFlag
	}
	if cfg.Workspace == "" {
		cfg.Workspace = ".revancedbot"
	}
	abs, err := filepath.Abs(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	cfg.Workspace = abs

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
