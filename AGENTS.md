# Agents Conventions

## Architecture Locations

- `revancedbot/app.py` -> The main application flow, tying together downloading and patching.
- `revancedbot/cli.py` -> Command-line interface entry points.
- `revancedbot/errors.py` -> Centralized error reporting infrastructure.
- `revancedbot/fetcher.py` -> Logic for downloading APKs (currently using Apkpure via Selenium).
- `revancedbot/models.py` -> Data structures and types (e.g., `PatchJob`).
- `revancedbot/patcher.py` -> Logic for managing the ReVanced patcher and downloading its tools.

## Centralized Error Handling

All code paths that handle unexpected errors MUST funnel through the centralized error-reporting function in `revancedbot/errors.py:report_error()`.
- Never call `logger.error` directly at the call site for unexpected errors.
- Never use an empty `except:` block. At a minimum, catch `Exception` and pass it to `report_error`, along with any useful context.
