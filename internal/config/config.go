package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
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
	LogLevel        string

	// Optional pool overrides. Zero means "use workspaced DefaultLimits for that pool".
	PoolIO       int
	PoolCPU      int
	PoolInternet int

	SigningBlob string
	GitHubToken string
}

// EnsureRepoDir makes sure path exists as a directory (mkdir -p). Idempotent.
// Used by fdroid-init so `revancedbot fdroid-init ./new-repo` creates the tree.
func EnsureRepoDir(repo string) (string, error) {
	repoAbs, err := filepath.Abs(repo)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(repoAbs)
	if err == nil {
		if !st.IsDir() {
			return "", fmt.Errorf("repo is not a directory: %s: %w", repoAbs, ErrBase)
		}
		return repoAbs, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("repo %s: %w", repoAbs, err)
	}
	if err := os.MkdirAll(repoAbs, 0o755); err != nil {
		return "", fmt.Errorf("mkdir repo %s: %w", repoAbs, err)
	}
	return repoAbs, nil
}

// LoadFromRepo loads REPO/revancedbot.yaml (or cfgFile override).
// cacheFlag empty means caller will mkdtemp.
// The REPO path must already exist as a directory (use EnsureRepoDir for init).
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
		return nil, fmt.Errorf("repo is not a directory: %s: %w", repoAbs, ErrBase)
	}

	v := viper.New()
	v.SetConfigType("yaml")
	v.SetEnvPrefix("REVANCEDBOT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	v.SetDefault("repo_name", "ReVanced F-Droid Repo")
	v.SetDefault("repo_url", "https://example.invalid/fdroid/repo")
	v.SetDefault("repo_description", "ReVanced-patched apps (simple binary repository).")
	// Empty = CLI/pipeline use download.DefaultOrder (aptoide, apkpure, apkmirror).
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
		// Optional; omit or 0 → workspaced DefaultLimits for that pool.
		PoolIO       int    `mapstructure:"pool_io"`
		PoolCPU      int    `mapstructure:"pool_cpu"`
		PoolInternet int    `mapstructure:"pool_internet"`
		LogLevel     string `mapstructure:"log_level"`
		Browser      struct {
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

// LogLevelOrDefault returns AuthorityDoc log_level, or info if c is nil or the field is empty.
func (c *Config) LogLevelOrDefault() string {
	if c == nil || strings.TrimSpace(c.LogLevel) == "" {
		return "info"
	}
	return c.LogLevel
}

// ParseLogLevel maps AuthorityDoc log_level to slog.Level.
// Empty or whitespace is info. Names match slog: debug, info, warn, error.
func ParseLogLevel(s string) (slog.Level, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return slog.LevelInfo, nil
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return 0, fmt.Errorf("log_level %q: %w", s, err)
	}
	return level, nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
