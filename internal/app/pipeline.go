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
	"github.com/lucasew/revancedbot/internal/workspace"
	"workspaced/pkg/logging"
	"workspaced/pkg/taskgroup"
)

// App wires config + workspace for CLI commands.
type App struct {
	Cfg  *config.Config
	WS   *workspace.Layout
	Blob *signing.Blob
}

func New(cfg *config.Config) (*App, error) {
	ws := workspace.New(cfg.Workspace)
	if err := ws.Ensure(); err != nil {
		return nil, err
	}
	return &App{Cfg: cfg, WS: ws}, nil
}

// LoadSigning materializes the signing blob from config.
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

// FetchTools downloads latest ReVanced CLI + patches.
func (a *App) FetchTools(ctx context.Context) error {
	log := logging.GetLogger(ctx)
	log.Info("fetching latest ReVanced CLI and patches")
	return revanced.FetchLatest(ctx, a.Cfg.GitHubToken, a.WS.PatcherJAR(), a.WS.PatchesRVP())
}

// ListJobs returns patch jobs.
func (a *App) ListJobs() ([]revanced.Job, error) {
	if _, err := os.Stat(a.WS.PatcherJAR()); err != nil {
		return nil, fmt.Errorf("missing CLI jar; run fetch-tools first: %w", err)
	}
	return revanced.ListJobs("java", a.WS.PatcherJAR(), a.WS.PatchesRVP())
}

// ProcessPackage downloads and patches one package (version walk).
func (a *App) ProcessPackage(ctx context.Context, job revanced.Job) error {
	log := logging.GetLogger(ctx)
	reg := download.DefaultRegistry()
	order := a.Cfg.DownloaderOrder
	if len(order) == 0 {
		order = []string{"apkpure"}
	}

	versions := job.Versions
	if len(versions) == 0 {
		versions = []string{""}
	}

	var lastErr error
	for _, ver := range versions {
		req := download.Request{PackageID: job.PackageID, Version: ver}
		log.Info("download attempt", "package", job.PackageID, "version", emptyAsLatest(ver))
		res, err := download.FetchFirst(ctx, reg, order, req, a.WS.StockAPKs)
		if err != nil {
			lastErr = err
			log.Warn("download failed", "err", err)
			continue
		}
		outName := fmt.Sprintf("%s_%s_revanced.apk", sanitize(job.PackageID), sanitize(emptyAsLatest(ver)))
		outPath := filepath.Join(a.WS.PatchedAPKs, outName)
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
		// Stage into fdroid tree
		pubID := job.PackageID + ".revanced"
		if err := fdroid.StageAPK(a.WS.FDroid, outPath); err != nil {
			return err
		}
		if err := fdroid.WritePatchesMetadata(a.WS.FDroid, pubID, patches); err != nil {
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

// EnsureFDroidConfig writes config.yml for fdroidserver.
func (a *App) EnsureFDroidConfig() error {
	if a.Blob == nil {
		return fmt.Errorf("signing not loaded")
	}
	if err := fdroid.EnsureLayout(a.WS.FDroid); err != nil {
		return err
	}
	// Copy keystore into fdroid dir for relative path simplicity
	ksInFDroid := filepath.Join(a.WS.FDroid, "keystore.p12")
	bin, err := os.ReadFile(a.WS.KeystorePath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(ksInFDroid, bin, 0o600); err != nil {
		return err
	}
	return fdroid.WriteConfig(a.WS.FDroidConfig(), fdroid.RepoMeta{
		Name:        a.Cfg.RepoName,
		URL:         a.Cfg.RepoURL,
		Description: a.Cfg.RepoDescription,
	}, ksInFDroid, a.Blob)
}

// FDroidUpdate runs fdroid update -c.
func (a *App) FDroidUpdate(createMeta bool) error {
	return fdroid.Update(a.WS.FDroid, createMeta)
}

// RunFull executes the full pipeline with taskgroup Isolate per package.
func (a *App) RunFull(ctx context.Context) error {
	log := logging.GetLogger(ctx)
	if err := a.LoadSigning(); err != nil {
		return err
	}
	if err := a.FetchTools(ctx); err != nil {
		return err
	}
	if err := a.EnsureFDroidConfig(); err != nil {
		return err
	}

	jobs, err := a.ListJobs()
	if err != nil {
		return err
	}
	log.Info("jobs loaded", "count", len(jobs))

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

	// Session should already be entered by CLI; use group from ctx.
	g := taskgroup.MustFromContext(ctx)

	type item struct {
		Job revanced.Job
	}
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
			// Return nil on failure so Map does not abort siblings (skip policy).
			// Isolate keeps any nested group errors from cancelling the parent session.
			err := taskgroup.Isolate(ctx, func(ctx context.Context) error {
				return a.ProcessPackage(ctx, it.Job)
			})
			if err != nil {
				logging.GetLogger(ctx).Warn("skip package", "package", it.Job.PackageID, "err", err)
				return nil
			}
			return nil
		},
	}.Run(ctx)
	if err != nil {
		return err
	}
	_ = g

	log.Info("running fdroid update")
	if err := a.FDroidUpdate(true); err != nil {
		return err
	}
	log.Info("done", "fdroid_root", a.WS.FDroid)
	return nil
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
