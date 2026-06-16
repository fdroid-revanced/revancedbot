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

## IGNORE: Upgrading GitHub Actions to Non-Existent Versions

**- Pattern:** Bumping GitHub Actions versions to non-existent major versions (e.g., `actions/checkout@v6`).
**- Justification:** Bumping to fabricated or unreleased versions causes CI workflow failures. Dependency downgrades or invalid version upgrades are consistently rejected.
**- Files Affected:** `.github/workflows/*.yaml`, `.github/workflows/*.yml`

## IGNORE: Breaking Static Cache with Random Temp Directories

**- Pattern:** Replacing hardcoded or static temporary directory paths (e.g., `/tmp/revancedbot`) with randomized paths like `tempfile.mkdtemp()` to fix predictable temp directory vulnerabilities.
**- Justification:** The project relies on the static `/tmp/revancedbot` directory to cache downloaded ReVanced tools and fetched APKs across executions. Changing it to a randomized path breaks this expected caching behavior.
**- Files Affected:** `revancedbot/__init__.py`, `revancedbot/app.py`, `revancedbot/patcher.py`
