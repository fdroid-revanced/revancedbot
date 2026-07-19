package app

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/lucasew/revancedbot/internal/apkmeta"
	"github.com/lucasew/revancedbot/internal/config"
	"github.com/lucasew/revancedbot/internal/download"
	"github.com/lucasew/revancedbot/internal/fdroid"
	"github.com/lucasew/revancedbot/internal/netx"
	"github.com/lucasew/revancedbot/internal/revanced"
	"github.com/lucasew/revancedbot/internal/signing"
	"github.com/lucasew/revancedbot/internal/toolscheck"
	"github.com/lucasew/revancedbot/internal/workspace"
	"github.com/lucasew/workspaced/pkg/logging"
	"github.com/lucasew/workspaced/pkg/taskgroup"
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

// PrepareStage seeds CACHE/fdroid from live REPO (history) and writes stage config.yml.
// No live REPO mutation.
func (a *App) PrepareStage() error {
	if a.Blob == nil {
		return fmt.Errorf("signing not loaded")
	}
	if err := fdroid.SeedStage(a.WS.Stage, a.WS.Repo); err != nil {
		return err
	}
	return fdroid.WriteConfig(a.WS.StageConfig(), fdroid.RepoMeta{
		Name:        a.Cfg.RepoName,
		URL:         a.Cfg.RepoURL,
		Description: a.Cfg.RepoDescription,
	}, a.WS.KeystorePath, a.Blob)
}

// WriteFDroidConfig is an alias for PrepareStage (stage-only config).
func (a *App) WriteFDroidConfig() error {
	return a.PrepareStage()
}

// PublishStage atomically replaces REPO/{repo,metadata,config.yml} from CACHE/fdroid.
func (a *App) PublishStage() error {
	return fdroid.Publish(a.WS.Stage, a.WS.Repo)
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
// No nested Control Maps — progress for stock APKs is owned by the parent
// "apks" Map in RunFull (plus httpclient fetch bars for network).
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
		if err := a.processVersion(ctx, job, ver, reg, order); err != nil {
			lastErr = err
			log.Warn("version failed", "package", job.PackageID, "version", emptyAsLatest(ver), "err", err)
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no versions to try")
	}
	return fmt.Errorf("skip %s: %w", job.PackageID, lastErr)
}

