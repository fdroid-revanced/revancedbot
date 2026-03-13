import logging
from pathlib import Path

from revancedbot.fetcher import ApkpureFetcher
from revancedbot.patcher import Patcher
from revancedbot.errors import report_error

logger = logging.getLogger(__name__)

LOW_LIMIT_MAX_JOBS = 3

class App:
    def __init__(self, root=Path("/tmp/revancedbot"), lowlimit=False):
        self.root = root
        self.patcher = Patcher(root/"patcher")
        self.lowlimit = lowlimit
        self._jobs = None
        self._fetched_apks = None

    @property
    def jobs(self):
        if self._jobs is None:
            self._jobs = list(self.patcher.jobs)
        if self.lowlimit:
            self._jobs = self._jobs[:LOW_LIMIT_MAX_JOBS]
        return self._jobs

    @property
    def fetched_apks(self):
        apk_dir = self.root / "downloaded_apks"
        apk_dir.mkdir(parents=True, exist_ok=True)
        if self._fetched_apks is None:
            fetcher = ApkpureFetcher(apk_dir)
            logger.info("Baixando apks...")
            for job in self.jobs:
                version = job.package_version or "latest"
                logger.info(f"Baixando {job.package_id}@{version}")
                fetcher.fetch(job)
            fetcher.wait_settle()
            self._fetched_apks = list(apk_dir.iterdir())
        return self._fetched_apks

    @property
    def patched_apks(self):
        apk_dir = self.root / "patched_apks"
        apk_dir.mkdir(parents=True, exist_ok=True)
        for fetched_apk in self.fetched_apks:
            try:
                logger.info(f"Patching {fetched_apk.name}...")
                self.patcher("patch", fetched_apk, "-o", apk_dir / fetched_apk.name, f"-p={self.patcher.patch_file}")
            except Exception as e:
                report_error(e, {"apk": fetched_apk.name, "action": "patch"})
