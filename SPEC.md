# revancedbot — product & technical spec

Status: agreed (product grill + toolchain grill, 2026-07-18).  
Python under `revancedbot/` is a prototype only; the target implementation is **Go**.

## 1. Goal

Build and maintain an **F-Droid simple binary repository** of **ReVanced-patched** Android apps:

1. Resolve jobs from ReVanced patches  
2. Download stock APKs (pluggable sources)  
3. Patch with ReVanced CLI (defaults/recommended + forced package rename)  
4. Sign APKs (ReVanced CLI + operator key) and the F-Droid index (`fdroid update` + same key)  
5. Write a complete F-Droid tree to disk (git/Pages deploy is **not** this tool’s job)

Primary hard problem: **reliable, pluggable APK downloaders** under hostile/unstable sources.

## 2. Two-repo architecture

| Repository | Role |
|------------|------|
| **`revancedbot` (this repo, public)** | Implementation: Go module, CLI, downloaders, patch orchestration, F-Droid helpers, **GoReleaser** release assets for mise |
| **Consumer repo (later)** | GHA schedule, secrets, root YAML, durable workspace/F-Droid tree, **how** that tree becomes Pages/branches — owned by the consumer, not the bot |

Consumer visibility (public vs private) is **deferred**. The **tool** is public.

## 3. Runtime & CLI

