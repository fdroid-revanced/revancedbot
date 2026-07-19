# revancedbot

Builds a **simple binary F-Droid repository** of **ReVanced-patched** apps.

Implementation is **Go**. Product and toolchain decisions: **[SPEC.md](./SPEC.md)**.

## Quick start (dev)

```bash
mise install
mise run build

# one pasteable secret for GHA / env
bin/revancedbot keys generate
export REVANCEDBOT_SIGNING='…'

# F-Droid REPO root contains revancedbot.yaml (authority)
mkdir -p /tmp/demo-fdroid && cp revancedbot.example.yaml /tmp/demo-fdroid/revancedbot.yaml
bin/revancedbot fetch-tools /tmp/demo-fdroid --cache /tmp/rvb-cache
bin/revancedbot list-jobs /tmp/demo-fdroid --cache /tmp/rvb-cache
# bin/revancedbot run /tmp/demo-fdroid   # --cache defaults to mkdtemp
```

### Tests

```bash
mise run test       # unit tests
mise run test:e2e   # end-to-end (java/keytool + network)
```

E2E env knobs:

| Variable | Meaning |
|----------|---------|
| `GITHUB_TOKEN` | GitHub API rate limits (CLI releases) |
| `REVANCEDBOT_PATCHES_FILE` | Local `.rvp` path |
| `REVANCEDBOT_PATCHES_URL` | Direct URL to a `.rvp` |
| `REVANCEDBOT_PATCHES_REPO` | Alternate GitHub `owner/repo` for patches |
| `REVANCEDBOT_E2E_PACKAGE` | Force package id for download/patch step |
| `REVANCEDBOT_E2E_STRICT=1` | Fail soft steps instead of skipping |

Patches: GitHub `ReVanced/revanced-patches` is often DMCA-blocked ([where-is-revanced-patches](https://github.com/ReVanced/where-is-revanced-patches)).  
`fetch-tools` resolves the latest tag from **GitLab** (`gitlab.com/ReVanced/revanced-patches`), tries GitHub assets, then a community SourceForge mirror for the prebuilt `.rvp`.

Consumer repos pin `revancedbot` from GitHub Releases via mise, provide `revancedbot.yaml`, and deploy the written F-Droid tree themselves.
