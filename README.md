# revancedbot

Builds a **simple binary F-Droid repository** of **ReVanced-patched** apps.

**Status:** Go implementation in progress (see [SPEC.md](./SPEC.md)). The Python package under `revancedbot/` is a legacy prototype.

## Quick start (dev)

```bash
mise install
mise run build

# one pasteable secret for GHA / env
bin/revancedbot keys generate
export REVANCEDBOT_SIGNING='…'

bin/revancedbot fetch-tools --workspace .revancedbot
bin/revancedbot list-jobs --workspace .revancedbot
# bin/revancedbot run --workspace .revancedbot
```

### Tests

```bash
mise run test       # unit tests
mise run test:e2e   # end-to-end (java/keytool + network)
```

E2E env knobs:

| Variable | Meaning |
|----------|---------|
| `GITHUB_TOKEN` | GitHub API rate limits |
| `REVANCEDBOT_PATCHES_FILE` | Local `.rvp` if GitHub blocks `revanced-patches` (DMCA 451) |
| `REVANCEDBOT_PATCHES_REPO` | Alternate `owner/repo` for patches releases |
| `REVANCEDBOT_E2E_PACKAGE` | Force package id for download/patch step |
| `REVANCEDBOT_E2E_STRICT=1` | Fail soft steps (APKPure / fdroid) instead of skipping |

Consumer repos pin `revancedbot` from GitHub Releases via mise, provide `revancedbot.yaml`, and deploy the written F-Droid tree themselves.

## Spec

Product and toolchain decisions: **[SPEC.md](./SPEC.md)**.
