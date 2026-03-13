# Agents

- `revancedbot/__init__.py` -> Main entrypoint and business logic for patching.
- `revancedbot/errors.py` -> Centralized error reporting function (`report_error()`).
- `.github/workflows/autorelease.yml` -> Unified CI/CD workflow containing the single job.
- `mise.toml` -> Tools, environment setup, and unified task runner configurations.

## Architecture Guidelines
- Centralized Error Reporting: Use `revancedbot/errors.py:report_error()` for all unhandled exceptions. Empty `except` blocks are strictly forbidden.
- Testing: Tests are located in the `tests` directory (if they exist) and executed via `uv run pytest`.
