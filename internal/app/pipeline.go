package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/lucasew/revancedbot/internal/config"
	"github.com/lucasew/revancedbot/internal/download"
	"github.com/lucasew/revancedbot/internal/fdroid"
	"github.com/lucasew/revancedbot/internal/revanced"
	"github.com/lucasew/revancedbot/internal/signing"
	"github.com/lucasew/revancedbot/internal/toolscheck"
	"github.com/lucasew/revancedbot/internal/workspace"
	"workspaced/pkg/logging"
	"workspaced/pkg/taskgroup"
)

// App wires config + layout for CLI commands.
type App struct {
	Cfg  *config.Config
	WS   *workspace.Layout
	Blob *signing.Blob
}

// New builds layout from config (REPO + CACHE).
func New(cfg *config.Config) (*App, error) {
	ws, err := workspace.New(cfg.Repo, cfg.Cache)
	if err != nil {
		return nil, err
	}
	if err := ws.Ensure(); err != nil {
		return nil, err
	}
	// Reflect resolved cache path (including mkdtemp).
	cfg.Cache = ws.Cache
	return &App{Cfg: cfg, WS: ws}, nil
}

// LoadSigning materializes the signing blob into CACHE.
func (a *App) LoadSigning() error {
	if a.Cfg.SigningBlob == "" {
		return fmt.Errorf("REVANCEDBOT_SIGNING is required")
	}
	blob, err := signing.DecodeBlob(a.Cfg.SigningBlob)
	if err != nil {
		return err
	}
	if err := blob.Materialize(a.WS.KeystorePath); err != nil {
		return err
	}
	a.Blob = blob
	return nil
}

// WriteFDroidConfig generates gitignored REPO/config.yml from revancedbot.yaml authority.
func (a *App) WriteFDroidConfig() error {
	if a.Blob == nil {
		return fmt.Errorf("signing not loaded")
	}
	if err := fdroid.EnsureLayout(a.WS.Repo); err != nil {
		return err
	}
	return fdroid.WriteConfig(a.WS.FDroidConfig(), fdroid.RepoMeta{
		Name:        a.Cfg.RepoName,
		URL:         a.Cfg.RepoURL,
		Description: a.Cfg.RepoDescription,
	}, a.WS.KeystorePath, a.Blob)
}

// FetchTools downloads CLI + patches into CACHE (skips name hits).
func (a *App) FetchTools(ctx context.Context) error {
	log := logging.GetLogger(ctx)
	cli := a.WS.PatcherJAR()
	rvp := a.WS.PatchesRVP()
	if workspace.CacheHit(cli) && workspace.CacheHit(rvp) {
		log.Info("tools cache hit", "cli", cli, "patches", rvp)
		return nil
	}
	log.Info("fetching ReVanced CLI and patches into cache", "cache", a.WS.Cache)
	// Always re-fetch both if either missing (simplicity).
	return revanced.FetchLatest(ctx, a.Cfg.GitHubToken, cli, rvp)
}

// ListJobs returns patch jobs.
func (a *App) ListJobs() ([]revanced.Job, error) {
	if !workspace.CacheHit(a.WS.PatcherJAR()) {
		return nil, fmt.Errorf("missing CLI jar in cache; run fetch-tools first: %s", a.WS.PatcherJAR())
	}
	if !workspace.CacheHit(a.WS.PatchesRVP()) {
		return nil, fmt.Errorf("missing patches in cache; run fetch-tools first: %s", a.WS.PatchesRVP())
	}
	return revanced.ListJobs("java", a.WS.PatcherJAR(), a.WS.PatchesRVP())
}

// ProcessPackage downloads and patches one package (version walk).
func (a *App) ProcessPackage(ctx context.Context, job revanced.Job) error {
	log := logging.GetLogger(ctx)
	reg := download.DefaultRegistry()
	order := a.Cfg.DownloaderOrder
	if len(order) == 0 {
		order = download.DefaultOrder
	}

	versions := job.Versions
	if len(versions) == 0 {
		versions = []string{""}
	}

	var lastErr error
	for _, ver := range versions {
		stockPath := a.WS.StockAPKPath(job.PackageID, ver)
		var res *download.Result
		if workspace.CacheHit(stockPath) {
			if err := download.AcceptCached(stockPath); err != nil {
				log.Warn("stock cache rejected", "path", stockPath, "err", err)
				// fall through to re-download
			} else {
				log.Info("stock cache hit", "path", stockPath)
				res = &download.Result{Path: stockPath, SourceID: "cache"}
			}
		}
		if res == nil {
			log.Info("download attempt", "package", job.PackageID, "version", emptyAsLatest(ver))
			// Prefer writing to the naive cache path name.
			got, err := download.FetchFirst(ctx, reg, order, download.Request{
				PackageID: job.PackageID,
				Version:   ver,
			}, a.WS.StockAPKs)
			if err != nil {
				lastErr = err
				log.Warn("download failed", "err", err)
				continue
			}
			// Normalize to stock cache name if downloader used a different name.
			if got.Path != stockPath {
				if err := os.Rename(got.Path, stockPath); err != nil {
					// copy fallback
					b, rerr := os.ReadFile(got.Path)
					if rerr != nil {
						lastErr = rerr
						continue
					}
					if werr := os.WriteFile(stockPath, b, 0o644); werr != nil {
						lastErr = werr
						continue
					}
					_ = os.Remove(got.Path)
				}
				got.Path = stockPath
			}
			res = got
		}

		outName := fmt.Sprintf("%s_%s_revanced.apk", sanitize(job.PackageID), sanitize(emptyAsLatest(ver)))
		outPath := filepath.Join(a.WS.Work, outName)
		_ = os.MkdirAll(a.WS.Work, 0o755)
		log.Info("patching", "in", res.Path, "out", outPath)
		patches, err := revanced.Patch(revanced.PatchOptions{
			CLIJar:                  a.WS.PatcherJAR(),
			PatchesRVP:              a.WS.PatchesRVP(),
			InputAPK:                res.Path,
			OutputAPK:               outPath,
			KeystorePath:            a.WS.KeystorePath,
			Blob:                    a.Blob,
			EnableChangePackageName: true,
		})
		if err != nil {
			lastErr = err
			log.Warn("patch failed", "err", err)
			continue
		}
		pubID := job.PackageID + ".revanced"
		if err := fdroid.StageAPK(a.WS.Repo, outPath); err != nil {
			return err
		}
		if err := fdroid.WritePatchesMetadata(a.WS.Repo, pubID, patches); err != nil {
			return err
		}
		log.Info("package ok", "package", job.PackageID, "apk", outPath)
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no versions to try")
	}
	return fmt.Errorf("skip %s: %w", job.PackageID, lastErr)
}

