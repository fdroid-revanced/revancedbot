import logging
from pathlib import Path
import time
from selenium import webdriver
from selenium.webdriver.chrome.options import Options

from revancedbot.models import PatchJob

logger = logging.getLogger(__name__)

WINDOW_WIDTH = 1920
WINDOW_HEIGHT = 1080
POLL_INTERVAL_SECONDS = 1
SETTLE_DELAY_SECONDS = 5

class ApkpureFetcher():
    def __init__(self, location: Path):
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
        options.add_argument(f"--window-size={WINDOW_WIDTH},{WINDOW_HEIGHT}")
        options.add_experimental_option("prefs", prefs)
        self.driver = webdriver.Chrome(options=options)

    def url_from_job(self, job: PatchJob):
        return f"https://d.apkpure.com/b/APK/{job.package_id}?version={job.package_version or 'latest'}"

    def fetch(self, job: PatchJob):
        self.driver.get(self.url_from_job(job))

    def wait_settle(self):
        while True:
            logger.info("Checking if downloads are finished")
            pending = list(self.location.glob("*.crdownload"))
            if len(pending) == 0:
                break
            time.sleep(POLL_INTERVAL_SECONDS)
        logger.info(f"Downloads finished, waiting for {SETTLE_DELAY_SECONDS}s")
        time.sleep(SETTLE_DELAY_SECONDS)
        self.driver.close()
