import logging
import sys
import tempfile
from pathlib import Path
import time
from typing import Optional
from github import Github
from dataclasses import dataclass
import subprocess
from selenium import webdriver
from .errors import report_error
from selenium.webdriver.common.by import By
from selenium.webdriver.chrome.options import Options
from selenium.webdriver.support.ui import WebDriverWait
from tqdm import tqdm

logger = logging.getLogger(__name__)

@dataclass
class PatchJob:
    """
    Represents a specific application version targeted for patching.

    This acts as the fundamental unit of work across the pipeline (discovery -> fetch -> patch).
    By holding `package_id` and an optional `package_version`, it allows targeting
    either a specific known-good release or dynamically fetching the 'latest'.
    """
    package_id: str
    package_version: Optional[str] # latest if None


class ApkpureFetcher():
    """
    Handles headless browser automation to scrape and download APKs from APKPure.

    It configures a dedicated Chrome profile to automatically save files into `location`
    without prompting, sidestepping bot-protections or manual dialogs.
    """
    def __init__(self, location: Path):
        """
        Initializes the Chrome webdriver with automated download settings.

        Args:
            location (Path): The target directory where `.apk` files will be dropped.
        """
        location.mkdir(parents=True, exist_ok=True)
        self.location = location
        prefs = {
            "download.default_directory": str(location.resolve()),
            "download.prompt_for_download": False,
            "download.directory_upgrade": True,
            "safebrowsing.enabled": True # without this it blocks because it can't check, anything better? send a PR please!
        }
        options = Options()
        options.add_argument("--headless=new") # for Chrome >= 109
        options.add_argument("--window-size=1920,1080")
        options.add_experimental_option("prefs", prefs)
        self.driver = webdriver.Chrome(options=options)

    def url_from_job(self, job: PatchJob):
        """Builds the direct APKPure download URL for a given PatchJob."""
        return f"https://d.apkpure.com/b/APK/{job.package_id}?version={job.package_version or 'latest'}"

    def fetch(self, job: PatchJob):
        """
        Triggers the browser to navigate to the download link.

        This initiates the download asynchronously within the browser session.
        It does not block until the download is complete; use `wait_settle` for that.
        """
        self.driver.get(self.url_from_job(job))

    def wait_settle(self):
        """
        Blocks execution until all active Chrome downloads finish.

        It polls the target directory for Chrome's temporary `.crdownload` files,
        waiting until none remain. It also enforces a final 5-second sleep to ensure
        file system handles are released before closing the browser.
        """
        while True:
            logger.info("Checking if downloads are finished")
            pending = list(self.location.glob("*.crdownload"))
            if len(pending) == 0:
                break
            time.sleep(1)
        logger.info("Downloads finished, waiting for 5s")
        time.sleep(5)
        self.driver.close()

class Patcher():
    """
    Wrapper for the external ReVanced Java tooling (CLI and Patches).

    It is responsible for bootstrapping the required `.jar` and `.rvp` assets from GitHub
    releases if they are missing, and executing the patching commands in a subprocess.
    """
    def __init__(self, tool_location: Path =None):
        """
        Sets up the working directory for the ReVanced toolchain.

        If no path is provided, it falls back to a temporary directory.
        """
        if tool_location is None:
            tool_location = Path(tempfile.mkdtemp()).parent / "revancedbot"
        self.tool_location = tool_location
        self._started = None
    
    @property
    def patch_file(self):
        return self.tool_location / "patches.rvp"
    
    @property
    def patcher_file(self):
        return self.tool_location / "patcher.jar"

    def _startup(self):
        """
        Lazy-loads the ReVanced CLI and Patches bundles from GitHub.

        This makes an unauthenticated API call to GitHub to find the latest releases.
        It runs exactly once per instance to avoid unnecessary network latency or rate limits.
        """
        if self._started is not None:
            return
        g = Github()
        self.tool_location.mkdir(parents=True, exist_ok=True)
        if not self.patch_file.exists():
            latest_patch_release = g.get_repo("ReVanced/revanced-patches").get_latest_release()
            patch_asset = [p for p in latest_patch_release.assets if p.name.endswith(".rvp")][0]
            patch_asset.download_asset(self.patch_file)

        if not self.patcher_file.exists():
            latest_patcher_release = g.get_repo("ReVanced/revanced-cli").get_latest_release()
            patcher_asset = [p for p in latest_patcher_release.assets if p.name.endswith(".jar")][0]
            patcher_asset.download_asset(self.patcher_file)
        self._started = True

    def __call__(self, *args, stdin=None, stdout=None, stderr=None):
        """
        Executes a command against the ReVanced CLI jar.

        Args:
            *args: CLI arguments to pass to the jar (e.g., 'patch', '-o', ...).
            stdin, stdout, stderr: File handles for redirecting I/O streams.
        """
        self._startup()
        return subprocess.run(
            ["java", "-jar", self.patcher_file, *args],
            stdin=stdin,
            stdout=stdout,
            stderr=stderr
        )
    
    @property
    def jobs(self):
        """
        Parses the `.rvp` bundle to discover all patchable apps and their compatible versions.

        Yields:
            PatchJob: A job definition for each compatible package/version pair found.
        """
        data = self("list-versions", self.patch_file, stdout=subprocess.PIPE).stdout.decode()
        for package in data.split("Package name: "):
            package_parts = package.split("Most common compatible versions:")
            if len(package_parts) != 2:
                continue
            package_id = package_parts[0].strip()
            rest = package_parts[1]
            for version in rest.split('\n'):
                version = version.strip().split(' ')[0]
                if version == '':
                    continue
                yield PatchJob(package_id=package_id, package_version=None if version == 'Any' else version)

