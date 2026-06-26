## IGNORE: Moving Root Module to src/ Directory

**- Pattern:** Moving the `revancedbot/` directory or its contents into a `src/` subdirectory during refactoring.
**- Justification:** The project does not use a `src/` layout; the main application codebase must remain structured within the `revancedbot/` directory at the project root.
**- Files Affected:** `revancedbot/*`, `src/revancedbot/*`

## IGNORE: Modifying Executable Logic in Docs PRs

**- Pattern:** Adding or changing functional code, such as implementing centralized error reporting or altering default arguments, while tasked with adding documentation or docstrings.
**- Justification:** Documentation tasks must be strictly limited to improving docs and docstrings. They must never alter executable logic, dependency graphs, or runtime build configurations.
**- Files Affected:** `revancedbot/**/*.py`, `experimentation.ipynb`

## IGNORE: Module-Level Imports Below First Cell in Notebooks

**- Pattern:** Failing to move or adding module-level imports into intermediate or lower cells in Jupyter notebooks (`.ipynb` files).
**- Justification:** Notebooks must not contain module-level imports below the first cell to avoid linting errors and maintain clean execution flow.
**- Files Affected:** `experimentation.ipynb`

## IGNORE: Invalid Dependency Upgrades and Downgrades

**- Pattern:** Bumping GitHub Actions to non-existent versions (e.g., `actions/checkout@v6`) or downgrading dependencies without explicit request (e.g., `actions/checkout@v4`, downgrading tools in `mise.toml`).
**- Justification:** Bumping to fabricated or unreleased versions causes CI workflow failures. Dependency downgrades or invalid version upgrades are consistently rejected.
**- Files Affected:** `.github/workflows/*.yaml`, `.github/workflows/*.yml`, `mise.toml`

## IGNORE: Breaking Static Cache with Random Temp Directories

**- Pattern:** Replacing hardcoded or static temporary directory paths (e.g., `/tmp/revancedbot`) with randomized paths like `tempfile.mkdtemp()` to fix predictable temp directory vulnerabilities.
**- Justification:** The project relies on the static `/tmp/revancedbot` directory to cache downloaded ReVanced tools and fetched APKs across executions. Changing it to a randomized path breaks this expected caching behavior.
**- Files Affected:** `revancedbot/__init__.py`, `revancedbot/app.py`, `revancedbot/patcher.py`

## IGNORE: Out-of-Scope Code Changes in Meta-Agent PRs

**- Pattern:** Meta-agents (like Denoiser) modifying runtime product code, dependencies (e.g., `mise.toml`), or CI workflows.
**- Justification:** Meta-agents are strictly scoped to updating memory, learning prompts, and tracking rejection patterns. Changing unrelated codebase files violates the agent's operational contract.
**- Files Affected:** `mise.toml`, `.github/workflows/*`, `revancedbot/*`

## IGNORE: Adding Unrequested Dependencies in Janitor Tasks

**- Pattern:** Adding new dependencies to `pyproject.toml` or `uv.lock` (e.g., `webdriver-manager`, `python-dotenv`) while tasked with fixing linting errors.
**- Justification:** Janitor tasks must focus on code quality, formatting, and structural best practices without altering the project's dependency graph.
**- Files Affected:** `pyproject.toml`, `uv.lock`