// FDroidUpdate runs fdroid update in REPO.
func (a *App) FDroidUpdate(createMeta bool) error {
	if a.Blob == nil {
		return fmt.Errorf("signing not loaded")
	}
	return fdroid.Update(a.WS.Repo, a.Blob, createMeta)
}

// RunFull is the kitchen-sink pipeline for REPO.
func (a *App) RunFull(ctx context.Context) error {
	if err := toolscheck.Check(toolscheck.DefaultRun()); err != nil {
		return err
	}
	log := logging.GetLogger(ctx)
	if err := a.LoadSigning(); err != nil {
		return err
	}
	if err := a.WriteFDroidConfig(); err != nil {
		return err
	}
	if err := a.FetchTools(ctx); err != nil {
		return err
	}

	jobs, err := a.ListJobs()
	if err != nil {
		return err
	}
	log.Info("jobs loaded", "count", len(jobs), "repo", a.WS.Repo, "cache", a.WS.Cache)

	limits := taskgroup.DefaultLimits()
	if a.Cfg.PoolIO > 0 {
		limits.IO = a.Cfg.PoolIO
	}
	if a.Cfg.PoolCPU > 0 {
		limits.CPU = a.Cfg.PoolCPU
	} else {
		limits.CPU = max(runtime.NumCPU(), 1)
	}
	if a.Cfg.PoolInternet > 0 {
		limits.Internet = a.Cfg.PoolInternet
	}
	_ = limits // pools applied by existing session; optional re-enter not needed

	type item struct{ Job revanced.Job }
	items := make([]item, len(jobs))
	for i, j := range jobs {
		items[i] = item{Job: j}
	}

	err = taskgroup.Each[item]{
		Name:     "packages",
		Items:    items,
		PoolKind: taskgroup.Internet,
		TaskName: func(_ int, it item) string { return it.Job.PackageID },
		Fn: func(ctx context.Context, s *taskgroup.Status, it item) error {
			s.Update(it.Job.PackageID)
			err := taskgroup.Isolate(ctx, func(ctx context.Context) error {
				return a.ProcessPackage(ctx, it.Job)
			})
			if err != nil {
				logging.GetLogger(ctx).Warn("skip package", "package", it.Job.PackageID, "err", err)
			}
			return nil
		},
	}.Run(ctx)
	if err != nil {
		return err
	}

	log.Info("running fdroid update", "repo", a.WS.Repo)
	if err := a.FDroidUpdate(true); err != nil {
		return err
	}
	log.Info("done", "repo", a.WS.Repo)
	return nil
}

// RunSmoke tries packages until maxOK succeed (or list exhausted). For TMP e2e.
func (a *App) RunSmoke(ctx context.Context, maxOK int) (ok int, err error) {
	if err := toolscheck.Check(toolscheck.DefaultRun()); err != nil {
		return 0, err
	}
	if err := a.LoadSigning(); err != nil {
		return 0, err
	}
	if err := a.WriteFDroidConfig(); err != nil {
		return 0, err
	}
	if err := a.FetchTools(ctx); err != nil {
		return 0, err
	}
	jobs, err := a.ListJobs()
	if err != nil {
		return 0, err
	}
	log := logging.GetLogger(ctx)
	if maxOK <= 0 {
		maxOK = 1
	}
	for _, job := range jobs {
		if ok >= maxOK {
			break
		}
		// Prefer non-huge packages
		low := strings.ToLower(job.PackageID)
		if strings.Contains(low, "youtube") || strings.Contains(low, "photos") {
			continue
		}
		log.Info("smoke try", "package", job.PackageID)
		if err := a.ProcessPackage(ctx, job); err != nil {
			log.Warn("smoke skip", "package", job.PackageID, "err", err)
			continue
		}
		ok++
	}
	if ok == 0 {
		return 0, fmt.Errorf("no package succeeded download+patch (tried %d jobs)", len(jobs))
	}
	if err := a.FDroidUpdate(true); err != nil {
		return ok, err
	}
	return ok, nil
}

func emptyAsLatest(v string) string {
	if v == "" {
		return "latest"
	}
	return v
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, s)
}