| Decision | Choice |
|----------|--------|
| Language | **Go** (rewrite) |
| Binary name | **`revancedbot`** |
| CLI | **Cobra** subcommands + **Viper** config |
| Parallelism | [`workspaced/pkg/taskgroup`](https://github.com/lucasew/workspaced) — pools, `Map`/`Each`, **`Isolate`/`GoIsolated` for skip-on-fail** |
| Logging | [`workspaced/pkg/logging`](https://github.com/lucasew/workspaced) — **`slog` logger in context** (`ContextWithLogger` / `GetLogger`) |
| GitHub API | [`google/go-github`](https://github.com/google/go-github) (ReVanced release assets; token when present) |
| Browser automation | [**go-rod**](https://github.com/go-rod/rod) against a **CDP URL** (no in-process browser launch in production) |
| Releases of this tool | **GoReleaser** → GitHub Releases → consumer mise pin |

### 3.1 Subcommands (target surface)

```text
revancedbot
  keys generate          # keytool under the hood; print one pasteable secret blob
  keys validate          # validate blob / materialize keystore into workspace
  fetch-tools            # download latest ReVanced CLI + patches (go-github)
  list-jobs              # packages × version preference order
  download               # run downloaders → stock APK dir
  patch                  # patch + sign APKs (operator keystore)
  fdroid-init            # scaffold simple binary repo layout if missing
  fdroid-update          # stage APKs, merge metadata, run `fdroid update`
  run                    # full pipeline (what cron calls)
  version
```

No **`publish`** that talks to git/Pages. Bot **only writes files**. Cron: `revancedbot run` after `mise install`.

### 3.2 Config

- **Primary:** `revancedbot.yaml` at the **root of the consumer repo** (branding, downloader order, pool limits, paths, log level, optional `browser.cdp_url`).
- **Secrets via env:** e.g. `REVANCEDBOT_SIGNING` (paste blob), `GITHUB_TOKEN` for GitHub rate limits, `REVANCEDBOT_CDP_URL` (or yaml) for rod.
- **Flags** for overrides (`--workspace`, dry-run, etc.).

### 3.3 taskgroup usage

- Cobra: `Session.Enter` / `Close` around command lifetime.
- Fan-out packages with `Map`/`Each`; resource pools (`Internet` downloads, `CPU` patch).
- Per-package failures use **`Isolate`** so one failure does not cancel siblings.
- Default pool limits follow taskgroup defaults unless config overrides (IO=4, CPU=NumCPU, Internet=4).

### 3.4 Module dependency

Import from [lucasew/workspaced](https://github.com/lucasew/workspaced):

- `workspaced/pkg/taskgroup`
- `workspaced/pkg/logging`

Note: upstream `go.mod` currently declares `module workspaced`; require/replace may be needed until the module path is published cleanly.

## 4. Toolchain (what runs on the machine)

### 4.1 mise tools (consumer — **all versions pinned**)

| Tool | Role |
|------|------|
| `java` (**Temurin 21.x** pin) | ReVanced CLI, `keytool` |
| `uv` (pinned) | Enables mise `pipx:` backend via **uv** (`pipx.uvx`, default true) |
| `android-sdk` (pinned) | What **fdroidserver** needs for simple binary repos (esp. **aapt**) |
| `pipx:fdroidserver` (pinned) | Puts **`fdroid` on PATH** (not raw `uvx` from the bot) |
| `revancedbot` (pinned to a release) | This project’s binary from GitHub Releases |

**Optional / not default:** Chrome, chromedriver — production browser path is **Browserless + CDP URL**.

**Dev repo (this project):** may pin `go`, `goreleaser`, etc. for building/releasing; separate from consumer pins.

Example consumer sketch (versions illustrative — real pins go in the consumer `mise.toml`):

```toml
[tools]
java = "temurin-21.0.x"          # exact pin
uv = "x.y.z"
android-sdk = "…"
"pipx:fdroidserver" = "2.x.y"
revancedbot = "0.1.0"
```

Renovate (or similar) bumps pins via PRs.

### 4.2 Not installed via mise

| Piece | Role |
|-------|------|
| **Browserless** (or equivalent) GHA **service container** | Headless Chromium; bot only receives CDP/WebSocket URL |
| **ReVanced CLI `.jar` + patches `.rvp`** | Downloaded by the bot every run — **always latest** |

### 4.3 Processes the bot execs

| Binary | Purpose |
|--------|---------|
| `keytool` | Hidden behind `keys generate` / `keys validate` |
| `java -jar revanced-cli.jar` | `list-versions`, **patch + sign APKs** with operator keystore |
| `fdroid` | **Simple binary repo** only: scaffold + **`fdroid update`** (never `fdroid build`) |

### 4.4 In-process Go libraries (selected)

| Library | Role |
|---------|------|
| Cobra / Viper | CLI + config |
| workspaced taskgroup + logging | Parallelism + ctx logger |
| google/go-github | GitHub Releases API |
| go-rod | Browser downloaders when CDP URL is set |
| stdlib `net/http`, `os/exec` | HTTP downloaders, subprocesses |

### 4.5 Pipeline data flow

```text
mise install
→ revancedbot run
  → validate signing blob → materialize keystore + fdroid config.yml bits
  → fetch latest revanced-cli + patches (go-github)
  → list jobs (all packages; version preference order)
  → for each package (Isolate): download → patch+sign → stage
  → write/merge metadata (minimal + patches footer)
  → fdroid update
→ stop (files on disk; consumer deploys)
```

## 5. Job selection & failure policy

| Rule | Detail |
|------|--------|
| Package set | **All** packages advertised by ReVanced patches |
| Versions | **One build per package per run**: walk “most common compatible” **top → bottom** |
| Success | First version that downloads and patches successfully wins |
| Failure | **Skip** package; record reason; continue |
| Rebuild | **Full** pipeline work every scheduled run |
| Same versionCode | If stock-derived versionCode already published for that patched id → **ignore** (no fake Android updates) |
| History | **Accumulate** distinct versionCodes until cleanup is designed later |

## 6. Downloaders

Pluggable interface. First real source can follow the APKPure prototype path; more sources later.

### 6.1 Contract

**Input:** stock `package_id`, exact `version` (optional hints: abi, dpi, locale).

**Output:** path + light metadata (source id, optional URL, sha256).

**Trust model:** **trust the source**. No hard package forensics in the downloader; bad files fail at patch and are skipped.

**Artifact preference:**

1. Single **APK** over XAPK / split APKS  
2. **Universal / multi-ABI** over niche ABI  
3. Broad DPI / nodpi over device-specific  

### 6.2 Implementation style

**Hybrid per source:**

- Prefer plain HTTP  
- **go-rod + CDP URL** when a source needs a browser  
- GHA: start **Browserless** (service container); pass URL into the bot  
- Local: same URL pattern, or disable browser downloaders if unset  

### 6.3 Orchestration order

1. For each preferred version (top → bottom)  
2. For each downloader in order: try download  
3. Download OK → patch  
4. Patch fail → next version  
5. Exhausted → skip package  

## 7. Patching

| Rule | Detail |
|------|--------|
| Patch set | ReVanced **defaults / recommended** |
| Package rename | **Always** enable rename patch |
| Rename rule | Default: **append `.revanced`** |
| Identity | Stock id for jobs/download; **`.revanced`** id in F-Droid |
| APK signing | **ReVanced CLI during patch** with operator keystore from secret blob |
| ReVanced tool versions | **Always latest** CLI + patches each run |

## 8. Metadata (F-Droid listing)

| Field kind | Policy (v1) |
|------------|-------------|
| Marketing scrapers (Play, APKMirror, …) | **Deferred** — not reliable enough to promise in v1 |
| Base listing | From APK + **`fdroid update`** / create-metadata (`-c` as needed) |
| Description | Always include **Patches applied:** from the successful patch run |
| Scrapers later | Pluggable providers can be added; try every run + last-good cache when they exist |

## 9. Signing

1. `revancedbot keys generate` → runs **`keytool`** internally → prints **one text blob**.  
2. Paste blob into one GHA secret (e.g. `REVANCEDBOT_SIGNING`).  
3. On run: **validate** blob or refuse to start.  
4. Materialize keystore + fdroid `config.yml` key fields.  
5. **Same key** signs **all APKs** (via ReVanced CLI) and the **F-Droid index** (via fdroidserver).

User never manages keytool flags in the happy path.

## 10. F-Droid repo output

| Piece | Choice |
|-------|--------|
| Mode | **Simple binary repository only** (no `fdroid build`) |
| Index tool | **`fdroid`** from mise `pipx:fdroidserver` |
| Bot output | Complete tree on disk (`repo/`, `metadata/`, indexes, …) |
| Deploy | **Out of scope** for the bot (branches/Pages = consumer) |
| Cleanup | Not in v1 |

## 11. Consumer CI (target)

```yaml
# Conceptual — lives in the consumer repo
on:
  schedule:
    - cron: "0 2 * * 6"   # Saturday 02:00 UTC
  workflow_dispatch:

jobs:
  build-repo:
    runs-on: ubuntu-latest
    services:
      browserless:
        image: ghcr.io/browserless/chromium  # pin digest/tag in real workflow
        ports:
          - 3000:3000
    steps:
      - checkout
      - mise install                 # all tools pinned in mise.toml
      - revancedbot run
        env:
          REVANCEDBOT_SIGNING: ${{ secrets.REVANCEDBOT_SIGNING }}
          REVANCEDBOT_CDP_URL: ws://localhost:3000
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      # consumer-owned: commit / pages deploy / rsync / …
```

## 12. Distribution of this tool

- **GoReleaser** publishes multi-arch binaries to GitHub Releases.  
- Consumer pins `revancedbot = "x.y.z"` in mise.  
- ReVanced jars/patches are **never** inside the release artifact.

## 13. Explicit non-goals (v1)

- Incremental skip of download/patch work  
- Per-app custom patch matrices  
- Automatic cleanup of historical APKs  
- Consumer public-vs-private decision  
- Hard integrity verification inside downloaders  
- Bot-owned git/Pages publishing  
- Reliable Play/APKMirror marketing scrapers  
- Multi-tenant hosted SaaS  
- `fdroid build` / full buildserver mode  

## 14. Gap map (current tree → target)

| Area | Today (prototype) | Target |
|------|-------------------|--------|
| Language | Python | Go + Cobra/Viper |
| Scope | Dump APKs to Pages from this repo | Simple binary F-Droid tree; deploy elsewhere |
| Jobs | Many versions per package | One package → version walk → one success |
| Failures | bare `except` | Isolate + structured skip |
| Download | Selenium local Chrome | HTTP + rod/CDP (Browserless on GHA) |
| F-Droid | unused `fdroidserver` dep | mise `pipx:fdroidserver` + `fdroid update` |
| Keys | none | keytool abstracted → paste secret |
| Metadata | none | APK/fdroid + patches footer; scrapers later |
| Parallelism | sequential | workspaced taskgroup |
| Logging | logging module ad hoc | workspaced ctx logging |
| Releases | none | GoReleaser + pinned mise |

## 15. Suggested implementation order

1. Go module scaffold, Cobra, Viper, workspaced logging/taskgroup wiring  
2. `keys generate` / `validate` (keytool + blob)  
3. `fetch-tools` + `list-jobs` (go-github, always-latest ReVanced)  
4. Downloader interface + first HTTP source; rod path behind CDP URL  
5. `patch` (defaults, `.revanced`, operator keystore)  
6. Simple binary `fdroid-init` / `fdroid-update`  
7. `run` + Isolate fan-out  
8. GoReleaser + example consumer `mise.toml` / workflow sketch  
9. (Later) marketing scrapers + more downloaders  

## 16. Open implementation details (non-blocking)

- Exact `revancedbot.yaml` schema  
- Exact secret blob JSON schema (`v: 1`, …)  
- Exact Browserless image pin and CDP path/token  
- First downloader source priority list  
- Go module path (`github.com/lucasew/revancedbot` expected)  
- workspaced require/replace until module path is clean  
- How much metadata `fdroid update -c` generates vs what we write by hand  