func (a *App) processVersion(ctx context.Context, job revanced.Job, ver string, reg download.Registry, order []string) error {
	log := logging.GetLogger(ctx)
	// Request label for downloaders (empty = source "latest" / newest release).
	reqVer := ver
	stockPath := a.WS.StockAPKPath(job.PackageID, reqVer)
	var res *download.Result

	if workspace.CacheHit(stockPath) {
		if err := download.AcceptCached(stockPath); err != nil {
			log.Warn("stock cache rejected", "path", stockPath, "err", err)
		} else {
			log.Info("stock cache hit", "path", stockPath)
			res = &download.Result{Path: stockPath, SourceID: "cache"}
		}
	}
	if res == nil {
		label := reqVer
		if label == "" {
			label = "latest"
		}
		log.Info("download attempt", "package", job.PackageID, "version", label)
		got, err := download.FetchFirst(netx.WithLabel(ctx, "download stock "+job.PackageID), reg, order, download.Request{
			PackageID: job.PackageID,
			Version:   reqVer,
		}, a.WS.StockAPKs)
		if err != nil {
			return err
		}
		res = got
	}

	// Ground truth: versionName from the APK. "Any"/latest downloads must not
	// stay labeled "latest" in the F-Droid tree.
	resolved := reqVer
	if info, err := apkmeta.Inspect(res.Path); err != nil {
		log.Warn("apk version inspect failed; using request label", "err", err, "label", emptyAsLatest(reqVer))
		if resolved == "" {
			resolved = "latest"
		}
	} else {
		if info.VersionName != "" {
			resolved = info.VersionName
		} else if info.VersionCode != "" {
			resolved = info.VersionCode
		}
		if reqVer != "" && info.VersionName != "" && !versionMatches(reqVer, info.VersionName) {
			log.Warn("requested version differs from APK", "want", reqVer, "got", info.VersionName)
		}
		log.Info("resolved apk version", "package", job.PackageID, "versionName", info.VersionName, "versionCode", info.VersionCode)
	}

	// Canonical stock cache path under the real version name.
	canonStock := a.WS.StockAPKPath(job.PackageID, resolved)
	if res.Path != canonStock {
		if err := moveFile(res.Path, canonStock); err != nil {
			return fmt.Errorf("canonicalize stock apk: %w", err)
		}
		res.Path = canonStock
	}

	outName := fmt.Sprintf("%s_%s_revanced.apk", sanitize(job.PackageID), sanitize(resolved))
	outPath := filepath.Join(a.WS.Work, outName)
	if err := os.MkdirAll(a.WS.Work, 0o755); err != nil {
		return err
	}

	var patches []string
	err := taskgroup.GoIsolated(ctx, "patch "+job.PackageID, taskgroup.CPU, func(ctx context.Context, s *taskgroup.Status) error {
		defer s.Unit()()
		s.Update("ReVanced CLI")
		log.Info("patching", "in", res.Path, "out", outPath, "version", resolved)
		ps, err := revanced.Patch(revanced.PatchOptions{
			CLIJar:                  a.WS.PatcherJAR(),
			PatchesRVP:              a.WS.PatchesRVP(),
			InputAPK:                res.Path,
			OutputAPK:               outPath,
			KeystorePath:            a.WS.KeystorePath,
			Blob:                    a.Blob,
			EnableChangePackageName: true,
		})
		if err != nil {
			return err
		}
		patches = ps
		return nil
	})
	if err != nil {
		return err
	}

	pubID := job.PackageID + ".revanced"
	if err := taskgroup.GoIsolated(ctx, "stage "+job.PackageID, taskgroup.IO, func(ctx context.Context, s *taskgroup.Status) error {
		defer s.Unit()()
		s.Update("copy into F-Droid stage")
		if err := fdroid.StageAPK(a.WS.Stage, outPath); err != nil {
			return err
		}
		return fdroid.WritePatchesMetadata(a.WS.Stage, pubID, patches)
	}); err != nil {
		return err
	}
	log.Info("package ok", "package", job.PackageID, "version", resolved, "apk", outPath)
	return nil
}

// versionMatches reports whether a request label agrees with APK versionName.
func versionMatches(want, got string) bool {
	want = strings.TrimSpace(want)
	got = strings.TrimSpace(got)
	if want == "" || got == "" {
		return true
	}
	if want == got {
		return true
	}
	return strings.HasPrefix(got, want) || strings.HasPrefix(want, got)
}

func moveFile(src, dst string) error {
	if src == dst {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return err
	}
	_ = os.Remove(src)
	return nil
}

// FDroidUpdate runs fdroid update on the CACHE stage tree (not live REPO).
func (a *App) FDroidUpdate(ctx context.Context, createMeta bool) error {
	if a.Blob == nil {
		return fmt.Errorf("signing not loaded")
	}
	return taskgroup.GoIsolated(ctx, "rebuild F-Droid index", taskgroup.IO, func(ctx context.Context, s *taskgroup.Status) error {
		defer s.Unit()()
		s.Update("fdroid update")
		return fdroid.Update(a.WS.Stage, a.Blob, createMeta, a.WS.Shims)
	})
}

