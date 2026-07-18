# revancedbot

Builds a **simple binary F-Droid repository** of **ReVanced-patched** apps.

**Status:** Go implementation in progress (see [SPEC.md](./SPEC.md)). The Python package under `revancedbot/` is a legacy prototype.

## Quick start (dev)

```bash
mise install
go build -o bin/revancedbot ./cmd/revancedbot

# one pasteable secret for GHA / env
bin/revancedbot keys generate
export REVANCEDBOT_SIGNING='…'

bin/revancedbot fetch-tools --workspace .revancedbot
bin/revancedbot list-jobs --workspace .revancedbot
# bin/revancedbot run --workspace .revancedbot
```

Consumer repos pin `revancedbot` from GitHub Releases via mise, provide `revancedbot.yaml`, and deploy the written F-Droid tree themselves.

## Spec

Product and toolchain decisions: **[SPEC.md](./SPEC.md)**.