class App:
    """
    High-level orchestrator for the entire fetch-and-patch lifecycle.

    It connects the `Patcher` (for discovering what needs patching and doing the work)
    with the `ApkpureFetcher` (for acquiring the raw APKs). It maintains internal state
    to avoid re-fetching or re-discovering jobs unnecessarily.
    """
    def __init__(self, root=Path("/tmp/revancedbot"), lowlimit=False):
        """
        Initializes the application state and its dependencies.

        Args:
            root (Path): Base directory for tooling, downloaded APKs, and patched outputs.
            lowlimit (bool): If True, restricts the pipeline to only process the first 3 jobs,
                             useful for testing or debugging without doing a full run.
        """
        self.root = root
        self.patcher = Patcher(root/"patcher")
        self.lowlimit = lowlimit
        self._jobs = None
        self._fetched_apks = None

    @property
    def jobs(self):
        """
        Retrieves the list of target patch jobs, lazily querying the Patcher if needed.
        Applies the `lowlimit` slice if configured.
        """
        if self._jobs is None:
            self._jobs = list(self.patcher.jobs)
        if self.lowlimit:
            self._jobs = self._jobs[:3]
        return self._jobs

    @property
    def fetched_apks(self):
        """
        Downloads the raw APKs for all identified jobs.

        It caches the resulting file paths in memory after the first run.
        This property has significant side effects (network I/O, file creation, headless browser execution).
        """
        apk_dir = self.root / "downloaded_apks"
        apk_dir.mkdir(parents=True, exist_ok=True)
        if self._fetched_apks is None:
            fetcher = ApkpureFetcher(apk_dir)
            logger.info("Baixando apks...")
            for job in self.jobs:
                logger.info(f"Baixando {job.package_id}@{job.package_version or "latest"}")
                fetcher.fetch(job)
            fetcher.wait_settle()
            self._fetched_apks = list(apk_dir.iterdir())
        return self._fetched_apks
    
    @property
    def patched_apks(self):
        """
        Iterates over all downloaded APKs and applies the ReVanced patches to them.

        Yields the processed files into the `patched_apks` directory.
        Any patching failures are caught, reported, and bypassed to allow the batch to continue.
        """
        apk_dir = self.root / "patched_apks"
        apk_dir.mkdir(parents=True, exist_ok=True)
        for fetched_apk in self.fetched_apks:
            try:
                logger.info(f"Patching {fetched_apk.name}...")
                self.patcher("patch", fetched_apk, "-o", apk_dir / fetched_apk.name, f"-p={self.patcher.patch_file}")
            except Exception as e:
                report_error(e, context=f"Failed to patch {fetched_apk.name}")

    

def run_patcher():
    """
    CLI entry point for the `repatcher` command.

    Parses a simple subcommand from `sys.argv[1]` to execute parts of the pipeline:
      - 'jobs': Lists discovered target apps and versions.
      - 'fetch': Downloads the target APKs.
      - 'patch-all': Runs the full fetch-and-patch sequence.
      - (fallback): Passes remaining args directly to the underlying ReVanced CLI.
    """
    logging.basicConfig(level=logging.DEBUG)
    a = App()
    if sys.argv[1] == 'jobs':
        for item in a.jobs:
            print(item, item.apkpure_url)
    elif sys.argv[1] == 'fetch':
        print(a.fetched_apks)
    elif sys.argv[1] == 'patch-all':
        print(a.patched_apks)
    else:
        a.patcher(*sys.argv[1:])