// pkgOutcome is a pure reduce element from the packages Map (no shared mutex).
type pkgOutcome struct {
	Package string
	OK      bool
	Skip    string // non-empty when soft-skipped
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
	// Shuffle so rate limits (403/429) and early aborts don't always starve the
	// same packages; over many runs every app gets a fair shot at updates.
	rand.Shuffle(len(jobs), func(i, j int) {
		jobs[i], jobs[j] = jobs[j], jobs[i]
	})
	log.Info("jobs loaded", "count", len(jobs), "repo", a.WS.Repo, "cache", a.WS.Cache)

	// One aggregate bar for all package APK work. Pure reduce after soft-skips.
	outcomes, err := taskgroup.Map[revanced.Job, pkgOutcome]{
		Name:     "packages",
		Items:    jobs,
		PoolKind: taskgroup.Control,
		TaskName: func(_ int, j revanced.Job) string { return j.PackageID },
		Fn: func(ctx context.Context, s *taskgroup.Status, job revanced.Job) (pkgOutcome, error) {
			s.Update("process " + job.PackageID)
			err := taskgroup.Isolate(ctx, func(ctx context.Context) error {
				return a.ProcessPackage(ctx, job)
			})
			if err != nil {
				logging.GetLogger(ctx).Warn("skip package", "package", job.PackageID, "err", err)
				return pkgOutcome{Package: job.PackageID, Skip: err.Error()}, nil
			}
			return pkgOutcome{Package: job.PackageID, OK: true}, nil
		},
	}.Run(ctx)
	if err != nil {
		return err
	}

	var okPkgs, skipPkgs []string
	for _, o := range outcomes {
		if o.OK {
			okPkgs = append(okPkgs, o.Package)
			continue
		}
		if o.Skip != "" {
			skipPkgs = append(skipPkgs, o.Package+": "+o.Skip)
		}
	}

	summarize := func() {
		log.Info("run summary",
			"ok", len(okPkgs),
			"skipped", len(skipPkgs),
			"ok_packages", strings.Join(okPkgs, ","),
		)
		for _, line := range skipPkgs {
			log.Info("skipped", "detail", line)
		}
	}
	if s := taskgroup.SessionFrom(ctx); s != nil {
		s.AfterWait(func() error {
			summarize()
			return nil
		})
	} else {
		summarize()
	}

	log.Info("running fdroid update", "stage", a.WS.Stage)
	if err := a.FDroidUpdate(ctx, true); err != nil {
		return err
	}
	log.Info("publishing stage to REPO", "stage", a.WS.Stage, "repo", a.WS.Repo)
	if err := a.PublishStage(); err != nil {
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

	// Filter candidates, shuffle so each smoke run picks a different starting app,
	// then Serial Each until maxOK succeed (stop scheduling via atomic).
	var candidates []revanced.Job
	for _, job := range jobs {
		low := strings.ToLower(job.PackageID)
		if strings.Contains(low, "youtube") || strings.Contains(low, "photos") {
			continue
		}
		candidates = append(candidates, job)
	}
	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})
	if len(candidates) > 0 {
		log.Info("smoke order", "first", candidates[0].PackageID, "candidates", len(candidates), "max_ok", maxOK)
	}

	var okCount atomic.Int64
	err = taskgroup.Each[revanced.Job]{
		Name:     "smoke packages",
		Items:    candidates,
		PoolKind: taskgroup.Control,
		Serial:   true,
		TaskName: func(_ int, j revanced.Job) string { return j.PackageID },
		Fn: func(ctx context.Context, s *taskgroup.Status, job revanced.Job) error {
			if okCount.Load() >= int64(maxOK) {
				return nil
			}
			s.Update(job.PackageID)
			log.Info("smoke try", "package", job.PackageID)
			err := taskgroup.Isolate(ctx, func(ctx context.Context) error {
				return a.ProcessPackage(ctx, job)
			})
			if err != nil {
				log.Warn("smoke skip", "package", job.PackageID, "err", err)
				return nil
			}
			okCount.Add(1)
			return nil
		},
	}.Run(ctx)
	if err != nil {
		return 0, err
	}
	ok = int(okCount.Load())
	if ok == 0 {
		return 0, fmt.Errorf("no package succeeded download+patch (tried %d jobs)", len(jobs))
	}
	if err := a.FDroidUpdate(ctx, true); err != nil {
		return ok, err
	}
	if err := a.PublishStage(); err != nil {
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
